//go:build windows

// Package sysproc is the one place that touches process-group and signal
// primitives, so every GOOS-specific syscall stays behind a small API the
// rest of the server can call without build tags.
package sysproc

import (
	"os"
	"os/exec"
	"strconv"

	"golang.org/x/sys/windows"
)

// OwnGroup is a no-op on Windows: there are no Unix process groups, and the
// tree helpers address descendants by walking from the pid instead.
func OwnGroup(cmd *exec.Cmd) {}

// stillActive is the exit code GetExitCodeProcess reports for a live
// process (the Win32 STILL_ACTIVE status, not exported by x/sys/windows).
const stillActive = 259

// Alive reports whether pid is running, probed via an exit-code read on a
// query-only handle. os.FindProcess cannot do this on Windows — it never
// fails — and os.Process.Signal(0) is rejected outright on Windows.
func Alive(pid int) bool {
	if pid <= 0 {
		return false
	}
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if err != nil {
		return false
	}
	defer windows.CloseHandle(h)
	var code uint32
	if err := windows.GetExitCodeProcess(h, &code); err != nil {
		return false
	}
	return code == stillActive
}

// TermProcess politely terminates one process. Windows has no SIGTERM
// delivery to an arbitrary pid, so this is TerminateProcess; a graceful exit
// is only reachable cooperatively, which an unresponsive process does not
// offer.
func TermProcess(pid int) error {
	return killPID(pid)
}

// KillProcess is the same primitive as TermProcess on Windows.
func KillProcess(pid int) error {
	return killPID(pid)
}

// taskKill runs taskkill against pid; /T walks child processes the way a
// Unix group signal covers a process group.
func taskKill(pid int) error {
	return exec.Command("taskkill", "/PID", strconv.Itoa(pid), "/T", "/F").Run()
}

func killPID(pid int) error {
	if pid <= 0 {
		return os.ErrInvalid
	}
	p, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	return p.Kill()
}

// TermTree terminates the tree rooted at pid. taskkill /F grants the tree no
// grace period, but Windows offers no group-level polite signal either.
func TermTree(pid int) {
	if pid <= 0 {
		return
	}
	_ = taskKill(pid)
}

// KillTree is TermTree on Windows.
func KillTree(pid int) {
	if pid <= 0 {
		return
	}
	_ = taskKill(pid)
}
