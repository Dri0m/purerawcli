package pureraw

import (
	"os"
	"path/filepath"
	"testing"
)

// useTempStateDir points the lock and journal at a temporary directory.
func useTempStateDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	orig := stateDir
	stateDir = func() (string, error) { return dir, nil }
	t.Cleanup(func() { stateDir = orig })
	return dir
}

func TestRecoverInterrupted(t *testing.T) {
	useTempStateDir(t)
	presets := filepath.Join(t.TempDir(), "ProcessingPresets.json")
	original := []byte(`[{"id": 0}]`)

	// A run journaled the original file, changed it and was killed.
	if err := writeJournal(&state{PresetsPath: presets, PresetsExisted: true, Presets: original, PresetsMode: 0o644}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(presets, []byte(`[{"id": 0}, {"name": "purerawcli (temporary)"}]`), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := recoverInterrupted(t.Logf); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(presets); string(got) != string(original) {
		t.Errorf("presets = %s, want %s", got, original)
	}
	journal, _ := journalPath()
	if _, err := os.Stat(journal); !os.IsNotExist(err) {
		t.Errorf("journal still present: %v", err)
	}
	// Nothing left to recover.
	if err := recoverInterrupted(t.Logf); err != nil {
		t.Fatal(err)
	}
}

func TestRecoverRemovesCreatedPresets(t *testing.T) {
	useTempStateDir(t)
	presets := filepath.Join(t.TempDir(), "ProcessingPresets.json")
	if err := writeJournal(&state{PresetsPath: presets}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(presets, []byte(`[]`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := recoverInterrupted(t.Logf); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(presets); !os.IsNotExist(err) {
		t.Errorf("presets file the run created still exists: %v", err)
	}
}
