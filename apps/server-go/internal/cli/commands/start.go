// Start command - launch the daemon. Port of apps/cli/commands/start.ts.
package commands

import (
	"fmt"
	"os"
	"os/exec"
	"time"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/cli/daemon"
)

// StartOptions mirrors the TS StartOptions: port/host save-and-reuse,
// daemon selects background vs foreground, dev pins dev storage.
type StartOptions struct {
	Port   string
	Host   string
	Daemon bool
	Dev    bool
}

// serverLaunch is how to spawn the server process.
type serverLaunch struct {
	cmd       string
	args      []string
	serveSelf bool
}

// resolveServerLaunch mirrors the old TS multi-call behavior: the CLI binary
// re-executes itself as the server (CONSOLE_SERVE=1), with an explicit
// CONSOLE_SERVER_BIN path as an escape hatch.
func resolveServerLaunch() (serverLaunch, error) {
	if envBin := os.Getenv("CONSOLE_SERVER_BIN"); envBin != "" {
		return serverLaunch{cmd: envBin}, nil
	}
	exe, err := os.Executable()
	if err != nil {
		return serverLaunch{}, fmt.Errorf("resolve server binary: %w", err)
	}
	return serverLaunch{cmd: exe, serveSelf: true}, nil
}

// mergedEnv returns the daemon environment: ~/.console/env under explicit
// process env (which always wins), plus the given overrides.
func mergedEnv(overrides map[string]string) []string {
	merged := daemon.LoadEnvFile()
	for _, kv := range os.Environ() {
		for i := 0; i < len(kv); i++ {
			if kv[i] == '=' {
				merged[kv[:i]] = kv[i+1:]
				break
			}
		}
	}
	for k, v := range overrides {
		merged[k] = v
	}
	env := make([]string, 0, len(merged))
	for k, v := range merged {
		env = append(env, k+"="+v)
	}
	return env
}

// StartDaemon launches the server as a background daemon or in the
// foreground, saving port/host so a later bare `start` reuses them.
func StartDaemon(options StartOptions) error {
	// Pin the storage mode explicitly: --dev wins, then an explicit
	// CONSOLE_ENV, otherwise production. Set before any dir resolution so
	// pid/config paths and the spawned server agree.
	if options.Dev {
		_ = os.Setenv("CONSOLE_ENV", "dev")
	} else if os.Getenv("CONSOLE_ENV") == "" {
		_ = os.Setenv("CONSOLE_ENV", "production")
	}

	status := daemon.GetStatus()
	if status.Running {
		fmt.Printf("Daemon is already running (PID: %d)\n", status.Pid)
		fmt.Printf("Server: http://%s:%s\n", status.Host, status.Port)
		fmt.Printf("Uptime: %s\n", status.Uptime)
		os.Exit(1)
	}

	// CLI flag > env > saved config > default. A bare `start` reuses the
	// saved port/host instead of silently resetting to 3000.
	saved := daemon.LoadConfig(daemon.ConsoleDir())
	port, host := daemon.ResolvePortHost(options.Port, options.Host, saved)
	reusedPort := options.Port == "" && os.Getenv("PORT") == ""
	reusedHost := options.Host == "" && os.Getenv("HOST") == ""
	if err := daemon.SaveConfig(daemon.Config{Port: port, Host: host, LogLevel: "info"}); err != nil {
		return fmt.Errorf("save config: %w", err)
	}
	if err := daemon.EnsureConsoleDir(); err != nil {
		return err
	}
	if reusedPort || reusedHost {
		fmt.Printf("Using saved config from ~/.console/config.json (port %s, host %s).\n", port, host)
	}

	fileEnv := daemon.LoadEnvFile()
	firecrawlStatus := "Firecrawl: no API key (keyless mode) — run 'console env' to add one"
	if os.Getenv("FIRECRAWL_API_KEY") != "" || fileEnv["FIRECRAWL_API_KEY"] != "" {
		firecrawlStatus = "Firecrawl: API key configured"
	}

	launch, err := resolveServerLaunch()
	if err != nil {
		return err
	}

	overrides := map[string]string{
		"PORT": port, "HOST": host, "CONSOLE_DAEMON": "true",
	}
	if launch.serveSelf {
		overrides["CONSOLE_SERVE"] = "1"
	}
	env := mergedEnv(overrides)

	if options.Daemon {
		fmt.Println("Starting console agent daemon...")
		fmt.Printf("Mode: %s (storage: %s)\n", daemon.ResolveMode(), daemon.ConsoleDir())
		fmt.Printf("Port: %s\n", port)
		fmt.Printf("Host: %s\n", host)
		fmt.Println(firecrawlStatus)

		logFile, err := os.OpenFile(daemon.LogFilePath(), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
		if err != nil {
			return fmt.Errorf("open log file: %w", err)
		}
		defer logFile.Close()

		child := exec.Command(launch.cmd, launch.args...)
		child.Env = env
		child.Stdin = nil
		child.Stdout = logFile
		child.Stderr = logFile
		child.SysProcAttr = detachedProcAttr()
		if err := child.Start(); err != nil {
			return fmt.Errorf("spawn daemon: %w", err)
		}
		pid := child.Process.Pid
		// Reap in the background: Wait both collects the exit status and
		// prevents a zombie (a zombie still answers kill(pid, 0), which
		// would fake a "running" daemon). The buffered channel never blocks
		// the reaper; the select below (then the cleanup watcher) drains it.
		exited := make(chan error, 1)
		go func() { exited <- child.Wait() }()

		// Wait a moment and check the process is still running — a server
		// that exits during the grace period is a failed start.
		select {
		case err := <-exited:
			if err != nil {
				fmt.Printf("Daemon exited during startup: %v\n", err)
			}
			fmt.Println("❌ Failed to start daemon")
			os.Exit(1)
		case <-time.After(time.Second):
		}
		// Keep draining so a later daemon exit is reaped (stale pid files
		// are cleared lazily by GetStatus).
		go func() { <-exited }()

		if err := daemon.WritePidFile(pid); err != nil {
			return fmt.Errorf("write pid file: %w", err)
		}
		fmt.Printf("Daemon started successfully (PID: %d)\n", pid)
		fmt.Printf("Server: http://%s:%s\n", host, port)
		fmt.Println("Logs: ~/.console/logs/daemon.log")
		fmt.Println("Run 'console logs' to view logs")
		fmt.Println("Run 'console stop' to stop the daemon")
		return nil
	}

	fmt.Println("Starting console agent in foreground...")
	fmt.Printf("Port: %s\n", port)
	fmt.Printf("Host: %s\n", host)
	fmt.Println(firecrawlStatus)
	fmt.Println("Press Ctrl+C to stop")

	child := exec.Command(launch.cmd, launch.args...)
	child.Env = env
	child.Stdin = os.Stdin
	child.Stdout = os.Stdout
	child.Stderr = os.Stderr
	_ = child.Run()
	os.Exit(0)
	return nil
}
