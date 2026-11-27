// Shared apt installer for the Linux integrations. Keeping the sudo/env
// handling in one place means the desktop and browser rows behave the same
// way when the box lacks passwordless sudo.
package integrations

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"strings"
	"time"
)

// aptInstall runs apt-get update and install for packages. sudo -n is used
// when the server is not root: a password prompt would hang the request, so a
// box without passwordless sudo gets a clear error instead of a stuck button.
func aptInstall(ctx context.Context, packages []string) error {
	if _, err := exec.LookPath("apt-get"); err != nil {
		return fmt.Errorf("apt-get not found; install %s with your package manager", strings.Join(packages, " "))
	}

	prefix := []string{}
	if os.Geteuid() != 0 {
		sudo, err := exec.LookPath("sudo")
		if err != nil {
			return fmt.Errorf("packages are missing and sudo is unavailable; run: sudo apt install %s", strings.Join(packages, " "))
		}
		prefix = []string{sudo, "-n"}
	}

	run := func(args ...string) error {
		cmdArgs := append([]string{}, prefix...)
		cmdArgs = append(cmdArgs, "env", "DEBIAN_FRONTEND=noninteractive", "NEEDRESTART_MODE=a")
		cmdArgs = append(cmdArgs, args...)

		runCtx, cancel := context.WithTimeout(ctx, 20*time.Minute)
		defer cancel()
		out, err := exec.CommandContext(runCtx, cmdArgs[0], cmdArgs[1:]...).CombinedOutput()
		if err != nil {
			return fmt.Errorf("%s failed: %s", strings.Join(args, " "), lastLines(string(out), 8))
		}
		return nil
	}

	slog.Info("integrations: installing packages", "packages", strings.Join(packages, " "))
	if err := run("apt-get", "update"); err != nil {
		// A stale index can still satisfy the install from cache; try it.
		slog.Warn("integrations: apt-get update failed; trying install anyway", "error", err)
	}
	installArgs := append([]string{"apt-get", "install", "-y"}, packages...)
	if err := run(installArgs...); err != nil {
		return fmt.Errorf("install packages: %w", err)
	}
	return nil
}

// lastLines keeps the end of apt output, where the actual failure usually is.
func lastLines(s string, n int) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, "\n")
}
