// Stop command - terminate the daemon. Port of apps/cli/commands/stop.ts.
package commands

import (
	"fmt"
	"os"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/cli/daemon"
)

// StopDaemon terminates the running daemon.
func StopDaemon() error {
	status := daemon.GetStatus()
	if !status.Running {
		fmt.Println("Daemon is not running")
		os.Exit(1)
	}

	fmt.Printf("Stopping daemon (PID: %d)...\n", status.Pid)
	if err := daemon.KillDaemon(status.Pid); err != nil {
		fmt.Printf("Failed to stop daemon: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("Daemon stopped successfully")
	return nil
}
