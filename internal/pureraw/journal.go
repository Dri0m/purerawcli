package pureraw

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// state is everything a job changes, as it was before the job. It is journaled
// before anything changes, so the next run can undo a run that was killed.
type state struct {
	PresetsPath    string         `json:"presetsPath"`
	PresetsExisted bool           `json:"presetsExisted"`
	Presets        []byte         `json:"presets,omitempty"`
	PresetsMode    os.FileMode    `json:"presetsMode,omitempty"`
	Prefs          []prefSnapshot `json:"prefs"`
}

func (s *state) restore() error {
	var errs []error
	for _, p := range s.Prefs {
		errs = append(errs, p.restore())
	}
	if s.PresetsExisted {
		errs = append(errs, writeFileAtomic(s.PresetsPath, s.Presets, s.PresetsMode))
	} else if err := os.Remove(s.PresetsPath); err != nil && !os.IsNotExist(err) {
		errs = append(errs, err)
	}
	return errors.Join(errs...)
}

// stateDir holds the lock and the journal. A variable so tests can redirect it.
var stateDir = func() (string, error) {
	dir, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	dir = filepath.Join(dir, "purerawcli")
	return dir, os.MkdirAll(dir, 0o755)
}

func journalPath() (string, error) {
	dir, err := stateDir()
	return filepath.Join(dir, "journal.json"), err
}

func writeJournal(s *state) error {
	path, err := journalPath()
	if err != nil {
		return err
	}
	data, err := json.Marshal(s)
	if err != nil {
		return err
	}
	return writeFileAtomic(path, data, 0o600)
}

func removeJournal() error {
	path, err := journalPath()
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// recoverInterrupted restores the settings recorded by a run that didn't
// finish, if any. The journal is kept if restoring fails, so it can be retried.
func recoverInterrupted(logf func(string, ...any)) error {
	path, err := journalPath()
	if err != nil {
		return err
	}
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil
	} else if err != nil {
		return err
	}
	var s state
	if err := json.Unmarshal(data, &s); err != nil {
		return fmt.Errorf("unreadable journal %s: %w", path, err)
	}
	logf("restoring PureRAW settings left behind by an interrupted run")
	if err := s.restore(); err != nil {
		return err
	}
	return removeJournal()
}
