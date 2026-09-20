// Restart command - restart the daemon. Port of apps/cli/commands/restart.ts.
package commands

import (
	"fmt"

	"github.com/Adelodunpeter25/console/apps/cli-go/internal/daemon"
)

// RestartOptions mirrors the TS RestartOptions. Dev pins dev storage;
// otherwise the running daemon's storage mode is preserved.
type RestartOptions struct {
	Port string
	Host string
	Dev  bool
}

// RestartDaemon stops the running daemon (if any) and starts it again.
// Unlike the TS version, restarting a stopped daemon works instead of
// exiting early (TS stopDaemon calls process.exit when nothing runs).
func RestartDaemon(options RestartOptions) error {
	fmt.Println("Restarting daemon...")

	before := daemon.GetStatus()

	if before.Running {
		if err := daemon.KillDaemon(before.Pid); err != nil {
			fmt.Printf("Failed to stop daemon: %v\n", err)
		} else {
			fmt.Println("Daemon stopped successfully")
		}
	} else {
		fmt.Println("Daemon was not running")
	}

	dev := options.Dev
	if !dev && before.Running && before.Mode == daemon.ModeDev {
		dev = true
	}
	return StartDaemon(StartOptions{
		Port: options.Port, Host: options.Host, Daemon: true, Dev: dev,
	})
}
