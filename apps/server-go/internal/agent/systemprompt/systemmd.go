// SYSTEM.md discovery: project overrides win over user-level. Port of
// apps/server/agent/src/systemprompt/discover-system-md.ts.
package systemprompt

import (
	"path/filepath"
	"sort"
	"strings"
)

// SystemPromptFile is a SYSTEM.md customization that replaces the default
// identity block. Nil when none is present.
type SystemPromptFile struct {
	Path    string
	Content string
	Level   ConfigLevel
	Source  SourceMeta
}

// DiscoverSystemPromptFile returns the nearest project SYSTEM.md, else the
// user-level one, else nil.
func DiscoverSystemPromptFile(roots Roots) *SystemPromptFile {
	projectDirs := ProjectConfigDirs(roots.Cwd, roots.StopAt)
	byDepth := append([]AncestorDir{}, projectDirs...)
	sort.Slice(byDepth, func(i, j int) bool { return byDepth[i].Depth < byDepth[j].Depth })
	for _, a := range byDepth {
		path := filepath.Join(a.Dir, "SYSTEM.md")
		if content, ok := readTextFile(path); ok && strings.TrimSpace(content) != "" {
			abs, err := filepath.Abs(path)
			if err != nil {
				abs = path
			}
			return &SystemPromptFile{Path: abs, Content: content, Level: LevelProject, Source: newSourceMeta("system-md", path, LevelProject)}
		}
	}
	for _, dir := range UserConfigDirs(roots.Home) {
		path := filepath.Join(dir, "SYSTEM.md")
		if content, ok := readTextFile(path); ok && strings.TrimSpace(content) != "" {
			abs, err := filepath.Abs(path)
			if err != nil {
				abs = path
			}
			return &SystemPromptFile{Path: abs, Content: content, Level: LevelUser, Source: newSourceMeta("system-md", path, LevelUser)}
		}
	}
	return nil
}
