// readSkill tool: on-demand skill content loader.
// The system prompt lists skills by name + description only; this tool loads
// full content when the model decides a skill is relevant.
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

// Capability is a built-in capability (a typed /command such as /visualize)
// as readSkill sees it. The agent cannot type a slash command, so readSkill is
// how it finds out these exist and how to use them.
type Capability struct {
	Name        string
	Description string
	// Text is the reference, the same text the typed command puts in setup.
	Text string
	// Active reports whether the capability is already switched on.
	Active bool
	// Activate switches it on for the session and loads its tools into the
	// running turn. Nil when only the user may do so (it adds a permission
	// prompt or a real-world side effect, as computer use does).
	Activate func() error
}

// ReadSkill reads skills from disk only. Runs use NewReadSkillTool, which also
// knows the built-in capabilities.
var ReadSkill = NewReadSkillTool(nil)

// NewReadSkillTool builds readSkill. capabilities, when set, is consulted at
// call time so Active reflects the session as it is now.
func NewReadSkillTool(capabilities func() []Capability) Tool {
	return NewTool("readSkill", "Read a skill's full instructions by name. Omit name to list available skills.", TierRead,
		func(ctx context.Context, in readSkillInput) (any, error) {
			return readSkill(in, capabilities)
		})
}

func readSkill(in readSkillInput, capabilities func() []Capability) (any, error) {
	roots := systemprompt.ResolveRoots(in.Cwd, "", "")
	skills := systemprompt.DiscoverSkills(roots)
	var caps []Capability
	if capabilities != nil {
		caps = capabilities()
	}

	query := strings.TrimSpace(in.Name)
	if query == "" {
		if len(skills) == 0 && len(caps) == 0 {
			return textResult("No skills discovered. Skills are loaded from `skills/` directories under `.console`, `.agent`, or `.agents` in the project tree and user home."), nil
		}
		lines := make([]string, 0, len(skills)+len(caps))
		for _, c := range caps {
			state := "built-in, off"
			if c.Active {
				state = "built-in, on"
			}
			lines = append(lines, fmt.Sprintf("- %s: %s (%s)", c.Name, c.Description, state))
		}
		for _, s := range skills {
			desc := strings.TrimSpace(s.Description)
			if desc == "" {
				desc = "(no description)"
			}
			lines = append(lines, fmt.Sprintf("- %s: %s", s.Name, desc))
		}
		return textResult(fmt.Sprintf("Available skills (%d):\n%s\n\nCall readSkill with a name to load full content.", len(lines), strings.Join(lines, "\n"))), nil
	}

	for _, c := range caps {
		if strings.EqualFold(c.Name, query) {
			return readCapability(c)
		}
	}
	var matched *systemprompt.Skill
	for i := range skills {
		if strings.EqualFold(skills[i].Name, query) {
			matched = &skills[i]
			break
		}
	}
	if matched == nil {
		names := make([]string, 0, len(skills)+len(caps))
		for _, c := range caps {
			names = append(names, c.Name)
		}
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
}

// readCapability returns a capability's reference and, when it is off and the
// agent may switch it on, switches it on so the reference is immediately usable.
func readCapability(c Capability) (any, error) {
	note := ""
	switch {
	case c.Active:
	case c.Activate != nil:
		if err := c.Activate(); err != nil {
			return nil, NewToolError("Could not switch on %s: %v", c.Name, err)
		}
		note = fmt.Sprintf("%s was off and is now switched on for this session; its tools are available from your next step.\n\n", c.Name)
	default:
		note = fmt.Sprintf("%s is off in this session and only the user can switch it on: ask them to type /%s. The reference below is for planning only; its tools are not available yet.\n\n", c.Name, c.Name)
	}
	return textResult(fmt.Sprintf("Skill: %s (built-in)\n\n%s%s", c.Name, note, c.Text)), nil
}
