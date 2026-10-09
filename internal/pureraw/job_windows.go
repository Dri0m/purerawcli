package pureraw

import (
	"errors"
	"os/exec"
	"strconv"
	"strings"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Running reports whether a PureRAW 6 process is alive in this user's
// session, including one sitting in the notification area (--run-in-tray).
func Running() bool {
	return len(pids()) > 0
}

// pids lists PureRAW processes in our session; other signed-in users'
// copies are none of our business.
func pids() []uint32 {
	var session uint32
	if err := windows.ProcessIdToSessionId(windows.GetCurrentProcessId(), &session); err != nil {
		return nil
	}
	snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return nil
	}
	defer windows.CloseHandle(snap)
	var pids []uint32
	var e windows.ProcessEntry32
	e.Size = uint32(unsafe.Sizeof(e))
	for err = windows.Process32First(snap, &e); err == nil; err = windows.Process32Next(snap, &e) {
		var s uint32
		if strings.EqualFold(windows.UTF16ToString(e.ExeFile[:]), exeName) &&
			windows.ProcessIdToSessionId(e.ProcessID, &s) == nil && s == session {
			pids = append(pids, e.ProcessID)
		}
	}
	return pids
}

// taskkill runs taskkill on our session's PureRAW processes.
func taskkill(flags ...string) error {
	args := append([]string(nil), flags...)
	for _, pid := range pids() {
		args = append(args, "/PID", strconv.FormatUint(uint64(pid), 10))
	}
	if len(args) == len(flags) {
		return nil
	}
	return exec.Command("taskkill", args...).Run()
}

// appCommand runs PureRAW's executable directly, as DxO's Lightroom plugin
// does on Windows. The process is ours, so Wait returns when it exits.
func appCommand(p Paths, args ...string) *exec.Cmd {
	return exec.Command(p.Exe(), args...)
}

// requestQuit asks PureRAW to close its windows (WM_CLOSE), like clicking
// the window's close button.
func requestQuit() int {
	if taskkill() != nil {
		return 0
	}
	return 1
}

// forceQuit is the fallback when PureRAW ignores requestQuit. /T also ends
// its helper processes.
func forceQuit() {
	taskkill("/F", "/T")
	waitExit(10 * time.Second)
}

func lock() (unlock func(), err error) {
	f, err := lockFile()
	if err != nil {
		return nil, err
	}
	const flags = windows.LOCKFILE_EXCLUSIVE_LOCK | windows.LOCKFILE_FAIL_IMMEDIATELY
	if err := windows.LockFileEx(windows.Handle(f.Fd()), flags, 0, 1, 0, &windows.Overlapped{}); err != nil {
		f.Close()
		return nil, errors.New("another purerawcli job is running")
	}
	return func() { f.Close() }, nil
}
