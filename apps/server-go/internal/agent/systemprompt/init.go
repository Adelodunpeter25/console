// Built-in `/init` slash command constants. Port of
// apps/server/agent/src/commands/init.ts. Prompt-level only — the agent
// does the work with its normal file tools.
package systemprompt

import (
	"regexp"
	"strings"
)

const (
	InitCommandName        = "init"
	InitCommandDescription = "Generate a console.toml with project run scripts for the Run tab"
)

// InitCommandDetails is inlined under /init in the system prompt so the
// agent knows the console.toml format without a tool round-trip.
var InitCommandDetails = strings.Join([]string{
	"Usage: /init — inspect package.json, Makefile, and other project manifests, then write console.toml at the project root.",
	"Schema: one [scripts.<id>] table per script; id matches /^[A-Za-z0-9_-]+$/.",
	"Required per script: label (non-empty), command (non-empty, runs with the repo root as working directory).",
	"Optional per script: shortcut (e.g. cmd-shift-r), persistent (boolean, true for long-running dev servers).",
	"Rules: never overwrite an existing console.toml without asking; validate the result parses before finishing.",
	"If the project has no console.toml, suggest /init or offer to create one.",
}, " ")

var initPromptRe = regexp.MustCompile(`^/init(?:\s|$)`)

// IsInitPrompt reports whether prompt invokes the built-in /init command.
func IsInitPrompt(prompt string) bool {
	return initPromptRe.MatchString(strings.TrimLeft(prompt, " \t\n\r"))
}
