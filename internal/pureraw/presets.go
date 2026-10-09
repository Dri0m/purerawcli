package pureraw

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// Preset is one entry of ProcessingPresets.json. It is kept as a generic map so
// fields added by future PureRAW versions survive the round trip.
type Preset map[string]any

const tempPresetName = "purerawcli (temporary)"

// forcedFields keep the output where the CLI expects it and stop PureRAW from
// handing the result to another application.
var forcedFields = map[string]any{
	"isDefault":                             false,
	"isCustom":                              true,
	"CustomDestinationFolderActivatedValue": true,
	"SubfolderActivatedValue":               false,
	"SubfolderValue":                        "",
	"FileRenamingActivatedValue":            false,
	"ExportToApplicationActivatedValue":     false,
	"ExportToApplicationPathValue":          "",
	"SmartObjectFormat":                     false,
}

func decodePresets(data []byte) ([]Preset, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	var presets []Preset
	if err := dec.Decode(&presets); err != nil {
		return nil, err
	}
	return presets, nil
}

func encodePresets(presets []Preset) ([]byte, error) {
	return json.MarshalIndent(presets, "", "    ")
}

// basePreset returns a copy of the first preset DxO ships with the app
// ("DeepPRIME 3 - DNG"), so results don't depend on the user's own presets.
func basePreset(p Paths) (Preset, error) {
	data, err := os.ReadFile(p.ShippedPresets())
	if err != nil {
		return nil, err
	}
	presets, err := decodePresets(data)
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", p.ShippedPresets(), err)
	}
	if len(presets) == 0 {
		return nil, fmt.Errorf("%s has no presets", p.ShippedPresets())
	}
	base := Preset{}
	for k, v := range presets[0] {
		base[k] = v
	}
	return base, nil
}

// BuildPreset overlays settings and the forced fields on the shipped base preset.
// It returns the field names in settings that the base preset doesn't have,
// which are likely typos or fields from another PureRAW version.
func BuildPreset(p Paths, settings map[string]any, outDir string) (Preset, []string, error) {
	preset, err := basePreset(p)
	if err != nil {
		return nil, nil, err
	}
	var unknown []string
	for k, v := range settings {
		if _, ok := preset[k]; !ok {
			unknown = append(unknown, k)
		}
		preset[k] = v
	}
	for k, v := range forcedFields {
		preset[k] = v
	}
	preset["name"] = tempPresetName
	preset["CustomDestinationFolderValue"] = outDir
	return preset, unknown, nil
}

// appendPreset adds preset to the user's preset list with an unused id and
// returns the new list and the preset's position in it.
func appendPreset(presets []Preset, preset Preset) ([]Preset, int) {
	maxID := int64(-1)
	for _, p := range presets {
		if n, ok := p["id"].(json.Number); ok {
			if id, err := n.Int64(); err == nil && id > maxID {
				maxID = id
			}
		}
	}
	preset["id"] = maxID + 1000
	return append(presets, preset), len(presets)
}

// writeFileAtomic replaces path without leaving a half-written file behind.
func writeFileAtomic(path string, data []byte, perm os.FileMode) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".purerawcli-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Chmod(perm); err != nil {
		tmp.Close()
		return err
	}
	// Flush before the rename, so a crash can't leave an empty file in place.
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}
