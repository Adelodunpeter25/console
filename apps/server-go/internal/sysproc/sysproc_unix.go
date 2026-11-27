//go:build !windows

// Package sysproc is the one place that touches process-group and signal
// primitives, so every GOOS-specific syscall stays behind a small API the
// rest of the server can call without build tags.
//
// The tree helpers address a spawned command's descendants: OwnGroup puts it
// in its own process group (Unix) and the kill helpers signal that group,
// falling back to walking the tree by pid where groups do not exist.
package sysproc

import (
	"os/exec"
	"syscall"
)

// OwnGroup gives cmd its own process group so signals can address the whole
// tree, including grandchildren that would otherwise escape a direct kill.
func OwnGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

// Alive reports whether pid exists (signal 0, Unix convention).
func Alive(pid int) bool {
	if pid <= 0 {
		return false
	}
	return syscall.Kill(pid, 0) == nil
}

// TermProcess politely terminates one process (no tree).
func TermProcess(pid int) error {
	if pid <= 0 {
		return syscall.ESRCH
	}
	return syscall.Kill(pid, syscall.SIGTERM)
}

// KillProcess force-kills one process (no tree).
func KillProcess(pid int) error {
	if pid <= 0 {
		return syscall.ESRCH
	}
	return syscall.Kill(pid, syscall.SIGKILL)
}

// TermTree politely terminates the tree rooted at pid: SIGTERM to the process
// group, falling back to the direct pid when the group signal fails (e.g.
// already reaped).
func TermTree(pid int) {
	if pid <= 0 {
		return
	}
	if err := syscall.Kill(-pid, syscall.SIGTERM); err != nil {
		_ = syscall.Kill(pid, syscall.SIGTERM)
	}
}

// KillTree force-kills the tree rooted at pid: group and direct pid, so a
// child that re-parented outside the group does not survive.
func KillTree(pid int) {
	if pid <= 0 {
		return
	}
	_ = syscall.Kill(-pid, syscall.SIGKILL)
	_ = syscall.Kill(pid, syscall.SIGKILL)
}
