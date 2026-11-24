//go:build linux

package computer

import (
	"context"
	"fmt"
	"log/slog"
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
	sessionCmd *exec.Cmd
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

// desktopInstalled reports whether both the virtual X server and an XFCE
// session are on PATH.
func desktopInstalled() bool {
	if _, err := exec.LookPath("Xvfb"); err != nil {
		return false
	}
	if _, err := exec.LookPath("startxfce4"); err == nil {
		return true
	}
	_, err := exec.LookPath("xfce4-session")
	return err == nil
}

// EnsureDisplay ensures a virtual X display and a desktop session are running.
// The processes belong to the service, not to the viewer that triggered them,
// so closing the client leaves the desktop (and its apps) running.
func (d *displayManager) EnsureDisplay(ctx context.Context) (string, error) {
	if isDisplayActive(d.displayNum) {
		return d.displayNum, nil
	}

	if !desktopInstalled() {
		return "", fmt.Errorf("no Linux desktop found: open Settings → Integrations and install Linux Desktop, then reopen the Computer tab")
	}

	xvfbPath, err := exec.LookPath("Xvfb")
	if err != nil {
		return "", fmt.Errorf("Xvfb not found: %w", err)
	}

	// No CommandContext here: the display outlives the viewer's context.
	d.xvfbCmd = exec.Command(xvfbPath, d.displayNum, "-screen", "0", "1920x1080x24", "-shmem", "-ac", "-nolisten", "tcp")
	d.xvfbCmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := d.xvfbCmd.Start(); err != nil {
		d.xvfbCmd = nil
		return "", fmt.Errorf("failed to start Xvfb: %w", err)
	}

	// Wait up to 3 seconds for the X11 socket to become ready.
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

	d.startSession()
	return d.displayNum, nil
}

// startSession starts the best desktop session available: XFCE, then the
// lightweight window managers this package has always supported.
func (d *displayManager) startSession() {
	env := append(os.Environ(),
		"DISPLAY="+d.displayNum,
		"XDG_SESSION_TYPE=x11",
		"XDG_CURRENT_DESKTOP=XFCE",
	)
	if os.Getenv("XDG_RUNTIME_DIR") == "" {
		dir := filepath.Join(os.TempDir(), fmt.Sprintf("runtime-%d", os.Getuid()))
		if err := os.MkdirAll(dir, 0o700); err == nil {
			env = append(env, "XDG_RUNTIME_DIR="+dir)
		}
	}

	type candidate struct {
		path string
		args []string
	}
	var candidates []candidate

	dbusRun, dbusErr := exec.LookPath("dbus-run-session")
	if startxfce4, err := exec.LookPath("startxfce4"); err == nil {
		if dbusErr == nil {
			candidates = append(candidates, candidate{dbusRun, []string{"--", startxfce4}})
		} else {
			candidates = append(candidates, candidate{startxfce4, nil})
		}
	}
	if xfce4Session, err := exec.LookPath("xfce4-session"); err == nil {
		if dbusErr == nil {
			candidates = append(candidates, candidate{dbusRun, []string{"--", xfce4Session}})
		} else {
			candidates = append(candidates, candidate{xfce4Session, nil})
		}
	}
	for _, wm := range []string{"openbox", "fluxbox", "dwm", "i3"} {
		if path, err := exec.LookPath(wm); err == nil {
			candidates = append(candidates, candidate{path, nil})
		}
	}

	for _, c := range candidates {
		cmd := exec.Command(c.path, c.args...)
		cmd.Env = env
		cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
		if err := cmd.Start(); err != nil {
			slog.Warn("computer: failed to start desktop session", "command", c.path, "error", err)
			continue
		}
		d.sessionCmd = cmd
		return
	}
	slog.Warn("computer: no desktop session or window manager found; streaming a bare X display")
}

func (d *displayManager) Close() {
	if d.sessionCmd != nil && d.sessionCmd.Process != nil {
		_ = syscall.Kill(-d.sessionCmd.Process.Pid, syscall.SIGTERM)
		d.sessionCmd = nil
	}
	if d.xvfbCmd != nil && d.xvfbCmd.Process != nil {
		_ = syscall.Kill(-d.xvfbCmd.Process.Pid, syscall.SIGTERM)
		d.xvfbCmd = nil
	}
}
