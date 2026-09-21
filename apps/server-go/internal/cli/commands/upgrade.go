// Upgrade command - blind re-download of the latest `console` binary.
// Port of apps/cli/commands/upgrade.ts: no version tags, no "already
// current" check — every push to main rebuilds the rolling release, so
// upgrade always fetches and swaps (running it twice is harmless).
package commands

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/cli/daemon"
)

const (
	upgradeRepo    = "Adelodunpeter25/console"
	upgradeBaseURL = "https://github.com/" + upgradeRepo + "/releases/download/console-server"
)

// upgradeTarget is where the new binary and fff sidecar land.
type upgradeTarget struct {
	suffix   string
	binPath  string
	libPath  string
	libAsset string
}

func resolveUpgradeTarget() (upgradeTarget, error) {
	var osName string
	switch runtime.GOOS {
	case "darwin":
		osName = "macos"
	case "linux":
		osName = "linux"
	default:
		return upgradeTarget{}, fmt.Errorf("unsupported OS: %s (only Linux and macOS are supported)", runtime.GOOS)
	}
	var arch string
	switch runtime.GOARCH {
	case "arm64":
		arch = "arm64"
	case "amd64":
		arch = "x64"
	default:
		return upgradeTarget{}, fmt.Errorf("unsupported architecture: %s (only x64 and arm64 are supported)", runtime.GOARCH)
	}

	suffix := osName + "-" + arch
	libAsset := "libfff_c-" + suffix + ".so"
	libFile := "libfff_c.so"
	if osName != "linux" {
		libAsset = "libfff_c-" + suffix + ".dylib"
		libFile = "libfff_c.dylib"
	}

	// A Go binary is always compiled, so the binary swaps itself in place —
	// unless running under `go run` (temp exe), where we target the install
	// dir like the TS dev path does via CONSOLE_INSTALL_DIR.
	if exe, err := os.Executable(); err == nil && !isGoBuildTemp(exe) {
		dir := filepath.Dir(exe)
		return upgradeTarget{suffix: suffix, binPath: exe, libPath: filepath.Join(dir, libFile), libAsset: libAsset}, nil
	}
	prefix := os.Getenv("CONSOLE_INSTALL_DIR")
	if prefix == "" {
		home, _ := os.UserHomeDir()
		prefix = filepath.Join(home, ".local", "bin")
	}
	return upgradeTarget{suffix: suffix, binPath: filepath.Join(prefix, "console"), libPath: filepath.Join(prefix, libFile), libAsset: libAsset}, nil
}

// isGoBuildTemp reports whether exe is a `go run`/`go test` temp binary
// (never a real install target).
func isGoBuildTemp(exe string) bool {
	return strings.Contains(exe, string(filepath.Separator)+"go-build") ||
		strings.HasSuffix(exe, ".test") ||
		strings.Contains(exe, string(filepath.Separator)+"Temp"+string(filepath.Separator))
}

func downloadTo(url, dest string, mode os.FileMode) error {
	resp, err := http.Get(url) //nolint:gosec // release asset URL is a fixed constant + suffix
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download failed (%d): %s", resp.StatusCode, url)
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	tmp := fmt.Sprintf("%s.new-%d", dest, os.Getpid())
	out, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(out, resp.Body)
	closeErr := out.Close()
	if copyErr != nil {
		_ = os.Remove(tmp)
		return copyErr
	}
	if closeErr != nil {
		_ = os.Remove(tmp)
		return closeErr
	}
	return os.Rename(tmp, dest)
}

// UpgradeDaemon downloads the latest binary + fff sidecar and restarts.
func UpgradeDaemon() error {
	target, err := resolveUpgradeTarget()
	if err != nil {
		return err
	}
	before := daemon.GetStatus()
	wasRunning := before.Running
	port := before.Port
	host := before.Host
	if port == "" || host == "" {
		cfg := daemon.LoadConfig(daemon.ConsoleDir())
		if port == "" {
			port = cfg.Port
		}
		if host == "" {
			host = cfg.Host
		}
	}

	fmt.Printf("Upgrading Console (%s)...\n", target.suffix)
	if err := downloadTo(upgradeBaseURL+"/console-"+target.suffix, target.binPath, 0o755); err != nil {
		return err
	}
	if err := downloadTo(upgradeBaseURL+"/"+target.libAsset, target.libPath, 0o644); err != nil {
		return err
	}
	fmt.Println("Binary swapped.")

	if wasRunning {
		fmt.Println("Restarting daemon on the new binary...")
		if before.Pid != 0 {
			if err := daemon.KillDaemon(before.Pid); err != nil {
				fmt.Printf("Could not stop old daemon: %v\n", err)
			}
		}
		return StartDaemon(StartOptions{Port: port, Host: host, Daemon: true, Dev: before.Mode == daemon.ModeDev})
	}
	fmt.Println("Daemon was not running. Run 'console start' to launch the new binary.")
	return nil
}
