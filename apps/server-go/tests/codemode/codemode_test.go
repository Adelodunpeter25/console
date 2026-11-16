package tests

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/codemode"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/loop"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/permissions"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/tools"
)

func newTestExecutor(t *testing.T, tmpDir string) *loop.Executor {
	t.Helper()
	reg := tools.NewRegistry(tools.DefaultTools()...)
	exec := loop.NewExecutor(reg, permissions.FullAccess)
	exec.Guard = permissions.NewGuard(tmpDir)
	return exec
}

func TestCodemodeBasicEvalAndPersistence(t *testing.T) {
	tmpDir := t.TempDir()
	exec := newTestExecutor(t, tmpDir)

	rt, err := codemode.NewRuntime(exec)
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}

	ctx := context.Background()

	// Variable assignment
	res, err := rt.Eval(ctx, "var counter = 40;", 5*time.Second)
	if err != nil {
		t.Fatalf("eval 1: %v", err)
	}
	env, ok := res.(tools.Envelope)
	if !ok || env.IsError {
		t.Fatalf("unexpected result: %#v", res)
	}

	// Read and increment across turns
	res2, err := rt.Eval(ctx, "counter += 2; counter;", 5*time.Second)
	if err != nil {
		t.Fatalf("eval 2: %v", err)
	}
	env2 := res2.(tools.Envelope)
	if env2.IsError || !strings.Contains(env2.Content.(string), "42") {
		t.Fatalf("expected 42 in output, got: %v", env2.Content)
	}
}

func TestCodemodeFilePrimitives(t *testing.T) {
	tmpDir := t.TempDir()
	exec := newTestExecutor(t, tmpDir)

	rt, err := codemode.NewRuntime(exec)
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}

	ctx := context.Background()
	testFile := filepath.Join(tmpDir, "sample.txt")

	// write() primitive
	writeScript := `await write(` + jsonString(testFile) + `, "hello world\nline 2");`
	res, err := rt.Eval(ctx, writeScript, 5*time.Second)
	if err != nil {
		t.Fatalf("write eval: %v", err)
	}
	if env := res.(tools.Envelope); env.IsError {
		t.Fatalf("write failed: %v", env.Content)
	}

	// Verify on disk
	data, err := os.ReadFile(testFile)
	if err != nil || string(data) != "hello world\nline 2" {
		t.Fatalf("expected file content on disk, got: %q, err: %v", string(data), err)
	}

	// file() primitive
	readScript := `await file(` + jsonString(testFile) + `);`
	res2, err := rt.Eval(ctx, readScript, 5*time.Second)
	if err != nil {
		t.Fatalf("file eval: %v", err)
	}
	if env := res2.(tools.Envelope); env.IsError || !strings.Contains(env.Content.(string), "hello world") {
		t.Fatalf("expected content in read output, got: %v", env.Content)
	}

	// edit() primitive
	editScript := `await edit(` + jsonString(testFile) + `, "hello world", "goodbye world");`
	res3, err := rt.Eval(ctx, editScript, 5*time.Second)
	if err != nil {
		t.Fatalf("edit eval: %v", err)
	}
	if env := res3.(tools.Envelope); env.IsError {
		t.Fatalf("edit failed: %v", env.Content)
	}

	// Verify updated on disk
	updated, err := os.ReadFile(testFile)
	if err != nil || !strings.HasPrefix(string(updated), "goodbye world") {
		t.Fatalf("expected edited file on disk, got: %q", string(updated))
	}
}

