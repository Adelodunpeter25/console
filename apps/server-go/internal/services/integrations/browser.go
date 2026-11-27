// Web browser integration for the Linux desktop. A minimal XFCE install has
// no graphical browser, so the desktop's Web Browser menu falls back to the
// terminal browser (lynx) that ships on the server image.
package integrations

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// browserBinaries are the graphical browsers worth detecting. lynx and
// w3m are deliberately absent: they are the fallback, not the goal.
var browserBinaries = []string{
	"firefox",
	"firefox-esr",
	"chromium",
	"chromium-browser",
	"google-chrome",
	"epiphany",
	"epiphany-browser",
}

// ErrBrowserPlatformUnsupported marks non-Linux boxes, which already have a
// browser and no apt path.
var ErrBrowserPlatformUnsupported = errors.New("automatic browser install is only supported on Linux")

// CheckBrowser reports whether a graphical browser is installed. Only
// meaningful on Linux; ListIntegrations adds the row there.
func CheckBrowser() IntegrationInfo {
	info := IntegrationInfo{
		ID:          "browser",
		Name:        "Web Browser",
		Description: "Graphical browser for the Linux desktop (the XFCE menu falls back to Lynx without one).",
		InstallURL:  "https://www.chromium.org/",
		InstallHint: "sudo apt install chromium",
	}
	for _, name := range browserBinaries {
		if path, err := exec.LookPath(name); err == nil {
			info.Installed = true
			info.Path = path
			break
		}
	}
	return info
}

// InstallBrowser installs Chromium with apt. It is a no-op when a graphical
// browser already resolves, matching the other installers' contract.
func InstallBrowser(ctx context.Context) (IntegrationInfo, error) {
	current := CheckBrowser()
	if current.Installed {
		return current, nil
	}
	if runtime.GOOS != "linux" {
		return IntegrationInfo{}, fmt.Errorf("%w (%s)", ErrBrowserPlatformUnsupported, runtime.GOOS)
	}
	if err := aptInstall(ctx, []string{"chromium"}); err != nil {
		return IntegrationInfo{}, err
	}
	// The menu reads helpers.rc each time exo-open runs, so point it at the
	// browser we just installed instead of the lynx fallback.
	setXfceWebBrowser("chromium")
	info := CheckBrowser()
	if !info.Installed {
		return IntegrationInfo{}, fmt.Errorf("chromium installed but no browser binary resolves; check PATH")
	}
	return info, nil
}

// setXfceWebBrowser updates ~/.config/xfce4/helpers.rc, preserving any other
// preferred-application entries. Failure is non-fatal: the browser is still
// installed, and XFCE may pick it up on its own.
func setXfceWebBrowser(helper string) {
	home, err := os.UserHomeDir()
	if err != nil {
		return
	}
	path := filepath.Join(home, ".config", "xfce4", "helpers.rc")
	raw, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return
	}

	var lines []string
	if len(raw) > 0 {
		lines = strings.Split(strings.TrimRight(string(raw), "\n"), "\n")
	}
	replaced := false
	for i, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "WebBrowser=") {
			lines[i] = "WebBrowser=" + helper
			replaced = true
		}
	}
	if !replaced {
		lines = append(lines, "WebBrowser="+helper)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return
	}
	_ = os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o644)
}
