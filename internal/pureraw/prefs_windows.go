package pureraw

import (
	"errors"
	"fmt"
	"strconv"

	"golang.org/x/sys/windows/registry"
)

// prefsKey is QSettings' native location for organisation "DxO",
// application "DxO PureRAW 6". Qt reads it once at startup.
const prefsKey = `Software\DxO\DxO PureRAW 6`

// prefSnapshot is a preference as found before a job; it is stored in the
// journal, hence the exported fields.
type prefSnapshot struct {
	Name   string   `json:"name"`
	Kind   prefKind `json:"kind"`
	Exists bool     `json:"exists"`
	Type   uint32   `json:"type,omitempty"` // registry value type, restored as found
	Str    string   `json:"str,omitempty"`
	Num    uint64   `json:"num,omitempty"`
}

func readPref(k prefKey) (prefSnapshot, error) {
	s := prefSnapshot{Name: k.name, Kind: k.kind}
	key, err := registry.OpenKey(registry.CURRENT_USER, prefsKey, registry.QUERY_VALUE)
	if errors.Is(err, registry.ErrNotExist) {
		return s, nil
	} else if err != nil {
		return s, err
	}
	defer key.Close()
	_, s.Type, err = key.GetValue(k.name, nil)
	if errors.Is(err, registry.ErrNotExist) {
		return s, nil
	} else if err != nil {
		return s, fmt.Errorf("read HKCU\\%s\\%s: %w", prefsKey, k.name, err)
	}
	switch s.Type {
	case registry.DWORD, registry.QWORD:
		s.Num, _, err = key.GetIntegerValue(k.name)
	case registry.SZ, registry.EXPAND_SZ:
		s.Str, _, err = key.GetStringValue(k.name)
	default:
		err = fmt.Errorf("unexpected registry type %d", s.Type)
	}
	if err != nil {
		return s, fmt.Errorf("read HKCU\\%s\\%s: %w", prefsKey, k.name, err)
	}
	s.Exists = true
	return s, nil
}

func writePref(k prefKey, value string) error {
	return withPrefsKey(func(key registry.Key) error {
		if k.kind == prefInt {
			n, err := strconv.ParseUint(value, 10, 32)
			if err != nil {
				return err
			}
			return key.SetDWordValue(k.name, uint32(n))
		}
		return key.SetStringValue(k.name, value)
	})
}

func (s prefSnapshot) restore() error {
	return withPrefsKey(func(key registry.Key) error {
		if !s.Exists {
			if err := key.DeleteValue(s.Name); err != nil && !errors.Is(err, registry.ErrNotExist) {
				return err
			}
			return nil
		}
		switch s.Type {
		case registry.DWORD:
			return key.SetDWordValue(s.Name, uint32(s.Num))
		case registry.QWORD:
			return key.SetQWordValue(s.Name, s.Num)
		case registry.EXPAND_SZ:
			return key.SetExpandStringValue(s.Name, s.Str)
		default:
			return key.SetStringValue(s.Name, s.Str)
		}
	})
}

func withPrefsKey(f func(registry.Key) error) error {
	key, _, err := registry.CreateKey(registry.CURRENT_USER, prefsKey, registry.SET_VALUE)
	if err != nil {
		return fmt.Errorf("open HKCU\\%s: %w", prefsKey, err)
	}
	defer key.Close()
	return f(key)
}
