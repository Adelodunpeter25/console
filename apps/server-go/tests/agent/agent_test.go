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
	"github.com/Adelodunpeter25/console/apps/server-go/tests/helpers"
)

type contextRecordingProvider struct {
	request loop.TurnRequest
}

func (p *contextRecordingProvider) RunTurn(
	ctx context.Context,
	request loop.TurnRequest,
	events *stream.Stream[loop.Event],
) error {
	p.request = request
	events.Push(loop.Event{Kind: loop.EventText, Text: "done"})
	events.Complete()
	return nil
}

// resultText extracts the text of the first MCP content block from a
// tool's successful return value, unwrapping tools.Envelope when present.
func resultText(t *testing.T, out any) string {
	t.Helper()
	content := out
	if env, ok := out.(tools.Envelope); ok {
		content = env.Content
	}
	blocks, ok := content.([]map[string]any)
	if !ok || len(blocks) == 0 {
		t.Fatalf("expected MCP content array, got %#v", out)
	}
	text, _ := blocks[0]["text"].(string)
	return text
}

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
	// Wire shape must be the MCP-style content array the TS server sends
	// (tool-output.ts normalizeToolOutput), not a raw Go struct — the
	// desktop UI's read-file renderer parses this exact "File: ...\n
	// Showing: ...\n\n N: line" text format.
	var blocks []map[string]any
	if err := json.Unmarshal(raw, &blocks); err != nil {
		t.Fatalf("unmarshal output: %v", err)
	}
	if len(blocks) != 1 || blocks[0]["type"] != "text" {
		t.Fatalf("content blocks: %+v", blocks)
	}
	text := blocks[0]["text"].(string)
	if !strings.HasPrefix(text, "File: "+p+"\n") {
		t.Fatalf("missing File: header: %q", text)
	}
	if !strings.Contains(text, "2: l2\n3: l3") {
		t.Fatalf("sliced numbered content: %q", text)
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
	globText := resultText(t, globOut)
	if !strings.Contains(globText, "Found 1 file(s)") || !strings.HasSuffix(strings.TrimSpace(globText), "a.go") {
		t.Fatalf("glob fallback matches: %#v", globOut)
	}

	grepArgs, _ := json.Marshal(map[string]any{"pattern": "hello", "root": dir})
	grepOut, err := tools.Grep.Execute(context.Background(), grepArgs)
	if err != nil {
		t.Fatalf("grep execute: %v", err)
	}
	grepRaw, err := json.Marshal(grepOut)
	if err != nil {
		t.Fatalf("marshal grep output: %v", err)
	}
	var grepBlocks []map[string]any
	if err := json.Unmarshal(grepRaw, &grepBlocks); err != nil {
		t.Fatalf("grep output must be an MCP content array: %v (%s)", err, grepRaw)
	}
	grepText := grepBlocks[0]["text"].(string)
	if !strings.Contains(grepText, "Found 1 match(es)") || !strings.Contains(grepText, "→    1: hello world") {
		t.Fatalf("grep result text: %q", grepText)
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
	globText := resultText(t, globOut)
	if !strings.Contains(globText, "Found 1 file(s)") || !strings.HasSuffix(strings.TrimSpace(globText), "a.go") {
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
	provider := &helpers.MockProvider{Turns: []func() []loop.Event{
		func() []loop.Event {
			return []loop.Event{
				{Kind: loop.EventText, Text: "writing"},
				{Kind: loop.EventToolCall, Call: &tools.ToolCall{
					ID: "c1", Name: "write_file",
					Arguments: helpers.MustJSONRaw(t, map[string]any{
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
	executor := loop.NewExecutor(registry, permissions.FullAccess, helpers.AutoApprover{})
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

func TestContextFilesMaterializeForAgentButPersistClean(t *testing.T) {
	manager, err := db.Open(db.OpenOptions{Path: ":memory:"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(manager.Close)
	sessions := services.NewSessionService(manager)
	header, err := sessions.Create(types.CreateSessionOptions{
		Cwd: "/tmp/context-files-test", ModelID: "mock", Provider: "mock",
	})
	if err != nil {
		t.Fatal(err)
	}

	provider := &contextRecordingProvider{}
	registry := tools.NewRegistry()
	executor := loop.NewExecutor(registry, permissions.FullAccess, helpers.AutoApprover{})
	agent := loop.New(provider, executor, sessions)
	user := loop.UserMessage{
		Role:         loop.RoleUser,
		Content:      "inspect the selected context",
		ContextFiles: []string{"apps/mobile", "README.md"},
	}
	events, err := agent.RunWithHistory(context.Background(), header.ID, nil, user, registry.Definitions())
	if err != nil {
		t.Fatal(err)
	}
	for {
		_, eventErr, ok := events.Next()
		if !ok {
			if eventErr != nil {
				t.Fatalf("stream error: %v", eventErr)
			}
			break
		}
	}

	if len(provider.request.Messages) != 1 {
		t.Fatalf("provider history length = %d, want 1", len(provider.request.Messages))
	}
	materialized, ok := provider.request.Messages[0].(loop.UserMessage)
	if !ok {
		t.Fatalf("provider message type = %T", provider.request.Messages[0])
	}
	if !strings.Contains(materialized.Content, "apps/mobile") || !strings.Contains(materialized.Content, "README.md") {
		t.Fatalf("provider did not receive context paths: %q", materialized.Content)
	}

	loaded, err := sessions.Load(header.ID, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	var persisted loop.UserMessage
	if err := json.Unmarshal(loaded.Messages[0], &persisted); err != nil {
		t.Fatal(err)
	}
	if persisted.Content != user.Content {
		t.Fatalf("persisted content = %q, want clean %q", persisted.Content, user.Content)
	}
	if len(persisted.ContextFiles) != 2 || persisted.ContextFiles[0] != "apps/mobile" || persisted.ContextFiles[1] != "README.md" {
		t.Fatalf("persisted context files = %v", persisted.ContextFiles)
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
	executor := loop.NewExecutor(registry, permissions.PlanMode, helpers.AutoApprover{})
	result, err := executor.Execute(context.Background(), tools.ToolCall{
		ID: "c1", Name: "write_file",
		Arguments: helpers.MustJSONRaw(t, map[string]any{"path": "/tmp/x.txt", "content": "y"}),
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Fatal("write_file must be denied in plan mode")
	}
}
