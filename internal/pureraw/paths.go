// Package pureraw drives an installed, licensed DxO PureRAW 6 through the
// command-line entry point DxO's own Lightroom Classic plugin uses.
//
// Settings are passed as a temporary processing preset, selected as the default
// preset, which PureRAW's "process using last settings" plugin mode applies.
// Everything changed is restored when the job ends.
package pureraw

import (
	"path/filepath"
	"runtime"
	"strings"
)

const (
	ProcessName = "PureRAWv6"

	triggerName    = "dxo-trigger-PureRAWv6"
	importListName = "dxo-imported-files-PureRAWv6"
)

// Paths locates everything the driver reads or writes.
type Paths struct {
	App     string // DxO PureRAW 6.app bundle (macOS) or install directory (Windows)
	Version string // e.g. "6.7" (macOS CFBundleShortVersionString) or "6.6.1.1" (Windows file version)
	DataDir string // per-user PureRAW data: presets, logs
	TempDir string // temp dir where PureRAW writes its Lightroom trigger files
}

func (p Paths) Presets() string    { return filepath.Join(p.DataDir, "ProcessingPresets.json") }
func (p Paths) Trigger() string    { return filepath.Join(p.TempDir, triggerName) }
func (p Paths) ImportList() string { return filepath.Join(p.TempDir, importListName) }

// Tested reports whether version is one of TestedVersions, ignoring further
// version components ("6.6.1.1" matches "6.6").
func Tested(version string) bool {
	for _, t := range TestedVersions {
		if version == t || strings.HasPrefix(version, t+".") {
			return true
		}
	}
	return false
}

// pathKey normalizes a path for comparison. PureRAW (Qt) reports Windows paths
// with forward slashes, and Windows paths are case-insensitive.
func pathKey(p string) string {
	p = filepath.Clean(filepath.FromSlash(p))
	if runtime.GOOS == "windows" {
		p = strings.ToLower(p)
	}
	return p
}
