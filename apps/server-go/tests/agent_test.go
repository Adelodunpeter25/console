// Agent core coverage: tool schemas, registry, permissions, executor, loop.
package tests

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/loop"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/permissions"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/stream"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/tools"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/db"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/fff"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/services"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/types"
)

func TestToolSchemaFromTags(t *testing.T) {
	registry := tools.NewRegistry(tools.DefaultTools()...)
	defs := registry.Definitions()
	if len(defs) != 16 {
		t.Fatalf("definitions: %d", len(defs))
	}

	// read_file: required path, properties present.
	readFile := findDef(t, defs, "read_file")
	if readFile.InputSchema["type"] != "object" {
		t.Fatalf("schema type: %v", readFile.InputSchema["type"])
	}
	props := readFile.InputSchema["properties"].(map[string]any)
	if _, ok := props["path"]; !ok {
		t.Fatal("path property missing")
	}
	required := readFile.InputSchema["required"].([]any)
	if len(required) != 1 || required[0] != "path" {
		t.Fatalf("required: %v", required)
	}
}

func findDef(t *testing.T, defs []tools.Definition, name string) tools.Definition {
	t.Helper()
	for _, d := range defs {
		if d.Name == name {
			return d
		}
	}
	t.Fatalf("tool %s not found", name)
	return tools.Definition{}
}

