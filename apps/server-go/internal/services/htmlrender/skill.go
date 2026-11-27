package htmlrender

import _ "embed"

//go:embed SKILL.md
var skillMarkdown string

// SkillText returns the /render instructions.
func SkillText() string { return skillMarkdown }
