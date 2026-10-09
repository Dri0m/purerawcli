package pureraw

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
)

const (
	BundleID = "com.dxo-labs.PureRAWv6.standalone"

	defaultAppPath = "/Applications/PureRAW 6/DxO PureRAW 6.app"
)

// TestedVersions are the PureRAW versions purerawcli was tested with. Others
// may work, but purerawcli relies on undocumented behaviour.
var TestedVersions = []string{"6.7"}

func (p Paths) Log() string { return filepath.Join(p.DataDir, "Logs", "DxO PureRAW 6.txt") }
func (p Paths) ShippedPresets() string {
	return filepath.Join(p.App, "Contents", "Resources", "ProcessingPresets.json")
}

// Discover finds the PureRAW installation and its per-user directories.
func Discover() (Paths, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return Paths{}, err
	}
	app, err := findApp()
	if err != nil {
		return Paths{}, err
	}
	version, _ := exec.Command("plutil", "-extract", "CFBundleShortVersionString", "raw",
		filepath.Join(app, "Contents", "Info.plist")).Output()
	return Paths{
		App:     app,
		Version: strings.TrimSpace(string(version)),
		DataDir: filepath.Join(home, "Library", "DxO_Labs", "DxO PureRAW 6"),
		TempDir: userTempDir(),
	}, nil
}

// findApp picks the one PureRAW copy that is inspected and launched: the
// default install location, else a Spotlight match, preferring /Applications
// and ignoring copies in the Trash.
func findApp() (string, error) {
	if fi, err := os.Stat(defaultAppPath); err == nil && fi.IsDir() {
		return defaultAppPath, nil
	}
	out, _ := exec.Command("mdfind", fmt.Sprintf("kMDItemCFBundleIdentifier == '%s'", BundleID)).Output()
	var found []string
	for _, line := range strings.Split(string(out), "\n") {
		if strings.HasSuffix(line, ".app") && !strings.Contains(line, "/.Trash/") {
			found = append(found, line)
		}
	}
	slices.SortStableFunc(found, func(a, b string) int {
		inA, inB := strings.HasPrefix(a, "/Applications/"), strings.HasPrefix(b, "/Applications/")
		switch {
		case inA && !inB:
			return -1
		case inB && !inA:
			return 1
		}
		return strings.Compare(a, b)
	})
	if len(found) == 0 {
		return "", errors.New("DxO PureRAW 6 is not installed (bundle " + BundleID + " not found)")
	}
	return found[0], nil
}

// userTempDir returns the directory PureRAW (a launchd-started app) sees as its
// temp dir. DARWIN_USER_TEMP_DIR is authoritative even if $TMPDIR was overridden.
func userTempDir() string {
	if out, err := exec.Command("getconf", "DARWIN_USER_TEMP_DIR").Output(); err == nil {
		if s := strings.TrimSpace(string(out)); s != "" {
			return s
		}
	}
	return os.TempDir()
}