func TestCodemodeShellCommand(t *testing.T) {
	tmpDir := t.TempDir()
	exec := newTestExecutor(t, tmpDir)

	rt, err := codemode.NewRuntime(exec)
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}

	ctx := context.Background()

	// Tagged template execution with .text()
	script1 := "(await $`echo \"code mode shell\"`).text();"
	res1, err := rt.Eval(ctx, script1, 10*time.Second)
	if err != nil {
		t.Fatalf("shell text eval: %v", err)
	}
	env1 := res1.(tools.Envelope)
	if env1.IsError || !strings.Contains(env1.Content.(string), "code mode shell") {
		t.Fatalf("expected command output, got: %v", env1.Content)
	}

	// Function call $("cmd") with .text()
	scriptFunc := `(await $("echo 'function syntax'")).text();`
	resFunc, err := rt.Eval(ctx, scriptFunc, 10*time.Second)
	if err != nil {
		t.Fatalf("shell func eval: %v", err)
	}
	envFunc := resFunc.(tools.Envelope)
	if envFunc.IsError || !strings.Contains(envFunc.Content.(string), "function syntax") {
		t.Fatalf("expected function syntax command output, got: %v", envFunc.Content)
	}

	// JSON parsing with .json()
	script2 := "(await $`echo '{\"score\": 99}'`).json().score;"
	res2, err := rt.Eval(ctx, script2, 10*time.Second)
	if err != nil {
		t.Fatalf("shell json eval: %v", err)
	}
	env2 := res2.(tools.Envelope)
	if env2.IsError || !strings.Contains(env2.Content.(string), "99") {
		t.Fatalf("expected json value 99, got: %v", env2.Content)
	}

	// nothrow() suppression with structured metadata
	script3 := `
		const res = await $("false").nothrow();
		({ code: res.code, exitCode: res.exitCode, ok: res.ok, hasDuration: typeof res.durationMs === "number" });
	`
	res3, err := rt.Eval(ctx, script3, 10*time.Second)
	if err != nil {
		t.Fatalf("shell nothrow eval: %v", err)
	}
	env3 := res3.(tools.Envelope)
	if env3.IsError || !strings.Contains(env3.Content.(string), `"ok":false`) || !strings.Contains(env3.Content.(string), `"hasDuration":true`) {
		t.Fatalf("expected ok: false and durationMs without throw, got: %v", env3.Content)
	}

	// Custom options $({ cwd: ... })
	subDir := filepath.Join(tmpDir, "sub")
	if err := os.MkdirAll(subDir, 0o755); err != nil {
		t.Fatalf("mkdir sub: %v", err)
	}
	scriptCwd := `(await $({ cwd: ` + jsonString(subDir) + ` })("pwd")).text();`
	resCwd, err := rt.Eval(ctx, scriptCwd, 10*time.Second)
	if err != nil {
		t.Fatalf("shell cwd eval: %v", err)
	}
	envCwd := resCwd.(tools.Envelope)
	if envCwd.IsError || !strings.Contains(envCwd.Content.(string), "sub") {
		t.Fatalf("expected output with sub directory, got: %v", envCwd.Content)
	}
}

func TestCodemodeToolDiscoveryAndGenericCall(t *testing.T) {
	tmpDir := t.TempDir()
	exec := newTestExecutor(t, tmpDir)

	rt, err := codemode.NewRuntime(exec)
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}

	ctx := context.Background()

	// searchTools
	searchScript := `
		const matches = await searchTools("grep");
		matches.some(m => m.name === "grep");
	`
	res1, err := rt.Eval(ctx, searchScript, 5*time.Second)
	if err != nil {
		t.Fatalf("searchTools eval: %v", err)
	}
	env1 := res1.(tools.Envelope)
	if env1.IsError || !strings.Contains(env1.Content.(string), "true") {
		t.Fatalf("expected searchTools to find grep, got: %v", env1.Content)
	}

	// describeTool
	descScript := `
		const desc = await describeTool("grep");
		desc.name === "grep" && !!desc.parameters;
	`
	res2, err := rt.Eval(ctx, descScript, 5*time.Second)
	if err != nil {
		t.Fatalf("describeTool eval: %v", err)
	}
	env2 := res2.(tools.Envelope)
	if env2.IsError || !strings.Contains(env2.Content.(string), "true") {
		t.Fatalf("expected describeTool for grep, got: %v", env2.Content)
	}

	// listTools
	listToolsScript := `
		const all = await listTools();
		Array.isArray(all) && all.length > 5 && all.some(t => t.name === "bash");
	`
	res3, err := rt.Eval(ctx, listToolsScript, 5*time.Second)
	if err != nil {
		t.Fatalf("listTools eval: %v", err)
	}
	env3 := res3.(tools.Envelope)
	if env3.IsError || !strings.Contains(env3.Content.(string), "true") {
		t.Fatalf("expected listTools to return tool list, got: %v", env3.Content)
	}
}

