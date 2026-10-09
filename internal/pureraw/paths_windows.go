package pureraw

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"unsafe"

	"golang.org/x/sys/windows"
)

const exeName = ProcessName + ".exe"

// TestedVersions are the PureRAW versions purerawcli was tested with. Others
// may work, but purerawcli relies on undocumented behaviour.
var TestedVersions = []string{"6.6"}

func (p Paths) Exe() string            { return filepath.Join(p.App, exeName) }
func (p Paths) Log() string            { return filepath.Join(p.DataDir, "Logs", "PureRAWv6.txt") }
func (p Paths) ShippedPresets() string { return filepath.Join(p.App, "ProcessingPresets.json") }

// Discover finds the PureRAW installation and its per-user directories.
func Discover() (Paths, error) {
	app := filepath.Join(os.Getenv("ProgramFiles"), "DxO", "DxO PureRAW 6")
	exe := filepath.Join(app, exeName)
	if _, err := os.Stat(exe); err != nil {
		return Paths{}, errors.New("DxO PureRAW 6 is not installed (" + exe + " not found)")
	}
	// %LOCALAPPDATA%, where PureRAW keeps DxO_Labs\DxO PureRAW 6.
	local, err := os.UserCacheDir()
	if err != nil {
		return Paths{}, err
	}
	version, _ := fileVersion(exe)
	return Paths{
		App:     app,
		Version: version,
		DataDir: filepath.Join(local, "DxO_Labs", "DxO PureRAW 6"),
		// PureRAW is our child process and inherits %TMP%/%TEMP%, so it
		// resolves the same temp dir (both use GetTempPath).
		TempDir: os.TempDir(),
	}, nil
}

// fileVersion returns the file version from an executable's version resource.
func fileVersion(path string) (string, error) {
	size, err := windows.GetFileVersionInfoSize(path, nil)
	if err != nil {
		return "", err
	}
	buf := make([]byte, size)
	if err := windows.GetFileVersionInfo(path, 0, size, unsafe.Pointer(&buf[0])); err != nil {
		return "", err
	}
	var info *windows.VS_FIXEDFILEINFO
	var n uint32
	if err := windows.VerQueryValue(unsafe.Pointer(&buf[0]), `\`, unsafe.Pointer(&info), &n); err != nil {
		return "", err
	}
	return fmt.Sprintf("%d.%d.%d.%d",
		info.FileVersionMS>>16, info.FileVersionMS&0xffff,
		info.FileVersionLS>>16, info.FileVersionLS&0xffff), nil
}
