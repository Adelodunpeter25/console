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

// recordingApprover counts prompts and records which tool asked.
type recordingApprover struct {
	calls int
	tools []string
}

func (r *recordingApprover) Approve(_ context.Context, req permissions.Request) (bool, error) {
	r.calls++
	r.tools = append(r.tools, req.ToolName)
	return true, nil
}

// execTool returns a TierExec tool that records that it ran.
func execTool(t *testing.T, ran *bool) tools.Tool {
	t.Helper()
	return tools.NewTool("dangerous_exec", "An exec-tier tool.", tools.TierExec,
		func(_ context.Context, _ struct{}) (any, error) {
			*ran = true
			return []map[string]any{{"type": "text", "text": "done"}}, nil
		})
}

// TestSubagentPromptsWhenParentModePrompts proves the mode is inherited: under
// always-ask an exec-tier tool in a subagent must reach the approver.
func TestSubagentPromptsWhenParentModePrompts(t *testing.T) {
	var ran bool
	approver := &recordingApprover{}
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
		ApprovalMode: permissions.AlwaysAsk,
		Approver:     approver,
	})
	if _, err := tool.(tools.CallAwareTool).ExecuteCall(context.Background(), subagentCall(t, "go")); err != nil {
		t.Fatalf("subagent call: %v", err)
	}
	if approver.calls != 1 {
		t.Fatalf("approver called %d times, want 1 (subagent bypassed the parent mode)", approver.calls)
	}
	if len(approver.tools) != 1 || approver.tools[0] != "dangerous_exec" {
		t.Fatalf("prompted for %v, want dangerous_exec", approver.tools)
	}
	if !ran {
		t.Fatal("approved exec tool did not run")
	}
}

// TestSubagentRunsUnattendedUnderFullAccess is the other half: a permissive parent
// means no prompt, so computer-use work can complete without a human.
func TestSubagentRunsUnattendedUnderFullAccess(t *testing.T) {
	var ran bool
	approver := &recordingApprover{}
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
		Approver:     approver,
	})
	if _, err := tool.(tools.CallAwareTool).ExecuteCall(context.Background(), subagentCall(t, "go")); err != nil {
		t.Fatalf("subagent call: %v", err)
	}
	if approver.calls != 0 {
		t.Fatalf("approver called %d times under full-access, want 0", approver.calls)
	}
	if !ran {
		t.Fatal("exec tool did not run under full-access")
	}
}

// TestSubagentDeniedToolIsNotRetriedSilently: with no approver configured and a
// prompting mode, the subagent cannot escalate to full-access on its own.
func TestSubagentCannotEscalateWithoutApprover(t *testing.T) {
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
		ApprovalMode: permissions.AlwaysAsk,
		// Approver deliberately nil.
	})
	out, err := tool.(tools.CallAwareTool).ExecuteCall(context.Background(), subagentCall(t, "go"))
	if err == nil && ran {
		t.Fatalf("subagent ran an exec tool with no approver available (output %v)", out)
	}
}

// TestSubagentDefaultsToAlwaysAsk: an unset mode must not be treated as
// full-access. This is the regression guard for the original bug.
func TestSubagentDefaultsToAlwaysAsk(t *testing.T) {
	if got := permissions.Resolve("", tools.TierExec); got != permissions.Prompt {
		t.Fatalf("empty mode resolves to %v, want prompt", got)
	}
}
