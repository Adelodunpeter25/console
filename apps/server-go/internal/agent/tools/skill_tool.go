// readSkill tool: on-demand skill content loader. Port of
// apps/server/agent/src/tools/read-skill.ts. The system prompt lists skills
// by name + description only; this tool loads full content when the model
// decides a skill is relevant.
package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/systemprompt"
)

type readSkillInput struct {
	Name string `json:"name,omitempty" jsonschema:"description=Exact name of the skill to read (as listed in the system prompt). Omit to list every available skill with its description."`
	Cwd  string `json:"cwd,omitempty" jsonschema:"description=Base directory for project-level skill discovery. Defaults to the current directory."`
}

var ReadSkill = NewTool("readSkill", "Read a skill's full instructions by name. Omit name to list available skills.", TierRead,
	func(ctx context.Context, in readSkillInput) (any, error) {
		roots := systemprompt.ResolveRoots(in.Cwd, "", "")
		skills := systemprompt.DiscoverSkills(roots)

		query := strings.TrimSpace(in.Name)
		if query == "" {
			if len(skills) == 0 {
				return textResult("No skills discovered. Skills are loaded from `skills/` directories under `.console`, `.agent`, or `.agents` in the project tree and user home."), nil
			}
			lines := make([]string, 0, len(skills))
			for _, s := range skills {
				desc := strings.TrimSpace(s.Description)
				if desc == "" {
					desc = "(no description)"
				}
				lines = append(lines, fmt.Sprintf("- %s: %s", s.Name, desc))
			}
			return textResult(fmt.Sprintf("Available skills (%d):\n%s\n\nCall readSkill with a name to load full content.", len(skills), strings.Join(lines, "\n"))), nil
		}

		var matched *systemprompt.Skill
		for i := range skills {
			if strings.EqualFold(skills[i].Name, query) {
				matched = &skills[i]
				break
			}
		}
		if matched == nil {
			names := make([]string, 0, len(skills))
			for _, s := range skills {
				names = append(names, s.Name)
			}
			hint := "\n\nNo skills were discovered."
			if len(names) > 0 {
				hint = "\n\nAvailable skills: " + strings.Join(names, ", ")
			}
			return nil, NewToolError("Skill '%s' not found.%s", query, hint)
		}

		return textResult(fmt.Sprintf("Skill: %s\nFile: %s\n\n%s", matched.Name, matched.Path, matched.Content)), nil
	})
