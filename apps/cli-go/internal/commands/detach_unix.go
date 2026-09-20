// detaches the child into its own session so it survives the CLI exiting.
package commands

import "syscall"

func detachedProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{Setsid: true}
}
