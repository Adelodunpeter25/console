//go:build !windows

package cua

import (
	"os"
	"syscall"
)

func ownedBy(path string, uid int) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	st, ok := info.Sys().(*syscall.Stat_t)
	return ok && int(st.Uid) == uid
}
