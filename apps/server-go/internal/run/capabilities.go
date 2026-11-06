// The built-in capabilities: typed /commands that switch on a bundle of tools
// and a reference text for a session. Each is declared once here, and the run
// loop, the slash-command popup and readSkill all read this table, so a
// capability cannot be known to one and missing from another.
package run

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/permissions"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/tools"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/services/browseruse"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/services/console"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/services/cua"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/services/visualize"
)

// Capability is one built-in capability.
type Capability struct {
	// Name is the command without the slash (computer-use, visualize, console).
	Name        string
	Description string
	// Text is the reference added to setup while the capability is on.
	Text func() string
	// Split recognises the typed command in a message.
	Split func(text string) (rest string, ok bool)
	// Load adds the capability's tools to a run's registry.
	Load func(s *Service, c *loadContext, registry *tools.Registry) error
	// AgentMayEnable lets the agent switch it on via readSkill. False when it
	// needs the user's consent or OS permissions (computer use).
	AgentMayEnable bool
	// AlwaysLoaded capabilities have their tools present every turn; only the
	// reference is opt-in.
	AlwaysLoaded bool
	// Tier is the strongest tier of the tools it loads. Modes that deny that
	// tier (plan mode denies exec) neither list nor read the capability.
	Tier tools.ToolTier
	// GatedTools are its always-present tools. Until the reference is on, a
	// call to one is refused and answered with the reference instead.
	GatedTools []string
}

// loadContext is what a Load needs to know about the run.
type loadContext struct {
	sessionID, projectID, cwd, mode string
	// browserHandler is the run's bridge to the desktop's browser, bound at
	// the same place as the low-level browser tool. The browser-use kernel
	// rides it instead of keeping its own connection.
	browserHandler tools.BrowserHandler
}

// Capabilities lists every built-in capability, in popup order.
var Capabilities = []Capability{
	{
		Name:        "computer-use",
		Description: "Drive the computer: read app windows and operate them with keyboard and mouse",
		Text:        cua.SkillText,
		Split:       cua.SplitInvocation,
		Tier:        tools.TierExec,
		Load: func(s *Service, c *loadContext, registry *tools.Registry) error {
			_, err := cua.NewLoader(s.cuaManager, c.sessionID).Load(registry)
			return err
		},
	},
	{
		Name:           "browser-use",
		Description:    "Drive the built-in browser: navigate, observe, extract, and clean up through one JavaScript kernel",
		Text:           browseruse.SkillText,
		Split:          browseruse.SplitInvocation,
		AgentMayEnable: true,
		AlwaysLoaded:   true,
		Tier:           tools.TierRead,
		GatedTools:     []string{"browser"},
		Load: func(s *Service, c *loadContext, registry *tools.Registry) error {
			_, err := browseruse.NewLoader(s.browseruseManager, c.sessionID, c.browserHandler).Load(registry)
			return err
		},
	},
	{
		Name:           "visualize",
		Description:    "Render a chart, table, or metric card in the chat",
		Text:           visualize.SkillText,
		Split:          visualize.SplitInvocation,
		AgentMayEnable: true,
		AlwaysLoaded:   true,
		Tier:           tools.TierRead,
		GatedTools:     []string{"visualize"},
		Load: func(_ *Service, c *loadContext, registry *tools.Registry) error {
			_, err := visualize.NewLoader(c.sessionID).Load(registry)
			return err
		},
	},
	{
		Name:           "console",
		Description:    "Operate the app: run scripts, manage ports and sessions through the console API",
		Text:           console.SkillText,
		Split:          console.SplitInvocation,
		AgentMayEnable: true,
		AlwaysLoaded:   true,
		Tier:           tools.TierExec,
		GatedTools:     []string{"console"},
		Load: func(s *Service, c *loadContext, registry *tools.Registry) error {
			_, err := console.NewLoader(s.consoleManager, c.sessionID, c.projectID, c.cwd, c.mode).Load(registry)
			return err
		},
	},
}

// CapabilityByName finds a capability.
func CapabilityByName(name string) (Capability, bool) {
	for _, c := range Capabilities {
		if strings.EqualFold(c.Name, name) {
			return c, true
		}
	}
	return Capability{}, false
}

// CapabilityDescriptions is the popup's view of the table.
func CapabilityDescriptions() [][2]string {
	out := make([][2]string, 0, len(Capabilities))
	for _, c := range Capabilities {
		out = append(out, [2]string{c.Name, c.Description})
	}
	return out
}

// capabilityOn reports whether a capability is switched on for a session. For an
// always-loaded one this means its reference is on; its tools are present anyway.
func (s *Service) capabilityOn(sessionID string, c Capability) bool {
	on, _ := s.sessions.Capabilities(sessionID)
	return contains(on, c.Name)
}

