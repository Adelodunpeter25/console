// Skill discovery from `skills/` config dirs (flat .md and <name>/SKILL.md).
// Port of apps/server/agent/src/systemprompt/discover-skills.ts + walk.ts:
// user dirs (~/.console|agent|agents) then project ancestors (closest wins).
package services

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
)

var configDirNames = []string{".console", ".agent", ".agents"}

type Skill struct {
	Name        string
	Description string
	Hide        bool
}

type SkillsService struct{}

func NewSkillsService() *SkillsService { return &SkillsService{} }

// Discover lists non-hidden skills applicable to cwd (user + ancestors).
func (s *SkillsService) Discover(cwd string) []Skill {
	byName := map[string]Skill{}

	home, _ := os.UserHomeDir()
	for _, name := range configDirNames {
		dir := filepath.Join(home, name)
		if info, err := os.Stat(dir); err == nil && info.IsDir() {
			for _, skill := range loadSkillsFromDir(dir) {
				byName[skill.Name] = skill
			}
		}
	}

	// Project ancestors, closest last so it overrides user skills of the
	// same name (map overwrite).
	for _, ancestor := range ancestorsOf(cwd) {
		for _, name := range configDirNames {
			dir := filepath.Join(ancestor, name)
			if info, err := os.Stat(dir); err == nil && info.IsDir() {
				for _, skill := range loadSkillsFromDir(dir) {
					byName[skill.Name] = skill
				}
			}
		}
	}

	out := make([]Skill, 0, len(byName))
	for _, skill := range byName {
		out = append(out, skill)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func ancestorsOf(cwd string) []string {
	abs, err := filepath.Abs(cwd)
	if err != nil {
		return nil
	}
	out := []string{}
	for dir := abs; ; dir = filepath.Dir(dir) {
		out = append(out, dir)
		if dir == "/" || dir == "." {
			break
		}
	}
	return out
}

func loadSkillsFromDir(dir string) []Skill {
	skillsDir := filepath.Join(dir, "skills")
	info, err := os.Stat(skillsDir)
	if err != nil || !info.IsDir() {
		return nil
	}
	skills := make([]Skill, 0)

	// Flat markdown files.
	if entries, err := os.ReadDir(skillsDir); err == nil {
		for _, entry := range entries {
			if entry.IsDir() || !isMarkdown(entry.Name()) || strings.HasPrefix(entry.Name(), ".") {
				continue
			}
			if skill := loadSkillFile(filepath.Join(skillsDir, entry.Name()), strings.TrimSuffix(entry.Name(), filepath.Ext(entry.Name()))); skill != nil {
				skills = append(skills, *skill)
			}
		}
		// Nested skills/<name>/SKILL.md.
		for _, entry := range entries {
			if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
				continue
			}
			if skill := loadSkillFile(filepath.Join(skillsDir, entry.Name(), "SKILL.md"), entry.Name()); skill != nil {
				skills = append(skills, *skill)
			}
		}
	}
	return skills
}

func isMarkdown(name string) bool {
	lower := strings.ToLower(name)
	return strings.HasSuffix(lower, ".md") || strings.HasSuffix(lower, ".mdc") || strings.HasSuffix(lower, ".markdown")
}

// loadSkillFile parses frontmatter (name, description, hide,
// disableModelInvocation); nil when the file is missing or hidden.
func loadSkillFile(path, fallbackName string) *Skill {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	meta := parseFrontmatter(string(data))
	name := strings.TrimSpace(meta["name"])
	if name == "" {
		name = fallbackName
	}
	hide := meta["hide"] == "true" || meta["disableModelInvocation"] == "true"
	if hide {
		return nil
	}
	return &Skill{
		Name:        name,
		Description: meta["description"],
		Hide:        hide,
	}
}

// parseFrontmatter extracts key: value pairs from a leading --- block.
func parseFrontmatter(text string) map[string]string {
	out := map[string]string{}
	if !strings.HasPrefix(text, "---") {
		return out
	}
	lines := strings.SplitN(text, "\n", -1)
	for i := 1; i < len(lines); i++ {
		line := strings.TrimSpace(lines[i])
		if line == "---" {
			break
		}
		if idx := strings.Index(line, ":"); idx > 0 {
			key := strings.TrimSpace(line[:idx])
			value := strings.TrimSpace(line[idx+1:])
			value = strings.Trim(value, "\"'")
			out[key] = value
		}
	}
	return out
}
