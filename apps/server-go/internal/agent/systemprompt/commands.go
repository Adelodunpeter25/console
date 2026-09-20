// User-defined slash command discovery from `commands/*.md`. Port of
// apps/server/agent/src/systemprompt/discover-commands.ts.
package systemprompt

import (
	"path/filepath"
	"sort"
	"strings"
)

// SlashCommand is a user-defined `/name` command loaded from a config dir.
type SlashCommand struct {
	Name        string
	Path        string
	Content     string
	Description string
	Level       ConfigLevel
	Source      SourceMeta
}

func commandNameFromPath(path, frontmatterName string) string {
	if strings.TrimSpace(frontmatterName) != "" {
		return strings.TrimPrefix(strings.TrimSpace(frontmatterName), "/")
	}
	base := filepath.Base(path)
	for _, suffix := range []string{".mdc", ".md", ".markdown"} {
		if strings.HasSuffix(strings.ToLower(base), suffix) {
			return base[:len(base)-len(suffix)]
		}
	}
	return base
}

func loadCommandFile(path string, level ConfigLevel) *SlashCommand {
	raw, ok := readTextFile(path)
	if !ok || strings.TrimSpace(raw) == "" {
		return nil
	}
	fm := ParseFrontmatter(raw)
	name := commandNameFromPath(path, fm.Values["name"])
	abs, err := filepath.Abs(path)
	if err != nil {
		abs = path
	}
	return &SlashCommand{
		Name:        name,
		Path:        abs,
		Content:     strings.TrimSpace(fm.Body),
		Description: fm.Values["description"],
		Level:       level,
		Source:      newSourceMeta("commands", path, level),
	}
}

func loadCommandsFromDir(dir string, level ConfigLevel) []SlashCommand {
	files := listMarkdownFiles(filepath.Join(dir, "commands"))
	var out []SlashCommand
	for _, file := range files {
		if cmd := loadCommandFile(file, level); cmd != nil {
			out = append(out, *cmd)
		}
	}
	return out
}

// DiscoverCommands loads slash commands from user then project config dirs;
// project commands override user commands of the same name.
func DiscoverCommands(roots Roots) []SlashCommand {
	byName := map[string]SlashCommand{}
	for _, dir := range UserConfigDirs(roots.Home) {
		for _, cmd := range loadCommandsFromDir(dir, LevelUser) {
			byName[cmd.Name] = cmd
		}
	}
	projectDirs := ProjectConfigDirs(roots.Cwd, roots.StopAt)
	ordered := append([]AncestorDir{}, projectDirs...)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].Depth > ordered[j].Depth })
	for _, a := range ordered {
		for _, cmd := range loadCommandsFromDir(a.Dir, LevelProject) {
			byName[cmd.Name] = cmd
		}
	}
	out := make([]SlashCommand, 0, len(byName))
	for _, cmd := range byName {
		out = append(out, cmd)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}
