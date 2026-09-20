// Skill discovery from `skills/` config dirs (flat .md and <name>/SKILL.md).
// Port of apps/server/agent/src/systemprompt/discover-skills.ts.
package systemprompt

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Skill is specialized knowledge/workflow the model can load by name via
// the readSkill tool. Listed by name+description in the prompt; full
// content is loaded on demand.
type Skill struct {
	Name        string
	Path        string
	Content     string
	Description string
	Hide        bool
	Level       ConfigLevel
	Source      SourceMeta
}

func skillNameFromPath(path, frontmatterName string) string {
	if strings.TrimSpace(frontmatterName) != "" {
		return strings.TrimSpace(frontmatterName)
	}
	base := filepath.Base(path)
	if strings.EqualFold(base, "skill.md") {
		return filepath.Base(filepath.Dir(path))
	}
	for _, suffix := range []string{".mdc", ".md", ".markdown"} {
		if strings.HasSuffix(strings.ToLower(base), suffix) {
			return base[:len(base)-len(suffix)]
		}
	}
	return base
}

func loadSkillFile(path string, level ConfigLevel) *Skill {
	raw, ok := readTextFile(path)
	if !ok || strings.TrimSpace(raw) == "" {
		return nil
	}
	fm := ParseFrontmatter(raw)
	name := skillNameFromPath(path, fm.Values["name"])
	hide, _ := fm.AsBool("hide")
	disableModelInvocation, _ := fm.AsBool("disableModelInvocation")
	abs, err := filepath.Abs(path)
	if err != nil {
		abs = path
	}
	return &Skill{
		Name:        name,
		Path:        abs,
		Content:     strings.TrimSpace(fm.Body),
		Description: fm.Values["description"],
		Hide:        hide || disableModelInvocation,
		Level:       level,
		Source:      newSourceMeta("skills", path, level),
	}
}

func loadSkillsFromDir(dir string, level ConfigLevel) []Skill {
	skillsDir := filepath.Join(dir, "skills")
	if !isDir(skillsDir) {
		return nil
	}
	var out []Skill
	for _, file := range listMarkdownFiles(skillsDir) {
		if skill := loadSkillFile(file, level); skill != nil {
			out = append(out, *skill)
		}
	}
	entries, err := os.ReadDir(skillsDir)
	if err != nil {
		return out
	}
	for _, e := range entries {
		if !e.IsDir() || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		if skill := loadSkillFile(filepath.Join(skillsDir, e.Name(), "SKILL.md"), level); skill != nil {
			out = append(out, *skill)
		}
	}
	return out
}

// DiscoverSkills loads skills from user and project config; project skills
// override user skills of the same name, closer project dirs win among
// project skills.
func DiscoverSkills(roots Roots) []Skill {
	byName := map[string]Skill{}
	for _, dir := range UserConfigDirs(roots.Home) {
		for _, skill := range loadSkillsFromDir(dir, LevelUser) {
			byName[skill.Name] = skill
		}
	}
	projectDirs := ProjectConfigDirs(roots.Cwd, roots.StopAt)
	ordered := append([]AncestorDir{}, projectDirs...)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].Depth > ordered[j].Depth })
	for _, a := range ordered {
		for _, skill := range loadSkillsFromDir(a.Dir, LevelProject) {
			byName[skill.Name] = skill
		}
	}
	out := make([]Skill, 0, len(byName))
	for _, skill := range byName {
		out = append(out, skill)
	}
	sort.Slice(out, func(i, j int) bool {
		return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
	})
	return out
}
