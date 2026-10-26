// Package cli is the Console Agent management CLI (start, stop, status,
// logs, restart, upgrade, env). It backs the default mode of the multi-call
// console binary; the agent server runs in the same binary under
// CONSOLE_SERVE=1.
package cli

import (
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/cli/commands"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/cli/daemon"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/providers/github"
)

// Execute runs the CLI. `-h` stays the help flag (Go convention), so host
// is long-only `--host`.
func Execute() {
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
	startWT := false
	startWTPort := ""
	startCmd := &cobra.Command{
		Use:   "start",
		Short: "Start the console agent daemon (reuses saved port/host from ~/.console/config.json when flags are omitted)",
		RunE: func(cmd *cobra.Command, args []string) error {
			daemon := startDaemon && !startNoDaemon
			return commands.StartDaemon(commands.StartOptions{
				Port: startPort, Host: startHost, Daemon: daemon, Dev: startDev,
				WT: startWT, WTPort: startWTPort,
			})
		},
	}
	startCmd.Flags().StringVarP(&startPort, "port", "p", "", "Port to run the server on (saves to config; omit to reuse saved port)")
	startCmd.Flags().StringVar(&startHost, "host", "", "Host to bind to (saves to config; omit to reuse saved host)")
	startCmd.Flags().BoolVar(&startDaemon, "daemon", true, "Run as background daemon")
	startCmd.Flags().BoolVar(&startNoDaemon, "no-daemon", false, "Run in foreground")
	startCmd.Flags().BoolVar(&startDev, "dev", false, "Use dev storage (~/.console-dev) instead of production (~/.console)")
	startCmd.Flags().BoolVar(&startWT, "wt", false, "Also serve WebTransport over QUIC, generating a development certificate if needed")
	startCmd.Flags().StringVar(&startWTPort, "wt-port", "", "WebTransport/QUIC port (default 14443)")
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

	// `console pair` is the explicit re-pair path: `status` and `start`
	// print a pairing URL as a convenience, but this is the command an
	// operator reaches for when they need another one on purpose.
	pairDev := false
	pairWaitRemote := 0
	pairCmd := &cobra.Command{
		Use:   "pair",
		Short: "Print a pairing URL to connect a new device to this server",
		Long: "Print a copy-pasteable URL that pairs a device with this server.\n" +
			"The URL carries a one-time grant, so it works once. Paste it into the\n" +
			"Console app on the device you want to connect.",
		RunE: func(cmd *cobra.Command, args []string) error {
			// The dev flag has to move the storage mode before anything
			// resolves a path, or the grant would be minted against the
			// wrong server.
			if pairDev {
				_ = os.Setenv("CONSOLE_ENV", "dev")
			} else if os.Getenv("CONSOLE_ENV") == "" {
				_ = os.Setenv("CONSOLE_ENV", "production")
			}
			// A server that is still starting creates its tunnel a moment
			// after launch; wait for it so the address is in the output.
			if pairWaitRemote > 0 {
				commands.WaitForRemote(daemon.ConsoleDir(), time.Duration(pairWaitRemote)*time.Second)
			}
			commands.PrintPairing(daemon.ConsoleDir())
			return nil
		},
	}
	pairCmd.Flags().IntVar(&pairWaitRemote, "wait-remote", 0, "Wait up to this many seconds for the server to create its remote tunnel before printing")
	pairCmd.Flags().BoolVar(&pairDev, "dev", false, "Use dev storage (~/.console-dev) instead of production (~/.console)")
	root.AddCommand(pairCmd)

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
	restartWT := false
	restartWTPort := ""
	restartCmd := &cobra.Command{
		Use:   "restart",
		Short: "Restart the console agent daemon (reuses saved port/host when flags are omitted)",
		RunE: func(cmd *cobra.Command, args []string) error {
			return commands.RestartDaemon(commands.RestartOptions{
				Port: restartPort, Host: restartHost, Dev: restartDev,
				WT: restartWT, WTPort: restartWTPort,
			})
		},
	}
	restartCmd.Flags().StringVarP(&restartPort, "port", "p", "", "Port to run the server on (omit to reuse saved port)")
	restartCmd.Flags().StringVar(&restartHost, "host", "", "Host to bind to (omit to reuse saved host)")
	restartCmd.Flags().BoolVar(&restartDev, "dev", false, "Use dev storage (~/.console-dev) instead of production (~/.console)")
	restartCmd.Flags().BoolVar(&restartWT, "wt", false, "Also serve WebTransport over QUIC (repeat on every restart; not remembered)")
	restartCmd.Flags().StringVar(&restartWTPort, "wt-port", "", "WebTransport/QUIC port (default 14443)")
	root.AddCommand(restartCmd)

	certDev := false
	certCmd := &cobra.Command{
		Use:   "cert-fingerprint",
		Short: "Print the SHA-256 fingerprint clients use to pin the development TLS certificate",
		Long: "Print the SHA-256 fingerprint of the development certificate that `console start --wt`\n" +
			"serves, generating the certificate if it does not exist yet. Pass the value to a\n" +
			"client as CONSOLE_WT_TLS=pin:<fingerprint>.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return commands.CertFingerprint(certDev)
		},
	}
	certCmd.Flags().BoolVar(&certDev, "dev", false, "Use dev storage (~/.console-dev) instead of production (~/.console)")
	root.AddCommand(certCmd)

	root.AddCommand(&cobra.Command{
		Use:   "env",
		Short: "Save provider API keys for the daemon (restart to apply)",
		RunE: func(cmd *cobra.Command, args []string) error {
			return commands.EnvCommand()
		},
	})

	root.AddCommand(commands.ComputerUseCommand())

	// Hidden third role of the multi-call binary. git does NOT reach this:
	// it invokes `!CONSOLE_GIT_CREDENTIAL_HELPER=1 <console>` with no
	// subcommand, which cmd/console/main.go dispatches before the serve
	// branch (that ordering is load-bearing — see main.go). This cobra
	// command exists so the helper can be driven by hand for debugging:
	//
	//   printf 'protocol=https\nhost=github.com\n\n' \
	//     | console git-credential-helper get
	//
	// Run (not RunE) plus os.Exit keeps stdout free of anything but the
	// credential protocol.
	root.AddCommand(&cobra.Command{
		Use:    github.HelperSubcommand,
		Short:  "git credential helper (internal)",
		Hidden: true,
		Args:   cobra.MaximumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			op := ""
			if len(args) == 1 {
				op = args[0]
			}
			os.Exit(github.RunCredentialHelper(op, os.Stdin, os.Stdout))
		},
	})

	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
