// subagent tool (static registration). The model-facing shape from
// apps/server/agent/src/tools/subagent.ts. This instance is simulated
// (no nested run); the run service replaces it with the loop-bound real
// one per turn, the same way ask/askMany/memory get per-run handlers.
package tools

import (
	"context"
	"fmt"
)

type subagentInput struct {
	Prompt string `json:"prompt" jsonschema:"required,description=Clear, actionable task description for the subagent"`
	Name   string `json:"name" jsonschema:"required,description=Name or identifier for the subagent to help differentiate it from other instances"`
	Role   string `json:"role,omitempty" jsonschema:"description=Role or job title for the subagent (e.g. 'Codebase Inspector', 'Test Runner')"`
}

// Subagent is the headless default-answer instance used by DefaultTools();
// a run with a provider replaces it with the loop-bound implementation.
var Subagent = NewTool("subagent", "Delegate a focused sub-task to an isolated subagent.", TierRead,
	func(ctx context.Context, in subagentInput) (any, error) {
		displayName := in.Name
		if displayName == "" {
			displayName = in.Role
		}
		return fmt.Sprintf("Subagent [%s] simulated run for: %q\n(No active provider attached to task tool context)", displayName, in.Prompt), nil
	})