func TestCodemodeReset(t *testing.T) {
	tmpDir := t.TempDir()
	exec := newTestExecutor(t, tmpDir)

	rt, err := codemode.NewRuntime(exec)
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}

	ctx := context.Background()

	_, _ = rt.Eval(ctx, "var secretValue = 12345;", 5*time.Second)

	// Call code_reset tool
	resetTool := codemode.NewCodeResetTool(rt)
	raw, _ := json.Marshal(map[string]any{})
	out, err := resetTool.Execute(ctx, raw)
	if err != nil {
		t.Fatalf("code_reset failed: %v", err)
	}
	if !strings.Contains(fmt.Sprint(out), "reset") {
		t.Fatalf("unexpected reset output: %v", out)
	}

	// Verify variable is gone
	res, _ := rt.Eval(ctx, "typeof secretValue;", 5*time.Second)
	env := res.(tools.Envelope)
	if !strings.Contains(env.Content.(string), "undefined") {
		t.Fatalf("expected secretValue to be undefined after reset, got: %v", env.Content)
	}
}

func TestCodemodeToolReplacementDefinitions(t *testing.T) {
	// Build registry with standard tools plus codemode tools
	reg := tools.NewRegistry(tools.DefaultTools()...)
	rt, err := codemode.NewRuntime(loop.NewExecutor(reg, permissions.FullAccess))
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}

	reg.Add(codemode.NewCodeTool(rt), codemode.NewCodeResetTool(rt))

	// Simulate the tool replacement filter used in turns.go
	filterDefs := func() []tools.Definition {
		var defs []tools.Definition
		for _, def := range reg.Definitions() {
			switch def.Name {
			case "code", "codeReset", "code_reset":
				defs = append(defs, def)
			}
		}
		return defs
	}

	filtered := filterDefs()
	if len(filtered) != 2 {
		t.Fatalf("expected exactly code and codeReset tools, got %d", len(filtered))
	}

	for _, d := range filtered {
		if d.Name != "code" && d.Name != "codeReset" && d.Name != "code_reset" {
			t.Fatalf("unexpected tool in filtered definitions: %s", d.Name)
		}
	}

	// Verify standard tools (e.g. read_file, bash) are still in the underlying registry
	if _, err := reg.Get("read_file"); err != nil {
		t.Fatalf("read_file should still be registered: %v", err)
	}
	if _, err := reg.Get("bash"); err != nil {
		t.Fatalf("bash should still be registered: %v", err)
	}
}

func TestCodemodeTodoPrimitive(t *testing.T) {
	tmpDir := t.TempDir()
	exec := newTestExecutor(t, tmpDir)

	rt, err := codemode.NewRuntime(exec)
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}

	ctx := context.Background()

	// Initialize todo list
	initScript := `await todo("init", ["first task", "second task"]);`
	res1, err := rt.Eval(ctx, initScript, 5*time.Second)
	if err != nil {
		t.Fatalf("todo init eval: %v", err)
	}
	env1 := res1.(tools.Envelope)
	if env1.IsError || !strings.Contains(env1.Content.(string), "first task") {
		t.Fatalf("expected initialized tasks, got: %v", env1.Content)
	}

	// Start task 1
	startScript := `await todo("start", 1);`
	res2, err := rt.Eval(ctx, startScript, 5*time.Second)
	if err != nil {
		t.Fatalf("todo start eval: %v", err)
	}
	env2 := res2.(tools.Envelope)
	if env2.IsError || !strings.Contains(env2.Content.(string), "in_progress") {
		t.Fatalf("expected in_progress task 1, got: %v", env2.Content)
	}

	// Done task 1
	doneScript := `await todo("done", 1);`
	res3, err := rt.Eval(ctx, doneScript, 5*time.Second)
	if err != nil {
		t.Fatalf("todo done eval: %v", err)
	}
	env3 := res3.(tools.Envelope)
	if env3.IsError || !strings.Contains(env3.Content.(string), "completed") {
		t.Fatalf("expected completed task 1, got: %v", env3.Content)
	}
}

func TestCodemodeWebPrimitivesWiring(t *testing.T) {
	tmpDir := t.TempDir()
	exec := newTestExecutor(t, tmpDir)

	rt, err := codemode.NewRuntime(exec)
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}

	ctx := context.Background()

	checkScript := `
		[
			typeof fetch,
			typeof webFetch,
			typeof webSearch,
			typeof todo,
			typeof visualize,
			typeof readChat,
			typeof readSkill,
			typeof writePlan,
			typeof ask,
			typeof askMany,
			typeof browser === "function",
			typeof browser.open === "function",
			typeof browser.snapshot === "function",
			typeof console.log === "function",
			typeof console.ports === "function",
			typeof console.ports.list === "function"
		].every(t => t === true || t === "function");
	`
	res, err := rt.Eval(ctx, checkScript, 5*time.Second)
	if err != nil {
		t.Fatalf("web primitives check eval: %v", err)
	}
	env := res.(tools.Envelope)
	if env.IsError || !strings.Contains(env.Content.(string), "true") {
		t.Fatalf("expected all primitives to be functions, got: %v", env.Content)
	}
}

