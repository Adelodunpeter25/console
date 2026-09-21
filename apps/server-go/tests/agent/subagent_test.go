// Subagent tool coverage: nested completion with lifecycle events,
// simulated headless response, abort, and exclusion from nested tools.
package tests

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/loop"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/stream"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/tools"
	"github.com/Adelodunpeter25/console/apps/server-go/tests/helpers"
)

func subagentCall(t *testing.T, prompt string) tools.ToolCall {
	t.Helper()
	raw, err := json.Marshal(map[string]any{"prompt": prompt, "name": "scout"})
	if err != nil {
		t.Fatal(err)
	}
	return tools.ToolCall{ID: "sub1", Name: "subagent", Arguments: raw}
}

func TestSubagentCompletes(t *testing.T) {
	nested := &helpers.MockProvider{Turns: []func() []loop.Event{
		func() []loop.Event { return []loop.Event{{Kind: loop.EventText, Text: "found it"}} },
	}}
	var kinds []loop.EventKind
	tool := loop.NewSubagentTool(&loop.SubagentContext{
		Provider: nested,
		Tools:    tools.DefaultTools(),
		OnEvent: func(e loop.Event) {
			kinds = append(kinds, e.Kind)
		},
	})
	out, err := tool.(tools.CallAwareTool).ExecuteCall(context.Background(), subagentCall(t, "inspect"))
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	text, _ := out.(string)
	if !strings.Contains(text, "found it") || !strings.Contains(text, "Completed Task") {
		t.Fatalf("summary: %q", text)
	}
	seen := map[loop.EventKind]bool{}
	for _, k := range kinds {
		seen[k] = true
	}
	for _, want := range []loop.EventKind{loop.EventSubagentStart, loop.EventSubagentEnd} {
		if !seen[want] {
			t.Fatalf("missing %s in %v", want, kinds)
		}
	}
}

func TestSubagentSimulated(t *testing.T) {
	tool := loop.NewSubagentTool(nil)
	out, err := tool.(tools.CallAwareTool).ExecuteCall(context.Background(), subagentCall(t, "inspect"))
	if err != nil {
		t.Fatalf("simulated must not error: %v", err)
	}
	if text, _ := out.(string); !strings.Contains(text, "simulated") {
		t.Fatalf("simulated: %q", text)
	}
}

func TestSubagentAborted(t *testing.T) {
	release := make(chan struct{})
	entered := make(chan struct{})
	nested := &blockingProvider{release: release, entered: entered}
	tool := loop.NewSubagentTool(&loop.SubagentContext{Provider: nested})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	var out any
	var runErr error
	go func() {
		defer close(done)
		out, runErr = tool.(tools.CallAwareTool).ExecuteCall(ctx, subagentCall(t, "slow"))
	}()
	select {
	case <-entered:
	case <-time.After(10 * time.Second):
		t.Fatal("nested provider never entered")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("subagent did not abort")
	}
	if runErr == nil {
		t.Fatal("aborted subagent must error")
	}
	_ = out
}

type recordingProvider struct {
	mock  *helpers.MockProvider
	names [][]string
}

func (r *recordingProvider) RunTurn(ctx context.Context, req loop.TurnRequest, s *stream.Stream[loop.Event]) error {
	var names []string
	for _, d := range req.Tools {
		names = append(names, d.Name)
	}
	r.names = append(r.names, names)
	return r.mock.RunTurn(ctx, req, s)
}

func TestSubagentInDefaultTools(t *testing.T) {
	registry := tools.NewRegistry(tools.DefaultTools()...)
	tool, err := registry.Get("subagent")
	if err != nil {
		t.Fatalf("subagent must be registered: %v", err)
	}
	raw, _ := json.Marshal(map[string]any{"prompt": "hi", "name": "scout"})
	out, err := tool.Execute(context.Background(), raw)
	if err != nil {
		t.Fatalf("simulated subagent must not error: %v", err)
	}
	if text, _ := out.(string); !strings.Contains(text, "simulated") {
		t.Fatalf("simulated: %q", text)
	}
}

func TestSubagentExcludedFromNested(t *testing.T) {
	rec := &recordingProvider{mock: &helpers.MockProvider{Turns: []func() []loop.Event{
		func() []loop.Event { return []loop.Event{{Kind: loop.EventText, Text: "ok"}} },
	}}}
	// Mirror run.go: nested tools are the parent list including the bound
	// subagent instance itself; the tool must filter itself out.
	parent := append(tools.DefaultTools(), loop.NewSubagentTool(&loop.SubagentContext{Provider: rec}))
	tool := loop.NewSubagentTool(&loop.SubagentContext{Provider: rec, Tools: parent})
	if _, err := tool.(tools.CallAwareTool).ExecuteCall(context.Background(), subagentCall(t, "go")); err != nil {
		t.Fatal(err)
	}
	if len(rec.names) == 0 {
		t.Fatal("nested turn never ran")
	}
	for _, name := range rec.names[0] {
		if name == "subagent" {
			t.Fatalf("nested tools must exclude subagent: %v", rec.names[0])
		}
	}
}
