// Run usage accounting: per-turn TurnUsage summed across a run, cold-miss
// counting, and subagent rollup into the parent tracker.
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

func usageEvent(input, cacheRead, cacheWrite, output int) loop.Event {
	return loop.Event{Kind: loop.EventUsage, Usage: &loop.TurnUsage{
		Input: input, CacheRead: cacheRead, CacheWrite: cacheWrite, Output: output,
	}}
}

func TestRunUsageAggregatesTurns(t *testing.T) {
	call := tools.ToolCall{ID: "c1", Name: "list_dir", Arguments: json.RawMessage(`{"path":"` + t.TempDir() + `"}`)}
	provider := &helpers.MockProvider{Turns: []func() []loop.Event{
		func() []loop.Event {
			return []loop.Event{{Kind: loop.EventToolCall, Call: &call}, usageEvent(100, 0, 50, 10)}
		},
		// Second turn reads nothing from cache: a cold miss.
		func() []loop.Event {
			return []loop.Event{{Kind: loop.EventToolCall, Call: &call}, usageEvent(120, 0, 0, 5)}
		},
		func() []loop.Event {
			return []loop.Event{{Kind: loop.EventText, Text: "done"}, usageEvent(20, 150, 0, 7)}
		},
	}}
	registry := tools.NewRegistry(tools.DefaultTools()...)
	agent := loop.New(provider, loop.NewExecutor(registry, permissions.FullAccess, nil), nil)
	events, err := agent.Run(context.Background(), "s1", "go", registry.Definitions())
	if err != nil {
		t.Fatal(err)
	}
	var final *loop.RunUsage
	for {
		event, err, ok := events.Next()
		if !ok {
			if err != nil {
				t.Fatal(err)
			}
			break
		}
		if event.Kind == loop.EventTurnDone {
			final = event.RunUsage
		}
	}
	if final == nil {
		t.Fatal("turnDone carried no RunUsage")
	}
	want := loop.RunUsage{Turns: 3, Input: 240, CacheRead: 150, CacheWrite: 50, Output: 22, ColdMisses: 1}
	if *final != want {
		t.Fatalf("usage: got %+v want %+v", *final, want)
	}
}

func TestRunUsageCountsUnreportedTurns(t *testing.T) {
	var tracker loop.UsageTracker
	tracker.AddTurn(nil)
	tracker.AddTurn(&loop.TurnUsage{Input: 5})
	got := tracker.Snapshot()
	if got.Turns != 2 || got.UnreportedTurns != 1 || got.Input != 5 {
		t.Fatalf("usage: %+v", got)
	}
}

func TestRunUsageRollsUpSubagents(t *testing.T) {
	nested := &helpers.MockProvider{Turns: []func() []loop.Event{
		func() []loop.Event {
			return []loop.Event{{Kind: loop.EventText, Text: "found it"}, usageEvent(300, 0, 200, 40)}
		},
	}}
	parent := &loop.UsageTracker{}
	parent.AddTurn(&loop.TurnUsage{Input: 10, Output: 1})
	tool := loop.NewSubagentTool(&loop.SubagentContext{Provider: nested, Tools: tools.DefaultTools(), Usage: parent})
	if _, err := tool.(tools.CallAwareTool).ExecuteCall(context.Background(), subagentCall(t, "inspect")); err != nil {
		t.Fatal(err)
	}
	got := parent.Snapshot()
	if got.Turns != 1 || got.Input != 10 {
		t.Fatalf("parent own usage: %+v", got)
	}
	if got.Subagents == nil || got.Subagents.Turns != 1 || got.Subagents.Input != 300 || got.Subagents.CacheWrite != 200 {
		t.Fatalf("subagent usage: %+v", got.Subagents)
	}
	total := got.Total()
	if total.Turns != 2 || total.Input != 310 || total.Output != 41 || total.Subagents != nil {
		t.Fatalf("total: %+v", total)
	}
}