type mockMethodTool struct {
	tools.Tool
	methods []string
}

func (m *mockMethodTool) Methods() []string {
	return m.methods
}

func TestCodemodeDescribeCapabilityTools(t *testing.T) {
	// Add browser and console tools implementing MethodDescriptor to registry
	mockBrowser := &mockMethodTool{
		Tool: tools.NewTool("browser", "Browser automation tool", tools.TierRead, func(ctx context.Context, in map[string]any) (any, error) {
			return "ok", nil
		}),
		methods: []string{"browser.open({url})", "browser.snapshot()"},
	}
	mockConsole := &mockMethodTool{
		Tool: tools.NewTool("console", "Console control tool", tools.TierRead, func(ctx context.Context, in map[string]any) (any, error) {
			return "ok", nil
		}),
		methods: []string{"console.ports.list()", "console.sessions.list()"},
	}

	reg := tools.NewRegistry(mockBrowser, mockConsole)
	rt, err := codemode.NewRuntime(loop.NewExecutor(reg, permissions.FullAccess))
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}

	ctx := context.Background()

	// Check describeTool for browser
	bScript := `
		const d = await describeTool("browser");
		d.name === "browser" && Array.isArray(d.methods) && d.methods.some(m => m.includes("browser.open"));
	`
	res1, err := rt.Eval(ctx, bScript, 5*time.Second)
	if err != nil {
		t.Fatalf("describe browser: %v", err)
	}
	env1 := res1.(tools.Envelope)
	if env1.IsError || !strings.Contains(env1.Content.(string), "true") {
		t.Fatalf("expected browser methods in describeTool, got: %v", env1.Content)
	}

	// Check describeTool for console
	cScript := `
		const d = await describeTool("console");
		d.name === "console" && Array.isArray(d.methods) && d.methods.some(m => m.includes("console.ports.list"));
	`
	res2, err := rt.Eval(ctx, cScript, 5*time.Second)
	if err != nil {
		t.Fatalf("describe console: %v", err)
	}
	env2 := res2.(tools.Envelope)
	if env2.IsError || !strings.Contains(env2.Content.(string), "true") {
		t.Fatalf("expected console methods in describeTool, got: %v", env2.Content)
	}
}

func TestCodemodeJSONUnwrapping(t *testing.T) {
	// A tool returning JSON string in textResult
	jsonTool := tools.NewTool("json_provider", "Provides JSON", tools.TierRead, func(ctx context.Context, in map[string]any) (any, error) {
		return []map[string]any{{"type": "text", "text": `{"success": true, "items": ["a", "b", "c"], "total": 3}`}}, nil
	})

	reg := tools.NewRegistry(jsonTool)
	rt, err := codemode.NewRuntime(loop.NewExecutor(reg, permissions.FullAccess))
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}

	ctx := context.Background()

	// Direct property access without JSON.parse()
	script := `
		const res = await tools.json_provider({});
		res.success === true && res.total === 3 && res.items.length === 3;
	`
	res, err := rt.Eval(ctx, script, 5*time.Second)
	if err != nil {
		t.Fatalf("eval json unwrapping: %v", err)
	}
	env := res.(tools.Envelope)
	if env.IsError || !strings.Contains(env.Content.(string), "true") {
		t.Fatalf("expected native object properties from json unwrapping, got: %v", env.Content)
	}
}

