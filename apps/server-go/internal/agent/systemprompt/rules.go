// Always-apply / inventory rule discovery from `rules/` config dirs. Port of
// apps/server/agent/src/systemprompt/discover-rules.ts.
package systemprompt

import (
	"path/filepath"
	"sort"
	"strings"
)

// Rule is a project/user rule. AlwaysApply rules are injected in full;
// others appear as a short inventory the model can request by name.
type Rule struct {
	Name        string
	Path        string
	Content     string
	Description string
	Globs       []string
	AlwaysApply bool
	Level       ConfigLevel
	Source      SourceMeta
}

func ruleNameFromPath(path string) string {
	base := filepath.Base(path)
	for _, suffix := range []string{".mdc", ".md", ".markdown"} {
		if strings.HasSuffix(strings.ToLower(base), suffix) {
			return base[:len(base)-len(suffix)]
		}
	}
	return base
}

func loadRuleFile(path string, level ConfigLevel) *Rule {
	raw, ok := readTextFile(path)
	if !ok || strings.TrimSpace(raw) == "" {
		return nil
	}
	fm := ParseFrontmatter(raw)
	name := strings.TrimSpace(fm.Values["name"])
	if name == "" {
		name = ruleNameFromPath(path)
	}
	alwaysApply, _ := fm.AsBool("alwaysApply")
	abs, err := filepath.Abs(path)
	if err != nil {
		abs = path
	}
	return &Rule{
		Name:        name,
		Path:        abs,
		Content:     strings.TrimSpace(fm.Body),
		Description: fm.Values["description"],
		Globs:       fm.AsStringSlice("globs"),
		AlwaysApply: alwaysApply,
		Level:       level,
		Source:      newSourceMeta("rules", path, level),
	}
}

func loadRulesFromDir(dir string, level ConfigLevel) []Rule {
	files := listMarkdownFiles(filepath.Join(dir, "rules"))
	var out []Rule
	for _, file := range files {
		if rule := loadRuleFile(file, level); rule != nil {
			out = append(out, *rule)
		}
	}
	return out
}

// DiscoverRules loads rules from user then project config dirs; project
// rules override user rules of the same name, closer project dirs win.
func DiscoverRules(roots Roots) []Rule {
	byName := map[string]Rule{}
	for _, dir := range UserConfigDirs(roots.Home) {
		for _, rule := range loadRulesFromDir(dir, LevelUser) {
			byName[rule.Name] = rule
		}
	}
	projectDirs := ProjectConfigDirs(roots.Cwd, roots.StopAt)
	ordered := append([]AncestorDir{}, projectDirs...)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].Depth > ordered[j].Depth })
	for _, a := range ordered {
		for _, rule := range loadRulesFromDir(a.Dir, LevelProject) {
			byName[rule.Name] = rule
		}
	}
	out := make([]Rule, 0, len(byName))
	for _, rule := range byName {
		out = append(out, rule)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}
