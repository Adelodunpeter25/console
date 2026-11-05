// PID file management and process control.
package daemon

import (
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/sysproc"
)

// WritePidFile records pid in ConsoleDir/daemon.pid.
func WritePidFile(pid int) error {
	if err := EnsureConsoleDir(); err != nil {
		return err
	}
	return os.WriteFile(pidFilePath(ConsoleDir()), []byte(strconv.Itoa(pid)), 0o644)
}

// ReadPidFile reads the PID recorded in dir, or 0 if absent/unreadable.
func ReadPidFile(dir string) int {
	data, err := os.ReadFile(pidFilePath(dir))
	if err != nil {
		return 0
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		return 0
	}
	return pid
}

// RemovePidFile deletes dir/daemon.pid if present.
func RemovePidFile(dir string) {
	_ = os.Remove(pidFilePath(dir))
}

// removePidFileFor deletes whichever storage dir's PID file holds pid.
func removePidFileFor(pid int) {
	for _, dir := range []string{ConsoleDir(), AlternateConsoleDir()} {
		if ReadPidFile(dir) == pid {
			RemovePidFile(dir)
		}
	}
}

// IsProcessRunning reports whether pid exists (signal 0, Unix convention).
func IsProcessRunning(pid int) bool {
	return sysproc.Alive(pid)
}

// KillDaemon sends SIGTERM, waits up to 5s for graceful exit, then SIGKILL.
// Removes whichever PID file held pid once it's confirmed gone.
func KillDaemon(pid int) error {
	if err := sysproc.TermProcess(pid); err != nil {
		return err
	}
	for i := 0; i < 50; i++ {
		time.Sleep(100 * time.Millisecond)
		if !sysproc.Alive(pid) {
			removePidFileFor(pid)
			return nil
		}
	}
	_ = sysproc.KillProcess(pid)
	removePidFileFor(pid)
	return nil
}
