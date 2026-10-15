// Restart command - restart the daemon.
package commands

import (
	"fmt"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/cli/daemon"
)

// RestartOptions configures restart. Dev pins dev storage;
// otherwise the running daemon's storage mode is preserved.
type RestartOptions struct {
	Port string
	Host string
	Dev  bool
	// WT and WTPort must be repeated on every restart. The daemon is
	// restarted from scratch with a fresh environment rather than inheriting
	// the old process's, so a `start --wt` that is not mirrored here would
	// silently come back without WebTransport.
	WT     bool
	WTPort string
}

// RestartDaemon stops the running daemon (if any) and starts it again.
// Restarting a stopped daemon works instead of exiting early.
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
		WT: options.WT, WTPort: options.WTPort,
	})
}