func TestCodemodePlanMode(t *testing.T) {
	tmpDir := t.TempDir()
	sampleFile := filepath.Join(tmpDir, "sample.txt")
	if err := os.WriteFile(sampleFile, []byte("read-only content"), 0o644); err != nil {
		t.Fatal(err)
	}

	reg := tools.NewRegistry(
		tools.DefaultTools()...,
	)
	reg.Add(tools.NewWritePlanTool(tmpDir))

	exec := loop.NewExecutor(reg, permissions.PlanMode)
	exec.Guard = permissions.NewGuard(tmpDir)

	rt, err := codemode.NewRuntime(exec)
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}

	ctx := context.Background()

	// 1. Reading file in plan mode succeeds
	readScript := `await file(` + jsonString(sampleFile) + `);`
	resRead, err := rt.Eval(ctx, readScript, 5*time.Second)
	if err != nil {
		t.Fatalf("plan mode read eval: %v", err)
	}
	envRead := resRead.(tools.Envelope)
	if envRead.IsError || !strings.Contains(envRead.Content.(string), "read-only content") {
		t.Fatalf("expected read success, got: %v", envRead.Content)
	}

	// 2. Writing a plan via writePlan succeeds in plan mode
	planScript := `await writePlan("test-plan", "# Test Plan\nEverything looks good.");`
	resPlan, err := rt.Eval(ctx, planScript, 5*time.Second)
	if err != nil {
		t.Fatalf("plan mode writePlan eval: %v", err)
	}
	envPlan := resPlan.(tools.Envelope)
	if envPlan.IsError || !strings.Contains(envPlan.Content.(string), "test-plan") {
		t.Fatalf("expected writePlan success, got: %v", envPlan.Content)
	}
	// Verify plan file on disk
	planPath := filepath.Join(tmpDir, "docs/plan/test-plan.md")
	if data, err := os.ReadFile(planPath); err != nil || !strings.Contains(string(data), "Test Plan") {
		t.Fatalf("expected plan file on disk at %s, got: %v (data: %s)", planPath, err, string(data))
	}

	// 3. Attempting to write or edit ordinary files in plan mode is denied
	writeScript := `await write(` + jsonString(filepath.Join(tmpDir, "denied.txt")) + `, "bad write");`
	resWrite, err := rt.Eval(ctx, writeScript, 5*time.Second)
	if err != nil {
		t.Fatalf("plan mode write eval: %v", err)
	}
	envWrite := resWrite.(tools.Envelope)
	if !envWrite.IsError || !strings.Contains(envWrite.Content.(string), "denied") {
		t.Fatalf("expected write to be denied in plan mode, got: %v", envWrite.Content)
	}
}

func TestCodemodeAskAndChatRead(t *testing.T) {
	// Mock read_chat tool
	mockReadChat := tools.NewTool("read_chat", "Read chat transcript", tools.TierRead, func(ctx context.Context, in map[string]any) (any, error) {
		return "Chat transcript for " + fmt.Sprint(in["ref"]), nil
	})
	// Mock ask tool
	mockAsk := tools.NewTool("ask", "Ask question", tools.TierRead, func(ctx context.Context, in map[string]any) (any, error) {
		return "[User Answer]: Selected Option A", nil
	})

	reg := tools.NewRegistry(mockReadChat, mockAsk)
	rt, err := codemode.NewRuntime(loop.NewExecutor(reg, permissions.FullAccess))
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}

	ctx := context.Background()

	// 1. readChat in code
	chatScript := `await readChat("session-123");`
	resChat, err := rt.Eval(ctx, chatScript, 5*time.Second)
	if err != nil {
		t.Fatalf("readChat eval: %v", err)
	}
	envChat := resChat.(tools.Envelope)
	if envChat.IsError || !strings.Contains(envChat.Content.(string), "session-123") {
		t.Fatalf("expected chat transcript, got: %v", envChat.Content)
	}

	// 2. ask in code
	askScript := `await ask("Which framework?", ["A", "B"]);`
	resAsk, err := rt.Eval(ctx, askScript, 5*time.Second)
	if err != nil {
		t.Fatalf("ask eval: %v", err)
	}
	envAsk := resAsk.(tools.Envelope)
	if envAsk.IsError || !strings.Contains(envAsk.Content.(string), "Option A") {
		t.Fatalf("expected ask response, got: %v", envAsk.Content)
	}
}

func TestCodemodeRequestFullAccess(t *testing.T) {
	mockReq := tools.NewTool("requestFullAccess", "Request full access", tools.TierRead, func(ctx context.Context, in map[string]any) (any, error) {
		return "Approved: Full Access granted", nil
	})
	reg := tools.NewRegistry(mockReq)
	rt, err := codemode.NewRuntime(loop.NewExecutor(reg, permissions.PlanMode))
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}
	ctx := context.Background()
	script := `await requestFullAccess("implement user features");`
	res, err := rt.Eval(ctx, script, 5*time.Second)
	if err != nil {
		t.Fatalf("requestFullAccess eval: %v", err)
	}
	env := res.(tools.Envelope)
	if env.IsError || !strings.Contains(env.Content.(string), "Approved") {
		t.Fatalf("expected approved result, got: %v", env.Content)
	}
}

func jsonString(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}
