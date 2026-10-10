// Storage path resolution. CONSOLE_ENV and CONSOLE_STORAGE_DIR are the only
// inputs.
package utils

import (
	"os"
	"path/filepath"
	"strings"
)

// IgnoredPaths lists directory and file names ignored by tree and search.
//
// Entries are matched exactly against a whole path segment, case-insensitively
// (see IsPathIgnored). Patterns therefore cannot be expressed here — a
// `cmake-build-debug` entry would never match its `cmake-build-release`
// sibling — so only literal segment names belong in this list.
var IgnoredPaths = []string{
	// Version control.
	".git", ".hg", ".svn",

	// JS / TS: dependencies, bundler caches, framework build output.
	"node_modules", "bower_components", ".pnpm-store", ".yarn", ".pnp.cjs",
	"dist", "build", "out", "tmp", ".next", ".nuxt", ".svelte-kit",
	".turbo", ".nx", ".vite", ".vite-temp", ".cache", ".parcel-cache",
	".eslintcache", ".output", ".expo",

	// Native: Cargo target, C/C++ build trees, Android NDK output.
	"target", "bin", "obj", ".cxx", ".externalNativeBuild",

	// Android / Gradle / Kotlin.
	".gradle", ".kotlin",

	// Python: virtualenvs, interpreter caches, coverage output.
	".venv", "venv", "__pycache__", ".pytest_cache", ".mypy_cache",
	".ruff_cache", ".tox", "htmlcov", ".ipynb_checkpoints",

	// IaC, editors, and assistants.
	".terraform", ".idea", ".vscode", ".gemini",

	// OS cruft and coverage reports.
	".ds_store", "thumbs.db", "coverage",
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

// GitHubCredentialsPath is the GitHub git-credential file
// (<storage>/github-creds.json, dir 0700 / file 0600). Unlike the
// providers/shared.CredentialPath helpers, which pin ~/.console regardless
// of mode, this resolves through ConsoleStorageDir so dev installs and
// CONSOLE_STORAGE_DIR overrides keep their credentials separate.
// GITHUB_CREDENTIALS_PATH wins outright (tests, unusual deployments).
func GitHubCredentialsPath() string {
	if override := os.Getenv("GITHUB_CREDENTIALS_PATH"); override != "" {
		return override
	}
	return filepath.Join(ConsoleStorageDir(), "github-creds.json")
}

// WorktreesDir is the central root for session worktrees
// ($HOME/console/worktrees/<id>). Centralized here — like ConsoleStorageDir —
// so dev and prod differ: CONSOLE_WORKTREES_DIR wins when set, otherwise
// ~/console-dev in dev mode, ~/console in production.
func WorktreesDir() string {
	if override := os.Getenv("CONSOLE_WORKTREES_DIR"); override != "" {
		return override
	}
	home, _ := os.UserHomeDir()
	folder := "console"
	if ConsoleMode() == "dev" {
		folder = "console-dev"
	}
	return filepath.Join(home, folder, "worktrees")
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
