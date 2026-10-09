package pureraw

import (
	"encoding/json"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"
)

func TestParseImportList(t *testing.T) {
	// Format as written by PureRAW 6.7 for its Lightroom importer.
	data := "DxO PureRAW 6\n" +
		"END_OF_COLLECTION_SET_SECTION\n" +
		"2026-10-09 17:51END_OF_COLLECTION_NAME_SECTION\n" +
		"+ selectCollection\n" +
		"- /in/DSC01834.ARW\n" +
		"- /in/DSC01902.ARW\n" +
		"0 /out/DSC01834-DxO_DeepPRIME 3.dng\n" +
		"1 /out/DSC01902-DxO_DeepPRIME 3.dng\n" +
		"1 /out/DSC01902.tif\n" +
		"/out/orphan.tif\n"
	outputs, orphans := parseImportList(data)
	want := map[string][]string{
		"/in/DSC01834.ARW": {"/out/DSC01834-DxO_DeepPRIME 3.dng"},
		"/in/DSC01902.ARW": {"/out/DSC01902-DxO_DeepPRIME 3.dng", "/out/DSC01902.tif"},
	}
	if !reflect.DeepEqual(outputs, want) {
		t.Errorf("outputs = %v, want %v", outputs, want)
	}
	if !reflect.DeepEqual(orphans, []string{"/out/orphan.tif"}) {
		t.Errorf("orphans = %v", orphans)
	}
}

func TestParseEffectiveSettings(t *testing.T) {
	// Shortened from a PureRAW 6.7 macOS log (hex thread ids).
	log := "1791561198.238 [0x1f5a33f80] INFO DxO PureRAW 6.txt <> - CAuxiliaryOptimizationOperator: Start processing:\n" +
		"Denoising type: DeepPRIME XD3\n" +
		" Force details: 0\n" +
		" Tiff quality: 8-Bit Compressed\n" +
		" Output Dir: \n" +
		" Output Dir Type: Custom folder\n" +
		" Collection name: 2026-10-09 17:53\n" +
		"1791561198.238 [0x1f5a33f80] INFO DxO PureRAW 6.txt <> - CAuxiliaryOptimizationOperator: Output path: /out/a.tif\n" +
		"Not part of the block: x\n"
	got := parseEffectiveSettings(log)
	want := []Setting{
		{"Denoising type", "DeepPRIME XD3"},
		{"Force details", "0"},
		{"Tiff quality", "8-Bit Compressed"},
		{"Output Dir", ""},
		{"Output Dir Type", "Custom folder"},
		{"Collection name", "2026-10-09 17:53"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestParseEffectiveSettingsWindowsLog(t *testing.T) {
	// Windows logs decimal thread ids; the block must end at the next record.
	log := "1791565339.476 [20808] INFO PureRAWv6.txt <> - CProcess::run: Start processing:\n" +
		"    Denoising type: DeepPRIME 3\n" +
		"    Output Settings:\n" +
		"1791565339.477 [20808] DEBUG PureRAWv6.txt <> - CProcess::run: Output path: C:/out/a.tif\n"
	got := parseEffectiveSettings(log)
	want := []Setting{{"Denoising type", "DeepPRIME 3"}, {"Output Settings", ""}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestMatchOutputs(t *testing.T) {
	in := "/in/DSC01834.ARW"
	reported := map[string][]string{"/in/DSC01834.ARW": {"/out/DSC01834.tif"}}
	if runtime.GOOS == "windows" {
		// Qt reports forward slashes; Windows paths are case-insensitive.
		in = `C:\Photos\DSC01834.ARW`
		reported = map[string][]string{"C:/photos/DSC01834.ARW": {"C:/out/DSC01834.tif"}}
	}
	got := matchOutputs([]string{in, filepath.FromSlash("/in/missing.ARW")}, reported)
	want := map[string][]string{in: {filepath.FromSlash(reported[firstKey(reported)][0])}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %q, want %q", got, want)
	}
}

func firstKey(m map[string][]string) string {
	for k := range m {
		return k
	}
	return ""
}

func TestTested(t *testing.T) {
	v := TestedVersions[0]
	for version, want := range map[string]bool{v: true, v + ".1.1": true, v + "9": false, "": false} {
		if Tested(version) != want {
			t.Errorf("Tested(%q) = %v", version, !want)
		}
	}
}

func TestAppendPreset(t *testing.T) {
	presets, err := decodePresets([]byte(`[{"id": 0, "name": "a"}, {"id": 7, "name": "b"}]`))
	if err != nil {
		t.Fatal(err)
	}
	presets, index := appendPreset(presets, Preset{"name": "tmp"})
	if index != 2 || len(presets) != 3 {
		t.Fatalf("index = %d, len = %d", index, len(presets))
	}
	data, err := encodePresets(presets)
	if err != nil {
		t.Fatal(err)
	}
	var back []map[string]any
	if err := json.Unmarshal(data, &back); err != nil {
		t.Fatal(err)
	}
	if back[2]["id"].(float64) != 1007 || back[1]["id"].(float64) != 7 {
		t.Errorf("ids not preserved/assigned: %s", data)
	}
}
