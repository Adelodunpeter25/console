// Logs command - tail daemon logs. Port of apps/cli/commands/logs.ts.
package commands

import (
	"fmt"
	"os"
	"os/exec"
	"os/signal"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/cli/daemon"
)

// LogsOptions mirrors the TS LogsOptions: follow streams, lines caps output.
type LogsOptions struct {
	Follow bool
	Lines  string
}

// LogsDaemon shows the last N lines of the daemon log, or follows it.
func LogsDaemon(options LogsOptions) error {
	if err := daemon.EnsureConsoleDir(); err != nil {
		return err
	}
	logFile := daemon.LogFilePath()
	if _, err := os.Stat(logFile); err != nil {
		fmt.Println("No log file found")
		fmt.Println("Daemon may not have been started yet")
		fmt.Println("Run 'console start' to start the daemon")
		os.Exit(1)
	}

	fmt.Printf("Daemon logs (%s)\nPress Ctrl+C to exit\n\n", logFile)

	if options.Follow {
		tail := exec.Command("tail", "-f", logFile)
		tail.Stdin = os.Stdin
		tail.Stdout = os.Stdout
		tail.Stderr = os.Stderr
		if err := tail.Start(); err != nil {
			fmt.Printf("Failed to tail logs: %v\n", err)
			os.Exit(1)
		}
		sig := make(chan os.Signal, 1)
		signal.Notify(sig, os.Interrupt)
		go func() {
			<-sig
			_ = tail.Process.Kill()
		}()
		_ = tail.Wait()
		return nil
	}

	lines := options.Lines
	if lines == "" {
		lines = "50"
	}
	tail := exec.Command("tail", "-n", lines, logFile)
	tail.Stdin = os.Stdin
	tail.Stdout = os.Stdout
	tail.Stderr = os.Stderr
	if err := tail.Run(); err != nil {
		fmt.Printf("Failed to read logs: %v\n", err)
		os.Exit(1)
	}
	return nil
}
