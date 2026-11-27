//go:build windows

// Windows counterpart of detach_unix.go: a child started with os/exec
// already survives the parent exiting (no controlling terminal to inherit),
// so no SysProcAttr is needed.
package commands

import "syscall"

func detachedProcAttr() *syscall.SysProcAttr {
	return nil
}
