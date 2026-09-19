// Storage path resolution. Port of agent/src/session/apppaths.ts — the
// binary-only NODE_ENV quirk does not apply to Go, so CONSOLE_ENV and
// CONSOLE_STORAGE_DIR are the only inputs. Keep in sync with the TS side.
package session

import (
	"os"
	"path/filepath"
)

func ConsoleMode() string {
	switch os.Getenv("CONSOLE_ENV") {
	case "dev":
		return "dev"
	case "production":
		return "production"
	default:
		return "production"
	}
}

func ConsoleStorageDir() string {
	if override := os.Getenv("CONSOLE_STORAGE_DIR"); override != "" {
		return override
	}
	home, _ := os.UserHomeDir()
	folder := ".console"
	if ConsoleMode() == "dev" {
		folder = ".console-dev"
	}
	return filepath.Join(home, folder)
}

func GlobalDBPath() string {
	return filepath.Join(ConsoleStorageDir(), "console-global.db")
}

func ProjectStorageDir(storageDir, projectID string) string {
	return filepath.Join(storageDir, "projects", projectID)
}

func ProjectSessionsDir(storageDir, projectID string) string {
	return filepath.Join(ProjectStorageDir(storageDir, projectID), "sessions")
}

func SessionDBPath(storageDir, projectID, sessionID string) string {
	return filepath.Join(ProjectSessionsDir(storageDir, projectID), sessionID+".db")
}

func ScratchSessionDBPath(storageDir, sessionID string) string {
	return filepath.Join(storageDir, "projects", "scratch", "sessions", sessionID+".db")
}
