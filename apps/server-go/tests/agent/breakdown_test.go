// Request breakdown: per-source token estimates for system sections, tool
// definitions, and history buckets, plus the OnRequest hook per turn.
package tests

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/loop"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/permissions"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/tools"
	"github.com/Adelodunpeter25/console/apps/server-go/tests/helpers"
)

func TestBreakdownBuckets(t *testing.T) {
	sections := []loop.NamedText{
		{Name: "identity", Content: strings.Repeat("a", 40)},       // 10 tokens
		{Name: "workspace-tree", Content: strings.Repeat("b", 80)}, // 20 tokens
	}
	defs := []tools.Definition{{Name: "read_file", Description: "Read.", InputSchema: map[string]any{"type": "object"}}}
	history := []any{
		loop.UserMessage{Role: loop.RoleUser, Content: strings.Repeat("u", 40)},
		loop.AssistantMessage{Role: loop.RoleAssistant, Content: []any{
			loop.TextPart{Type: "text", Text: strings.Repeat("t", 8)},
			loop.ToolCallPart{Type: "toolCall", Call: tools.ToolCall{ID: "1", Name: "read_file", Arguments: json.RawMessage(`{"path":"x"}`)}},
		}},
		loop.ToolResultMessage{Role: loop.RoleToolResult, Results: []tools.ToolResult{
			{ToolCallID: "1", ToolName: "read_file", Content: strings.Repeat("r", 300)},
			{ToolCallID: "2", ToolName: "bash", Content: strings.Repeat("s", 30)},
		}},
		loop.UserMessage{Role: loop.RoleUser, Content: "[Conversation Checkpoint: Compacted 4 messages]"},
	}
	b := loop.EstimateBreakdown(sections, "ignored when sections are set", defs, history)
	if b.System["identity"] != 10 || b.System["workspace-tree"] != 20 || len(b.System) != 2 {
		t.Fatalf("system: %+v", b.System)
	}
	if b.Tools["read_file"] == 0 {
		t.Fatalf("tools: %+v", b.Tools)
	}
	if b.UserText != 10 || b.AssistantText != 2 || b.ToolCallArgs == 0 || b.Summaries == 0 {
		t.Fatalf("history: %+v", b)
	}
	if b.ToolResults["read_file"] != 100 || b.ToolResults["bash"] != 10 {
		t.Fatalf("tool results: %+v", b.ToolResults)
	}
	if b.Total() <= 150 {
		t.Fatalf("total: %d", b.Total())
	}
}

func TestBreakdownWholePromptFallback(t *testing.T) {
	b := loop.EstimateBreakdown(nil, strings.Repeat("x", 400), nil, nil)
	if b.System["system"] != 100 {
		t.Fatalf("fallback: %+v", b.System)
	}
}

func TestBreakdownHookFiresPerTurn(t *testing.T) {
	call := tools.ToolCall{ID: "c1", Name: "list_dir", Arguments: json.RawMessage(`{"path":"` + t.TempDir() + `"}`)}
	provider := &helpers.MockProvider{Turns: []func() []loop.Event{
		func() []loop.Event { return []loop.Event{{Kind: loop.EventToolCall, Call: &call}} },
		func() []loop.Event { return []loop.Event{{Kind: loop.EventText, Text: "done"}} },
	}}
	registry := tools.NewRegistry(tools.DefaultTools()...)
	agent := loop.New(provider, loop.NewExecutor(registry, permissions.FullAccess, nil), nil)
	agent.SystemPrompt = "system prompt"
	var got []loop.Breakdown
	agent.OnRequest = func(b loop.Breakdown) { got = append(got, b) }
	events, err := agent.Run(context.Background(), "s1", "go", registry.Definitions())
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err, ok := events.Next(); !ok {
			if err != nil {
				t.Fatal(err)
			}
			break
		}
	}
	if len(got) != 2 {
		t.Fatalf("hook calls: %d", len(got))
	}
	if got[0].ToolResults["list_dir"] != 0 || got[1].ToolResults["list_dir"] == 0 {
		t.Fatalf("second request must include the list_dir result: %+v / %+v", got[0].ToolResults, got[1].ToolResults)
	}
	if len(got[0].Tools) != len(registry.Definitions()) {
		t.Fatalf("tools: %d", len(got[0].Tools))
	}
}
