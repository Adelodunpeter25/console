// Filesystem walk-up helpers for config discovery. Port of
// apps/server/agent/src/systemprompt/walk.ts.
package systemprompt

import (
	"os"
	"path/filepath"
)

// ProjectConfigDirNames are project-level config directory names checked at
// each ancestor of cwd.
var ProjectConfigDirNames = []string{".console", ".agent", ".agents"}

// UserConfigDirNames are user-level config directory names checked under home.
var UserConfigDirNames = []string{".console", ".agent", ".agents"}

// ConfigLevel distinguishes user-home config from project-tree config.
type ConfigLevel string

const (
	LevelUser    ConfigLevel = "user"
	LevelProject ConfigLevel = "project"
)

// SourceMeta records where a discovered item was loaded from.
type SourceMeta struct {
	Provider string      `json:"provider"`
	Path     string      `json:"path"`
	Level    ConfigLevel `json:"level"`
}

func newSourceMeta(provider, path string, level ConfigLevel) SourceMeta {
	abs, err := filepath.Abs(path)
	if err != nil {
		abs = path
	}
	return SourceMeta{Provider: provider, Path: abs, Level: level}
}

// Roots resolves the effective cwd/home/stopAt for one discovery run.
type Roots struct {
	Cwd    string
	Home   string
	StopAt string
}

// ResolveRoots fills in defaults (process cwd, os home dir) for unset fields.
func ResolveRoots(cwd, home, stopAt string) Roots {
	if cwd == "" {
		if wd, err := os.Getwd(); err == nil {
			cwd = wd
		}
	}
	if abs, err := filepath.Abs(cwd); err == nil {
		cwd = abs
	}
	if home == "" {
		home, _ = os.UserHomeDir()
	}
	if stopAt != "" {
		if abs, err := filepath.Abs(stopAt); err == nil {
			stopAt = abs
		}
	}
	return Roots{Cwd: cwd, Home: home, StopAt: stopAt}
}

// AncestorDir is one directory on the walk from cwd up to the filesystem
// root (or StopAt), with its distance from cwd.
type AncestorDir struct {
	Dir   string
	Depth int
}

// AncestorDirs lists cwd and each parent up to stopAt (inclusive) or the
// filesystem root, cwd first.
func AncestorDirs(cwd, stopAt string) []AncestorDir {
	current := cwd
	if abs, err := filepath.Abs(current); err == nil {
		current = abs
	}
	var out []AncestorDir
	depth := 0
	for {
		out = append(out, AncestorDir{Dir: current, Depth: depth})
		if stopAt != "" && current == stopAt {
			break
		}
		parent := filepath.Dir(current)
		if parent == current {
			break
		}
		current = parent
		depth++
	}
	return out
}

func readTextFile(path string) (string, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", false
	}
	return string(data), true
}

func isDir(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

// listMarkdownFiles lists .md/.mdc/.markdown files directly in dir, sorted.
func listMarkdownFiles(dir string) []string {
	if !isDir(dir) {
		return nil
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range entries {
		if e.IsDir() || !isMarkdownName(e.Name()) {
			continue
		}
		out = append(out, filepath.Join(dir, e.Name()))
	}
	return out
}

func isMarkdownName(name string) bool {
	for _, suffix := range []string{".md", ".mdc", ".markdown"} {
		if len(name) > len(suffix) && name[len(name)-len(suffix):] == suffix {
			return true
		}
	}
	return false
}

// UserConfigDirs returns existing user-level config dirs under home.
func UserConfigDirs(home string) []string {
	var out []string
	for _, name := range UserConfigDirNames {
		dir := filepath.Join(home, name)
		if isDir(dir) {
			out = append(out, dir)
		}
	}
	return out
}

// ProjectConfigDirs returns existing project-level config dirs at each
// ancestor of cwd (closest first), paired with their depth.
func ProjectConfigDirs(cwd, stopAt string) []AncestorDir {
	var out []AncestorDir
	for _, a := range AncestorDirs(cwd, stopAt) {
		for _, name := range ProjectConfigDirNames {
			dir := filepath.Join(a.Dir, name)
			if isDir(dir) {
				out = append(out, AncestorDir{Dir: dir, Depth: a.Depth})
			}
		}
	}
	return out
}
