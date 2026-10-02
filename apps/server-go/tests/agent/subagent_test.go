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
	text := resultText(t, out)
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
	if text := resultText(t, out); !strings.Contains(text, "simulated") {
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
	if text := resultText(t, out); !strings.Contains(text, "simulated") {
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

type modelCapture struct{ model string }

func (m *modelCapture) RunTurn(ctx context.Context, req loop.TurnRequest, s *stream.Stream[loop.Event]) error {
	m.model = req.Model
	s.Push(loop.Event{Kind: loop.EventText, Text: "ok"})
	s.Complete()
	return nil
}

func TestSubagentInheritsModel(t *testing.T) {
	p := &modelCapture{}
	tool := loop.NewSubagentTool(&loop.SubagentContext{Provider: p, Model: "claude-haiku-4-5", Tools: tools.DefaultTools()})
	if _, err := tool.(tools.CallAwareTool).ExecuteCall(context.Background(), subagentCall(t, "inspect")); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if p.model != "claude-haiku-4-5" {
		t.Fatalf("nested model = %q, want claude-haiku-4-5", p.model)
	}
}

type toolNameCapture struct {
	names []string
	descs map[string]string
}

func (c *toolNameCapture) RunTurn(ctx context.Context, req loop.TurnRequest, s *stream.Stream[loop.Event]) error {
	c.names = c.names[:0]
	c.descs = map[string]string{}
	for _, d := range req.Tools {
		c.names = append(c.names, d.Name)
		c.descs[d.Name] = d.Description
	}
	s.Push(loop.Event{Kind: loop.EventText, Text: "ok"})
	s.Complete()
	return nil
}

func TestSubagentToolSourceAndNestedTools(t *testing.T) {
	extra := tools.NewTool("mcp__srv__get_x", "lazily loaded", tools.TierRead,
		func(ctx context.Context, in struct{}) (any, error) { return "x", nil })
	staleLoader := tools.NewTool("loadTools", "parent-bound", tools.TierRead,
		func(ctx context.Context, in struct{}) (any, error) { return "parent", nil })
	freshLoader := tools.NewTool("loadTools", "nested-bound", tools.TierRead,
		func(ctx context.Context, in struct{}) (any, error) { return "nested", nil })
	parent := tools.NewRegistry(tools.DefaultTools()...)
	parent.Add(staleLoader, extra)

	p := &toolNameCapture{}
	tool := loop.NewSubagentTool(&loop.SubagentContext{
		Provider:   p,
		Model:      "m",
		ToolSource: parent.Tools,
		NestedTools: func(r *tools.Registry) {
			r.Add(freshLoader)
		},
	})
	if _, err := tool.(tools.CallAwareTool).ExecuteCall(context.Background(), subagentCall(t, "go")); err != nil {
		t.Fatal(err)
	}
	has := func(name string) bool {
		for _, n := range p.names {
			if n == name {
				return true
			}
		}
		return false
	}
	if !has("mcp__srv__get_x") || !has("loadTools") || has("subagent") {
		t.Fatalf("nested tools: %v", p.names)
	}
	if p.descs["loadTools"] != "nested-bound" {
		t.Fatalf("loadTools must be re-bound to the nested registry, got %q", p.descs["loadTools"])
	}
}

func TestRegistryToolsOrder(t *testing.T) {
	a := tools.NewTool("b_tool", "", tools.TierRead, func(ctx context.Context, in struct{}) (any, error) { return nil, nil })
	b := tools.NewTool("a_tool", "", tools.TierRead, func(ctx context.Context, in struct{}) (any, error) { return nil, nil })
	late := tools.NewTool("0_late", "", tools.TierRead, func(ctx context.Context, in struct{}) (any, error) { return nil, nil })
	r := tools.NewRegistry(a, b)
	r.Add(late)
	got := []string{}
	for _, tl := range r.Tools() {
		got = append(got, tl.Name())
	}
	defs := r.Definitions()
	if strings.Join(got, ",") != "a_tool,b_tool,0_late" || len(defs) != 3 || defs[2].Name != "0_late" {
		t.Fatalf("order: %v", got)
	}
}
