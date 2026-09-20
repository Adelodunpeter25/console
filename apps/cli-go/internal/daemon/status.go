// Daemon status: running/pid/uptime/port/host/mode. Port of getDaemonStatus
// in apps/cli/daemon-manager.ts, including the dual-directory fallback so
// stop/status work regardless of which storage mode the running daemon used.
package daemon

import (
	"os/exec"
	"strconv"
	"strings"
)

// Status is the point-in-time daemon status.
type Status struct {
	Running bool
	Pid     int
	Uptime  string
	Port    string
	Host    string
	Mode    Mode
}

// GetStatus checks ConsoleDir first, then AlternateConsoleDir, clearing any
// stale PID file it finds along the way.
func GetStatus() Status {
	for _, dir := range []string{ConsoleDir(), AlternateConsoleDir()} {
		pid := ReadPidFile(dir)
		if pid == 0 {
			continue
		}
		if !IsProcessRunning(pid) {
			RemovePidFile(dir)
			continue
		}
		return buildRunningStatus(pid, dir)
	}
	return Status{Running: false}
}

func buildRunningStatus(pid int, dir string) Status {
	uptime := ""
	out, err := exec.Command("ps", "-p", strconv.Itoa(pid), "-o", "etime=").Output()
	if err != nil {
		// Process might have died between the running-check and here.
		return Status{Running: false}
	}
	uptime = strings.TrimSpace(string(out))

	cfg := LoadConfig(dir)
	mode := ModeProduction
	if dir == ConsoleDir() {
		mode = ResolveMode()
	} else {
		mode = alternateMode()
	}
	return Status{
		Running: true, Pid: pid, Uptime: uptime,
		Port: cfg.Port, Host: cfg.Host, Mode: mode,
	}
}
