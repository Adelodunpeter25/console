// AGENTS.md / CLAUDE.md context-file discovery. Port of
// apps/server/agent/src/systemprompt/discover-agents-md.ts.
package systemprompt

import (
	"path/filepath"
	"sort"
	"strings"
)

// ContextFile is a persistent-instructions file (AGENTS.md, CLAUDE.md, …)
// injected into the system prompt under repo rules.
type ContextFile struct {
	Path    string
	Content string
	Level   ConfigLevel
	Depth   int // -1 when not applicable (user-level files)
	Source  SourceMeta
}

// rootContextNames are standalone filenames treated as context files when
// found at ancestor roots.
var rootContextNames = []string{"AGENTS.md", "CLAUDE.md", "GEMINI.md", "CODEX.md", ".cursorrules"}

// configContextNames are context filenames inside config dirs.
var configContextNames = []string{"AGENTS.md", "CLAUDE.md", "GEMINI.md"}

func tryLoadContext(path string, level ConfigLevel, depth int, provider string) *ContextFile {
	content, ok := readTextFile(path)
	if !ok || strings.TrimSpace(content) == "" {
		return nil
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		abs = path
	}
	return &ContextFile{Path: abs, Content: content, Level: level, Depth: depth, Source: newSourceMeta(provider, path, level)}
}

// DiscoverContextFiles loads project + user context files. Farther project
// files sort first, closest project files last (most prominent).
func DiscoverContextFiles(roots Roots) []ContextFile {
	var items []ContextFile

	// 1. Standalone root-level files walking up from cwd; skip dirs whose
	// own name is a hidden dir we don't recognize (e.g. inside .git).
	for _, a := range AncestorDirs(roots.Cwd, roots.StopAt) {
		base := filepath.Base(a.Dir)
		if strings.HasPrefix(base, ".") && base != "." && base != ".." {
			continue
		}
		for _, name := range rootContextNames {
			if cf := tryLoadContext(filepath.Join(a.Dir, name), LevelProject, a.Depth, "agents-md"); cf != nil {
				items = append(items, *cf)
			}
		}
	}

	// 2. Config-dir context files at each ancestor (.agent/AGENTS.md, …).
	for _, a := range ProjectConfigDirs(roots.Cwd, roots.StopAt) {
		for _, name := range configContextNames {
			if cf := tryLoadContext(filepath.Join(a.Dir, name), LevelProject, a.Depth, "agents-md-config"); cf != nil {
				items = append(items, *cf)
			}
		}
	}

	// 3. User-level.
	for _, dir := range UserConfigDirs(roots.Home) {
		for _, name := range configContextNames {
			if cf := tryLoadContext(filepath.Join(dir, name), LevelUser, -1, "agents-md-user"); cf != nil {
				items = append(items, *cf)
			}
		}
	}

	sort.SliceStable(items, func(i, j int) bool {
		if items[i].Depth != items[j].Depth {
			return items[i].Depth > items[j].Depth // farther first
		}
		if items[i].Level != items[j].Level {
			return items[i].Level == LevelUser // user before project among ties
		}
		return items[i].Path < items[j].Path
	})

	return dedupeByContent(items)
}

func dedupeByContent(files []ContextFile) []ContextFile {
	lastIndex := map[string]int{}
	for i, f := range files {
		lastIndex[f.Content] = i
	}
	out := make([]ContextFile, 0, len(files))
	for i, f := range files {
		if lastIndex[f.Content] == i {
			out = append(out, f)
		}
	}
	return out
}
