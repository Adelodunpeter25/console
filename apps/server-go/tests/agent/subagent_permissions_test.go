// Subagents inherit the parent run's permission mode. They receive the parent's
// tools, so they must not hold more authority over those tools than the run that
// spawned them. Previously the subagent executor was hard-coded to full-access.
package tests

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/loop"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/permissions"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/tools"
	"github.com/Adelodunpeter25/console/apps/server-go/tests/helpers"
)

// execTool returns a TierExec tool that records that it ran.
func execTool(t *testing.T, ran *bool) tools.Tool {
	t.Helper()
	return tools.NewTool("dangerous_exec", "An exec-tier tool.", tools.TierExec,
		func(_ context.Context, _ struct{}) (any, error) {
			*ran = true
			return []map[string]any{{"type": "text", "text": "done"}}, nil
		})
}

// TestSubagentInheritsPlanModeDenial proves the mode is inherited: under plan
// mode an exec-tier tool in a subagent is refused, never run and never asked.
func TestSubagentInheritsPlanModeDenial(t *testing.T) {
	var ran bool
	provider := &helpers.MockProvider{Turns: []func() []loop.Event{
		func() []loop.Event {
			args, _ := json.Marshal(map[string]any{})
			call := tools.ToolCall{ID: "inner", Name: "dangerous_exec", Arguments: args}
			return []loop.Event{{Kind: loop.EventToolCall, Call: &call}}
		},
		func() []loop.Event { return []loop.Event{{Kind: loop.EventText, Text: "done"}} },
	}}
	tool := loop.NewSubagentTool(&loop.SubagentContext{
		Provider:     provider,
		Tools:        []tools.Tool{execTool(t, &ran)},
		ApprovalMode: permissions.PlanMode,
	})
	_, _ = tool.(tools.CallAwareTool).ExecuteCall(context.Background(), subagentCall(t, "go"))
	if ran {
		t.Fatal("exec tool ran in a plan-mode subagent")
	}
}

// TestSubagentRunsUnattendedUnderFullAccess is the other half: a permissive parent
// means no prompt, so computer-use work can complete without a human.
func TestSubagentRunsUnattendedUnderFullAccess(t *testing.T) {
	var ran bool
	provider := &helpers.MockProvider{Turns: []func() []loop.Event{
		func() []loop.Event {
			args, _ := json.Marshal(map[string]any{})
			call := tools.ToolCall{ID: "inner", Name: "dangerous_exec", Arguments: args}
			return []loop.Event{{Kind: loop.EventToolCall, Call: &call}}
		},
		func() []loop.Event { return []loop.Event{{Kind: loop.EventText, Text: "done"}} },
	}}
	tool := loop.NewSubagentTool(&loop.SubagentContext{
		Provider:     provider,
		Tools:        []tools.Tool{execTool(t, &ran)},
		ApprovalMode: permissions.FullAccess,
	})
	if _, err := tool.(tools.CallAwareTool).ExecuteCall(context.Background(), subagentCall(t, "go")); err != nil {
		t.Fatalf("subagent call: %v", err)
	}
	if !ran {
		t.Fatal("exec tool did not run under full-access")
	}
}

// TestSubagentDefaultsToPlan: an unset mode must not be treated as
// full-access. This is the regression guard for the original bug.
func TestSubagentDefaultsToPlan(t *testing.T) {
	if got := permissions.Resolve(permissions.Normalize(""), tools.TierExec); got != permissions.Deny {
		t.Fatalf("empty mode resolves to %v, want deny", got)
	}
}
