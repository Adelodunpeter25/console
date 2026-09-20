// Status command - check daemon status. Port of apps/cli/commands/status.ts.
package commands

import (
	"fmt"

	"github.com/Adelodunpeter25/console/apps/cli-go/internal/daemon"
)

// StatusDaemon prints whether the daemon is running.
func StatusDaemon() error {
	status := daemon.GetStatus()
	if !status.Running {
		fmt.Println("Daemon is not running")
		fmt.Println("Run 'console start' to start the daemon")
		return nil
	}

	fmt.Println("Daemon is running")
	fmt.Printf("PID: %d\n", status.Pid)
	uptime := status.Uptime
	if uptime == "" {
		uptime = "unknown"
	}
	fmt.Printf("Uptime: %s\n", uptime)
	fmt.Printf("Server: http://%s:%s\n", status.Host, status.Port)
	return nil
}
