// Linux desktop integration: the Xvfb + XFCE stack the Computer tab streams.
// Detection is live like the gh row, and the install button runs apt on the
// server box instead of asking the user to SSH in and do it by hand.
package integrations

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// DesktopPackages is the apt set that provides the virtual display, the XFCE
// session, and a terminal to use inside it.
var DesktopPackages = []string{"xvfb", "xfce4", "xfce4-terminal", "dbus-x11"}

// ErrDesktopPlatformUnsupported marks non-Linux boxes: the apt path does not
// apply there, and macOS/Windows already have a real display to capture.
var ErrDesktopPlatformUnsupported = errors.New("automatic desktop install is only supported on Linux")

// CheckDesktop reports whether the headless desktop stack is present. Only
// meaningful on Linux; ListIntegrations adds the row there.
func CheckDesktop() IntegrationInfo {
	info := IntegrationInfo{
		ID:          "desktop",
		Name:        "Linux Desktop",
		Description: "Virtual X display and XFCE desktop used by the Computer tab.",
		InstallURL:  "https://www.xfce.org",
		InstallHint: "sudo apt install " + strings.Join(DesktopPackages, " "),
	}
	info.Installed = desktopInstalled()
	return info
}

// desktopInstalled requires the X server and an XFCE session. A bare window
// manager is not enough: the Computer tab is meant to show a usable desktop.
// A real X display also counts — there is nothing to install on a desktop box.
func desktopInstalled() bool {
	if realDisplayActive() {
		return true
	}
	if _, err := exec.LookPath("Xvfb"); err != nil {
		return false
	}
	if _, err := exec.LookPath("startxfce4"); err == nil {
		return true
	}
	_, err := exec.LookPath("xfce4-session")
	return err == nil
}

// realDisplayActive reports whether DISPLAY points at a live X socket.
func realDisplayActive() bool {
	display := os.Getenv("DISPLAY")
	if display == "" {
		return false
	}
	num := strings.TrimPrefix(display, ":")
	if i := strings.IndexByte(num, '.'); i >= 0 {
		num = num[:i]
	}
	conn, err := net.Dial("unix", filepath.Join("/tmp/.X11-unix", "X"+num))
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

// InstallDesktop installs the desktop stack with apt. It is a no-op when the
// stack already resolves, matching the gh installer's contract.
func InstallDesktop(ctx context.Context) (IntegrationInfo, error) {
	if desktopInstalled() {
		return CheckDesktop(), nil
	}
	if runtime.GOOS != "linux" {
		return IntegrationInfo{}, fmt.Errorf("%w (%s)", ErrDesktopPlatformUnsupported, runtime.GOOS)
	}
	if err := aptInstall(ctx, DesktopPackages); err != nil {
		return IntegrationInfo{}, err
	}
	info := CheckDesktop()
	if !info.Installed {
		return IntegrationInfo{}, fmt.Errorf("packages installed but Xvfb/XFCE still do not resolve; check PATH")
	}
	return info, nil
}
