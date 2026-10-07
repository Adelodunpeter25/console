// The /computer-use skill text: instructions the model receives exactly once,
// appended to the invoking message, when the user types /computer-use.
package cua

import _ "embed"

//go:embed SKILL.md
var skillMarkdown string

// SkillText returns the /computer-use instructions. Embedded so the text the
// model reads is always the text reviewed here, with no discovery path to
// drift from it.
func SkillText() string { return skillMarkdown }