// activateInvoked switches on every capability the user typed in text.
func (s *Service) activateInvoked(sessionID, text string) {
	for _, c := range Capabilities {
		if _, ok := c.Split(text); ok {
			_ = s.sessions.AddCapability(sessionID, c.Name)
		}
	}
}

// capabilitySetup is the references to append to setup for a session. An
// always-loaded capability's reference is only included once the user asked for
// it (typed it, or the agent read it), keeping the default turn small.
func (s *Service) capabilitySetup(sessionID string) []string {
	var out []string
	on, _ := s.sessions.Capabilities(sessionID)
	for _, c := range Capabilities {
		if contains(on, c.Name) {
			out = append(out, c.Text())
		}
	}
	return out
}

// loadCapabilities adds the tools of every capability that is on.
func (s *Service) loadCapabilities(lc *loadContext, registry *tools.Registry) []string {
	var failures []string
	for _, c := range Capabilities {
		if !c.AlwaysLoaded && !s.capabilityOn(lc.sessionID, c) {
			continue
		}
		if err := c.Load(s, lc, registry); err != nil {
			failures = append(failures, c.Name+" is unavailable: "+err.Error())
			continue
		}
		for _, name := range c.GatedTools {
			if t, err := registry.Get(name); err == nil {
				registry.Replace(&referenceGate{Tool: t, svc: s, sessionID: lc.sessionID, capability: c})
			}
		}
	}
	return failures
}

// referenceGate refuses a capability's tool until its reference is on, then
// switches it on and returns the reference as the answer. A blind call could
// click or run something, so it never executes; the retry does.
type referenceGate struct {
	tools.Tool
	svc        *Service
	sessionID  string
	capability Capability
}

func (g *referenceGate) Execute(ctx context.Context, arguments json.RawMessage) (any, error) {
	if g.svc.capabilityOn(g.sessionID, g.capability) {
		return g.Tool.Execute(ctx, arguments)
	}
	if err := g.svc.sessions.AddCapability(g.sessionID, g.capability.Name); err != nil {
		return nil, tools.NewToolError("Could not load the %s skill: %v", g.capability.Name, err)
	}
	return nil, tools.NewToolError("Not run: you called %s before reading the %s skill. It is loaded now; read it and call %s again.\n\n%s",
		g.Name(), g.capability.Name, g.Name(), g.capability.Text())
}

// allowedIn reports whether the mode permits the capability's tools.
func (c Capability) allowedIn(mode string) bool {
	return permissions.Resolve(permissions.Mode(mode), c.Tier) != permissions.Deny
}

// builtinSkillsSetup lists the built-in skills usable in a mode, so the model
// knows they exist and that its tools need the skill read first. It is names
// and one line each; the references load on demand.
func builtinSkillsSetup(mode string) string {
	var lines []string
	for _, c := range Capabilities {
		if !c.allowedIn(mode) {
			continue
		}
		line := fmt.Sprintf("- %s: %s", c.Name, c.Description)
		if len(c.GatedTools) > 0 {
			line += fmt.Sprintf(" (tool: %s)", strings.Join(c.GatedTools, ", "))
		} else if !c.AgentMayEnable {
			line += " (only the user can switch it on: ask them to type /" + c.Name + ")"
		}
		lines = append(lines, line)
	}
	if len(lines) == 0 {
		return ""
	}
	return "# Built-in skills\nCall readSkill with the name before first using a built-in skill or its tool; the tool refuses to run until you have.\n<builtin-skills>\n" +
		strings.Join(lines, "\n") + "\n</builtin-skills>"
}

// readSkillCapabilities is what the agent's readSkill sees for one run.
func (s *Service) readSkillCapabilities(lc *loadContext, registry *tools.Registry) func() []tools.Capability {
	return func() []tools.Capability {
		out := make([]tools.Capability, 0, len(Capabilities))
		for _, c := range Capabilities {
			c := c
			if !c.allowedIn(lc.mode) {
				continue
			}
			tc := tools.Capability{Name: c.Name, Description: c.Description, Text: c.Text(), Active: s.capabilityOn(lc.sessionID, c)}
			if c.AgentMayEnable {
				tc.Activate = func() error {
					if err := s.sessions.AddCapability(lc.sessionID, c.Name); err != nil {
						return err
					}
					// Loading is a no-op for an always-loaded capability, whose
					// tools are present already; registry.Add skips duplicates.
					return c.Load(s, lc, registry)
				}
			}
			out = append(out, tc)
		}
		return out
	}
}

func contains(list []string, name string) bool {
	for _, item := range list {
		if item == name {
			return true
		}
	}
	return false
}
