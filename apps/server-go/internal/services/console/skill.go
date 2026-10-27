// The console-control skill text: instructions the model receives when
// the console tool is available.
package console

import _ "embed"

//go:embed SKILL.md
var skillMarkdown string

// SkillText returns the console-control instructions. Embedded so the text the
// model reads is always the text reviewed here, with no discovery path to
// drift from it.
func SkillText() string { return skillMarkdown }
