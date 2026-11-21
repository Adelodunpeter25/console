//go:build linux

package computer

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"
)

// DefaultDisplay is the virtual display number used on headless Linux.
const DefaultDisplay = ":99"

type displayManager struct {
	xvfbCmd    *exec.Cmd
	wmCmd      *exec.Cmd
	displayNum string
}

func newDisplayManager() *displayManager {
	disp := os.Getenv("DISPLAY")
	if disp == "" {
		disp = DefaultDisplay
	}
	return &displayManager{displayNum: disp}
}

// isDisplayActive checks if the X11 socket for the display exists.
func isDisplayActive(display string) bool {
	num := display
	if len(num) > 0 && num[0] == ':' {
		num = num[1:]
	}
	socketPath := filepath.Join("/tmp/.X11-unix", "X"+num)
	conn, err := net.Dial("unix", socketPath)
	if err == nil {
		conn.Close()
		return true
	}
	return false
}

// EnsureDisplay ensures Xvfb is running on the target display.
func (d *displayManager) EnsureDisplay(ctx context.Context) (string, error) {
	if isDisplayActive(d.displayNum) {
		return d.displayNum, nil
	}

	// Look for Xvfb on PATH
	xvfbPath, err := exec.LookPath("Xvfb")
	if err != nil {
		return "", fmt.Errorf("Xvfb not found on PATH: %w", err)
	}

	// Launch Xvfb with shared memory enabled
	// e.g. Xvfb :99 -screen 0 1920x1080x24 -shmem -ac
	d.xvfbCmd = exec.CommandContext(ctx, xvfbPath, d.displayNum, "-screen", "0", "1920x1080x24", "-shmem", "-ac")
	d.xvfbCmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := d.xvfbCmd.Start(); err != nil {
		return "", fmt.Errorf("failed to start Xvfb: %w", err)
	}

	// Wait up to 3 seconds for X11 socket to become ready
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if isDisplayActive(d.displayNum) {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	if !isDisplayActive(d.displayNum) {
		d.Close()
		return "", fmt.Errorf("timeout waiting for Xvfb on %s", d.displayNum)
	}

	// Optionally start a lightweight window manager if installed (e.g. openbox, fluxbox)
	for _, wm := range []string{"openbox", "fluxbox", "dwm", "i3"} {
		if wmPath, err := exec.LookPath(wm); err == nil {
			cmd := exec.CommandContext(ctx, wmPath)
			cmd.Env = append(os.Environ(), "DISPLAY="+d.displayNum)
			cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
			if err := cmd.Start(); err == nil {
				d.wmCmd = cmd
			}
			break
		}
	}

	return d.displayNum, nil
}

func (d *displayManager) Close() {
	if d.wmCmd != nil && d.wmCmd.Process != nil {
		_ = syscall.Kill(-d.wmCmd.Process.Pid, syscall.SIGTERM)
		d.wmCmd = nil
	}
	if d.xvfbCmd != nil && d.xvfbCmd.Process != nil {
		_ = syscall.Kill(-d.xvfbCmd.Process.Pid, syscall.SIGTERM)
		d.xvfbCmd = nil
	}
}
