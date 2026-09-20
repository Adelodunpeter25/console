// Storage path resolution. Port of apps/cli/daemon-manager.ts's directory
// helpers. Compiled Go binaries have no NODE_ENV quirk to work around (that
// was a Bun-compiled-binary artifact), so CONSOLE_ENV alone decides mode —
// simpler than the TS side, same storage layout.
package daemon

import (
	"os"
	"path/filepath"
)

// Mode is the storage mode: which of ~/.console or ~/.console-dev is used.
type Mode string

const (
	ModeDev        Mode = "dev"
	ModeProduction Mode = "production"
)

// ResolveMode returns the effective storage mode. CONSOLE_ENV=dev|production
// wins explicitly; otherwise production.
func ResolveMode() Mode {
	switch os.Getenv("CONSOLE_ENV") {
	case "dev":
		return ModeDev
	case "production":
		return ModeProduction
	default:
		return ModeProduction
	}
}

func alternateMode() Mode {
	if ResolveMode() == ModeDev {
		return ModeProduction
	}
	return ModeDev
}

func dirForMode(mode Mode) string {
	home, _ := os.UserHomeDir()
	folder := ".console"
	if mode == ModeDev {
		folder = ".console-dev"
	}
	return filepath.Join(home, folder)
}

// ConsoleDir is the active storage directory (~/.console or ~/.console-dev).
func ConsoleDir() string {
	return dirForMode(ResolveMode())
}

// AlternateConsoleDir is the other mode's directory — checked as a fallback
// so `stop`/`status` work regardless of which mode a running daemon used.
func AlternateConsoleDir() string {
	return dirForMode(alternateMode())
}

// LogsDir is the daemon's log directory under ConsoleDir.
func LogsDir() string {
	return filepath.Join(ConsoleDir(), "logs")
}

// LogFilePath is the daemon's log file.
func LogFilePath() string {
	return filepath.Join(LogsDir(), "daemon.log")
}

func pidFilePath(dir string) string {
	return filepath.Join(dir, "daemon.pid")
}

func configFilePath(dir string) string {
	return filepath.Join(dir, "config.json")
}

// EnvFilePath is the per-machine secret env file (KEY=VALUE lines) merged
// into the daemon environment on start.
func EnvFilePath() string {
	return filepath.Join(ConsoleDir(), "env")
}

// EnsureConsoleDir creates the storage and logs directories.
func EnsureConsoleDir() error {
	if err := os.MkdirAll(ConsoleDir(), 0o755); err != nil {
		return err
	}
	return os.MkdirAll(LogsDir(), 0o755)
}
