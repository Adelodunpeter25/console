// Console Agent CLI - daemon management interface.
//
// Commands mirror apps/cli/index.ts: start, stop, status, logs, restart,
// upgrade, env. One deliberate flag divergence: `-h` stays the help flag
// (Go convention), so host is long-only `--host`.
package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/Adelodunpeter25/console/apps/cli-go/internal/commands"
)

func main() {
	root := &cobra.Command{
		Use:     "console",
		Short:   "Console Agent - AI coding agent daemon",
		Version: "1.0.0",
	}

	startPort := ""
	startHost := ""
	startDaemon := true
	startNoDaemon := false
	startDev := false
	startCmd := &cobra.Command{
		Use:   "start",
		Short: "Start the console agent daemon (reuses saved port/host from ~/.console/config.json when flags are omitted)",
		RunE: func(cmd *cobra.Command, args []string) error {
			daemon := startDaemon && !startNoDaemon
			return commands.StartDaemon(commands.StartOptions{
				Port: startPort, Host: startHost, Daemon: daemon, Dev: startDev,
			})
		},
	}
	startCmd.Flags().StringVarP(&startPort, "port", "p", "", "Port to run the server on (saves to config; omit to reuse saved port)")
	startCmd.Flags().StringVar(&startHost, "host", "", "Host to bind to (saves to config; omit to reuse saved host)")
	startCmd.Flags().BoolVar(&startDaemon, "daemon", true, "Run as background daemon")
	startCmd.Flags().BoolVar(&startNoDaemon, "no-daemon", false, "Run in foreground")
	startCmd.Flags().BoolVar(&startDev, "dev", false, "Use dev storage (~/.console-dev) instead of production (~/.console)")
	root.AddCommand(startCmd)

	root.AddCommand(&cobra.Command{
		Use:   "stop",
		Short: "Stop the running console agent daemon",
		RunE: func(cmd *cobra.Command, args []string) error {
			return commands.StopDaemon()
		},
	})

	root.AddCommand(&cobra.Command{
		Use:   "status",
		Short: "Check the status of the console agent daemon",
		RunE: func(cmd *cobra.Command, args []string) error {
			return commands.StatusDaemon()
		},
	})

	logsFollow := false
	logsLines := "50"
	logsCmd := &cobra.Command{
		Use:   "logs",
		Short: "Show console agent daemon logs",
		RunE: func(cmd *cobra.Command, args []string) error {
			return commands.LogsDaemon(commands.LogsOptions{Follow: logsFollow, Lines: logsLines})
		},
	}
	logsCmd.Flags().BoolVarP(&logsFollow, "follow", "f", false, "Follow log output (tail -f)")
	logsCmd.Flags().StringVarP(&logsLines, "lines", "n", "50", "Number of lines to show")
	root.AddCommand(logsCmd)

	root.AddCommand(&cobra.Command{
		Use:   "upgrade",
		Short: "Re-download the latest console binary and restart the daemon",
		RunE: func(cmd *cobra.Command, args []string) error {
			return commands.UpgradeDaemon()
		},
	})

	restartPort := ""
	restartHost := ""
	restartDev := false
	restartCmd := &cobra.Command{
		Use:   "restart",
		Short: "Restart the console agent daemon (reuses saved port/host when flags are omitted)",
		RunE: func(cmd *cobra.Command, args []string) error {
			return commands.RestartDaemon(commands.RestartOptions{
				Port: restartPort, Host: restartHost, Dev: restartDev,
			})
		},
	}
	restartCmd.Flags().StringVarP(&restartPort, "port", "p", "", "Port to run the server on (omit to reuse saved port)")
	restartCmd.Flags().StringVar(&restartHost, "host", "", "Host to bind to (omit to reuse saved host)")
	restartCmd.Flags().BoolVar(&restartDev, "dev", false, "Use dev storage (~/.console-dev) instead of production (~/.console)")
	root.AddCommand(restartCmd)

	root.AddCommand(&cobra.Command{
		Use:   "env",
		Short: "Save provider API keys for the daemon (restart to apply)",
		RunE: func(cmd *cobra.Command, args []string) error {
			return commands.EnvCommand()
		},
	})

	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
