// Status command - check daemon status.
package commands

import (
	"fmt"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/cli/daemon"
)

// StatusDaemon prints whether the daemon is running, and how to pair a
// device with it.
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
	// The address a client should use, not the bind address: a wildcard
	// bind or a tunnel means the two differ.
	fmt.Printf("Server: %s\n", publicBaseURL(status.Dir))
	// The pairing URL is the thing an operator actually wants from
	// `console status`: it is the copy-pasteable line that connects a
	// phone or another machine. It is minted against the *running*
	// server's storage directory, not the current mode's, so a dev daemon
	// gets a grant the dev daemon will honour.
	PrintPairing(status.Dir)
	return nil
}
