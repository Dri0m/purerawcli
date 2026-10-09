package pureraw

import (
	"errors"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// Running reports whether one of this user's PureRAW 6 processes is alive.
func Running() bool {
	return len(pids()) > 0
}

func pids() []int {
	out, _ := exec.Command("pgrep", "-U", strconv.Itoa(os.Getuid()), "-x", ProcessName).Output()
	var pids []int
	for _, f := range strings.Fields(string(out)) {
		if pid, err := strconv.Atoi(f); err == nil {
			pids = append(pids, pid)
		}
	}
	return pids
}

// appCommand launches the PureRAW copy Discover found through LaunchServices.
// open -W exits when PureRAW does. Its own process group keeps a terminal
// Ctrl-C from reaching it; purerawcli quits PureRAW itself.
func appCommand(p Paths, args ...string) *exec.Cmd {
	cmd := exec.Command("open", append([]string{"-n", "-W", "-a", p.App, "--args"}, args...)...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	return cmd
}

// forceQuit is the fallback when PureRAW ignores requestQuit.
func forceQuit() {
	signalAll(syscall.SIGTERM)
	if !waitExit(10 * time.Second) {
		signalAll(syscall.SIGKILL)
	}
}

func signalAll(sig syscall.Signal) {
	for _, pid := range pids() {
		syscall.Kill(pid, sig)
	}
}

func lock() (unlock func(), err error) {
	f, err := lockFile()
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		f.Close()
		return nil, errors.New("another purerawcli job is running")
	}
	return func() { f.Close() }, nil
}