func TestToolValidation(t *testing.T) {
	// Missing required arg -> tool error, not a panic.
	if _, err := tools.ReadFile.Execute(context.Background(), json.RawMessage(`{"startLine": 2}`)); err == nil {
		t.Fatal("expected error for missing path")
	}

	// Valid args execute with typed input and line slicing.
	dir := t.TempDir()
	p := filepath.Join(dir, "f.txt")
	if err := os.WriteFile(p, []byte("l1\nl2\nl3"), 0o644); err != nil {
		t.Fatal(err)
	}
	valid, _ := json.Marshal(map[string]any{"path": p, "startLine": 2, "endLine": 3})
	out, err := tools.ReadFile.Execute(context.Background(), valid)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	raw, err := json.Marshal(out)
	if err != nil {
		t.Fatalf("marshal output: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("unmarshal output: %v", err)
	}
	content := m["content"].(string)
	if content != "l2\nl3" {
		t.Fatalf("sliced content: %q", content)
	}
}

// TestGlobGrepFallback exercises the walk-based path (no fff manager wired),
// which always runs regardless of the environment.
func TestGlobGrepFallback(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "b.txt"), []byte("hello world\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	globArgs, _ := json.Marshal(map[string]any{"pattern": filepath.Join(dir, "*.go")})
	globOut, err := tools.Glob.Execute(context.Background(), globArgs)
	if err != nil {
		t.Fatalf("glob execute: %v", err)
	}
	matches, ok := globOut.([]string)
	if !ok || len(matches) != 1 || !strings.HasSuffix(matches[0], "a.go") {
		t.Fatalf("glob fallback matches: %#v", globOut)
	}

	grepArgs, _ := json.Marshal(map[string]any{"pattern": "hello", "root": dir})
	grepOut, err := tools.Grep.Execute(context.Background(), grepArgs)
	if err != nil {
		t.Fatalf("grep execute: %v", err)
	}
	if grepOut == nil {
		t.Fatal("grep fallback returned nil")
	}
}

// TestGlobGrepFff verifies glob/grep go through the real fff C ABI when the
// shared library is available. Skips (rather than fails) when FFF_LIB_PATH
// is unset, matching the rest of the fff integration's opt-in test pattern.
func TestGlobGrepFff(t *testing.T) {
	if os.Getenv("FFF_LIB_PATH") == "" {
		t.Skip("FFF_LIB_PATH not set")
	}
	manager := fff.NewManager()
	if !manager.Enabled() {
		t.Fatal("fff manager not enabled with FFF_LIB_PATH set")
	}
	tools.SetFffManager(manager)
	t.Cleanup(manager.CloseAll)
	t.Cleanup(func() { tools.SetFffManager(nil) })

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "b.txt"), []byte("needle here\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	globArgs, _ := json.Marshal(map[string]any{"pattern": "*.go", "root": dir})
	globOut, err := tools.Glob.Execute(context.Background(), globArgs)
	if err != nil {
		t.Fatalf("glob execute: %v", err)
	}
	matches, ok := globOut.([]string)
	if !ok || len(matches) != 1 || matches[0] != "a.go" {
		t.Fatalf("fff glob matches: %#v", globOut)
	}

	grepArgs, _ := json.Marshal(map[string]any{"pattern": "needle", "root": dir, "mode": "plain"})
	grepOut, err := tools.Grep.Execute(context.Background(), grepArgs)
	if err != nil {
		t.Fatalf("grep execute: %v", err)
	}
	if grepOut == nil {
		t.Fatal("fff grep returned nil")
	}
}

func mustJSONRaw(t *testing.T, v any) json.RawMessage {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func TestPermissionMatrix(t *testing.T) {
	cases := []struct {
		mode permissions.Mode
		tier tools.ToolTier
		want permissions.Policy
	}{
		{permissions.FullAccess, tools.TierExec, permissions.Allow},
		{permissions.AcceptEdits, tools.TierWrite, permissions.Allow},
		{permissions.AcceptEdits, tools.TierExec, permissions.Prompt},
		{permissions.PlanMode, tools.TierExec, permissions.Deny},
		{permissions.PlanMode, tools.TierWrite, permissions.Deny},
		{permissions.AlwaysAsk, tools.TierRead, permissions.Allow},
		{permissions.AlwaysAsk, tools.TierWrite, permissions.Prompt},
	}
	for _, c := range cases {
		if got := permissions.Resolve(c.mode, c.tier); got != c.want {
			t.Errorf("%s/%s: got %s want %s", c.mode, c.tier, got, c.want)
		}
	}
}

// mockProvider replays scripted turns; used to drive the loop without a
// real model backend (Phase 3 plugs in behind the same interface).
type mockProvider struct {
	turns []func() []loop.Event
	calls int
}

func (m *mockProvider) RunTurn(ctx context.Context, req loop.TurnRequest, s *stream.Stream[loop.Event]) error {
	script := m.turns[min(m.calls, len(m.turns)-1)]
	m.calls++
	for _, event := range script() {
		s.Push(event)
	}
	s.Complete()
	return nil
}

type autoApprover struct{}

func (autoApprover) Approve(ctx context.Context, req permissions.Request) (bool, error) {
	return true, nil
}

func TestAgentLoopToolRoundTrip(t *testing.T) {
	manager, err := db.Open(db.OpenOptions{Path: ":memory:"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(manager.Close)
	sessions := services.NewSessionService(manager)
	header, err := sessions.Create(types.CreateSessionOptions{
		Cwd: "/tmp/agent-test", ModelID: "mock", Provider: "mock",
	})
	if err != nil {
		t.Fatal(err)
	}

	// Turn 1: assistant asks to write a file. Turn 2: model finishes.
	provider := &mockProvider{turns: []func() []loop.Event{
		func() []loop.Event {
			return []loop.Event{
				{Kind: loop.EventText, Text: "writing"},
				{Kind: loop.EventToolCall, Call: &tools.ToolCall{
					ID: "c1", Name: "write_file",
					Arguments: mustJSONRaw(t, map[string]any{
						"path": "/tmp/agent-test-out/hello.txt", "content": "hi",
					}),
				}},
			}
		},
		func() []loop.Event {
			return []loop.Event{{Kind: loop.EventText, Text: "done"}}
		},
	}}
	registry := tools.NewRegistry(tools.DefaultTools()...)
	executor := loop.NewExecutor(registry, permissions.FullAccess, autoApprover{})
	agent := loop.New(provider, executor, sessions)

	events, err := agent.Run(context.Background(), header.ID, "please write hi", registry.Definitions())
	if err != nil {
		t.Fatal(err)
	}
	var sawToolResult, sawDone bool
	for {
		event, err, ok := events.Next()
		if !ok {
			if err != nil {
				t.Fatalf("stream error: %v", err)
			}
			break
		}
		switch event.Kind {
		case loop.EventToolResult:
			sawToolResult = true
			if event.Result.IsError {
				t.Fatalf("tool errored: %v", event.Result.Content)
			}
		case loop.EventTurnDone:
			sawDone = true
		}
	}
	if !sawToolResult || !sawDone {
		t.Fatalf("sawToolResult=%v sawDone=%v", sawToolResult, sawDone)
	}

	// The file was actually written by the tool executor.
	data, err := os.ReadFile("/tmp/agent-test-out/hello.txt")
	if err != nil || string(data) != "hi" {
		t.Fatalf("file: %v %q", err, data)
	}

	// Session persisted: user, assistant(toolUse), toolResult, assistant(stop).
	loaded, err := sessions.Load(header.ID, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Messages) != 4 {
		t.Fatalf("persisted messages: %d", len(loaded.Messages))
	}
}

func TestStreamNoEventLoss(t *testing.T) {
	// Producer outpacing consumer must not skip events (the TS bug fix).
	s := stream.New[int]()
	received := make(chan int, 100)
	go func() {
		for {
			v, err, ok := s.Next()
			if !ok {
				close(received)
				return
			}
			_ = err
			received <- v
		}
	}()
	for i := 0; i < 50; i++ {
		s.Push(i)
	}
	s.Complete()
	count := 0
	for v := range received {
		if v != count {
			t.Fatalf("skipped event: got %d want %d", v, count)
		}
		count++
	}
	if count != 50 {
		t.Fatalf("received %d/50", count)
	}
}

func TestExecutorDeniesInPlanMode(t *testing.T) {
	registry := tools.NewRegistry(tools.WriteFile)
	executor := loop.NewExecutor(registry, permissions.PlanMode, autoApprover{})
	result, err := executor.Execute(context.Background(), tools.ToolCall{
		ID: "c1", Name: "write_file",
		Arguments: mustJSONRaw(t, map[string]any{"path": "/tmp/x.txt", "content": "y"}),
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Fatal("write_file must be denied in plan mode")
	}
}
