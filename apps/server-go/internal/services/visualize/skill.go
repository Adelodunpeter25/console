// The /visualize skill text: instructions the model receives once the user
// invokes the command. Embedded so the grammar the model reads is always the
// grammar reviewed here, with no discovery path to drift from it.
package visualize

import _ "embed"

//go:embed SKILL.md
var skillMarkdown string

// SkillText returns the /visualize instructions.
func SkillText() string { return skillMarkdown }
