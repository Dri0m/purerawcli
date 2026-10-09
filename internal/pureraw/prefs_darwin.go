package pureraw

import (
	"fmt"
	"os/exec"
	"strings"
)

// Writes go through `defaults` so cfprefsd's cache stays coherent.

const PrefsDomain = "com.dxo.DxO PureRAW 6"

// typeFlag is the `defaults write` type flag for a value of kind k.
func (k prefKind) typeFlag() string {
	if k == prefInt {
		return "-int"
	}
	return "-string"
}

// prefSnapshot is a preference as found before a job; it is stored in the
// journal, hence the exported fields.
type prefSnapshot struct {
	Name   string   `json:"name"`
	Kind   prefKind `json:"kind"`
	Exists bool     `json:"exists"`
	Value  string   `json:"value,omitempty"`
}

func (s prefSnapshot) key() prefKey { return prefKey{s.Name, s.Kind} }

func readPref(k prefKey) (prefSnapshot, error) {
	s := prefSnapshot{Name: k.name, Kind: k.kind}
	out, err := exec.Command("defaults", "read", PrefsDomain, k.name).Output()
	if err != nil {
		if _, ok := err.(*exec.ExitError); ok {
			return s, nil // key absent
		}
		return s, err
	}
	s.Exists, s.Value = true, strings.TrimSpace(string(out))
	return s, nil
}

func writePref(k prefKey, value string) error {
	out, err := exec.Command("defaults", "write", PrefsDomain, k.name, k.kind.typeFlag(), value).CombinedOutput()
	if err != nil {
		return fmt.Errorf("defaults write %s: %v: %s", k.name, err, out)
	}
	return nil
}

func (s prefSnapshot) restore() error {
	if s.Exists {
		return writePref(s.key(), s.Value)
	}
	if cur, err := readPref(s.key()); err == nil && !cur.Exists {
		return nil // already absent, e.g. when replaying a journal
	}
	out, err := exec.Command("defaults", "delete", PrefsDomain, s.Name).CombinedOutput()
	if err != nil {
		return fmt.Errorf("defaults delete %s: %v: %s", s.Name, err, out)
	}
	return nil
}
