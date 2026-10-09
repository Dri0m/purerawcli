package pureraw

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

// testInput returns the raw file the end-to-end test processes: a single
// input.<ext> (e.g. input.dng) in the repository root, provided by whoever
// runs the tests. It must be a camera raw, not a file PureRAW already
// processed, which PureRAW rejects with a blocking dialog.
func testInput(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			break
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("repository root (go.mod) not found")
		}
		dir = parent
	}
	matches, _ := filepath.Glob(filepath.Join(dir, "input.*"))
	if len(matches) != 1 {
		t.Fatalf("put exactly one camera raw file named input.<ext> (e.g. input.dng) in %s; found %d", dir, len(matches))
	}
	return matches[0]
}

// TestRunEndToEnd processes the test input with the installed PureRAW and
// checks the output and that PureRAW's settings are left as they were.
// PureRAW must be installed, activated and not running.
func TestRunEndToEnd(t *testing.T) {
	input := testInput(t)
	p, err := Discover()
	if err != nil {
		t.Fatal(err)
	}
	if Running() {
		t.Fatal("quit DxO PureRAW before running the tests")
	}
	presetsBefore, presetsErr := os.ReadFile(p.Presets())
	prefsBefore := readPrefs(t)

	out := t.TempDir()
	res, err := Run(context.Background(), p, Job{
		Inputs: []string{input},
		OutDir: out,
		Settings: map[string]any{
			"DngOutputFormat":  false,
			"TiffOutputFormat": false,
			"JpegOutputFormat": true,
		},
		Timeout: 3 * time.Minute,
		Logf:    t.Logf,
	})
	if err != nil {
		t.Fatal(err)
	}

	if len(res.Missing) > 0 {
		t.Errorf("PureRAW produced nothing for %s (unsupported or already processed?)", input)
	}
	want := filepath.Join(out, strings.TrimSuffix(filepath.Base(input), filepath.Ext(input))+".jpg")
	if got := res.Outputs[input]; len(got) != 1 || pathKey(got[0]) != pathKey(want) {
		t.Errorf("outputs = %q, want [%q]", got, want)
	}
	if fi, err := os.Stat(want); err != nil || fi.Size() == 0 {
		t.Errorf("output %s: %v", want, err)
	}
	settings := map[string]string{}
	for _, s := range res.Settings {
		settings[s.Name] = s.Value
	}
	if settings["Output Dir Type"] != "Custom folder" {
		t.Errorf("PureRAW's log shows %q, want the temporary preset's custom folder", settings["Output Dir Type"])
	}

	presetsAfter, presetsErrAfter := os.ReadFile(p.Presets())
	if string(presetsAfter) != string(presetsBefore) || os.IsNotExist(presetsErr) != os.IsNotExist(presetsErrAfter) {
		t.Error("presets file not restored")
	}
	if prefsAfter := readPrefs(t); !reflect.DeepEqual(prefsAfter, prefsBefore) {
		t.Errorf("preferences not restored: %+v, want %+v", prefsAfter, prefsBefore)
	}
	journal, _ := journalPath()
	for _, f := range []string{journal, p.Trigger(), p.ImportList()} {
		if _, err := os.Stat(f); !os.IsNotExist(err) {
			t.Errorf("%s left behind", f)
		}
	}
}

func readPrefs(t *testing.T) []prefSnapshot {
	t.Helper()
	var snaps []prefSnapshot
	for _, k := range []prefKey{prefDefaultPresetIndex, prefHideFinalDialog} {
		s, err := readPref(k)
		if err != nil {
			t.Fatal(err)
		}
		snaps = append(snaps, s)
	}
	return snaps
}
