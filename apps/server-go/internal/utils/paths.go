// Storage path resolution. Port of agent/src/session/apppaths.ts — the
// binary-only NODE_ENV quirk does not apply to Go, so CONSOLE_ENV and
// CONSOLE_STORAGE_DIR are the only inputs. Keep in sync with the TS side.
package utils

import (
	"os"
	"path/filepath"
	"strings"
)

// IgnoredPaths mirrors apps/server/api/src/utils/ignored.ts.
var IgnoredPaths = []string{
	"node_modules", ".git", ".hg", ".svn", "dist", "build", ".next",
	".turbo", ".vite", ".vite-temp", ".cache", "coverage", ".ds_store",
	"thumbs.db", ".gemini", "target", "tmp", ".parcel-cache", "out",
	".output", ".expo", ".gradle", "bin", "obj",
}

// IsPathIgnored reports whether any path segment is an ignored entry.
func IsPathIgnored(path string) bool {
	if path == "" {
		return false
	}
	for _, segment := range strings.Split(path, "/") {
		for _, ignored := range IgnoredPaths {
			if strings.EqualFold(segment, ignored) {
				return true
			}
		}
	}
	return false
}

// IsHiddenName reports whether a client should not show the entry by default.
func IsHiddenName(name string) bool {
	return strings.HasPrefix(name, ".")
}

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
