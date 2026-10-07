// Cua Driver integration against a real library. Skipped unless
// CUA_DRIVER_LIB_PATH points at a real libcua_driver_sdk, so CI is unaffected.
//
// These are the assertions that only a real driver can satisfy: the ABI check,
// the tool inventory shape, a live tool call, image decoding, and the stop
// contract. Everything else in the package is covered by unit tests.
package tests

import (
	"context"
	"encoding/json"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/tools"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/services/cua"
)

// requireDriver opens the real library, wherever the loader finds it
// (vendored tree, system dir, or explicit override). It skips when no usable
// library exists, so CI is unaffected — but an explicitly configured path
// that fails to open is a real misconfiguration and fails loudly instead of
// silently skipping.
func requireDriver(t *testing.T) *cua.Driver {
	t.Helper()
	driver, err := cua.Open(cua.Options{})
	if err == nil {
		t.Cleanup(func() { _ = driver.Shutdown() })
		return driver
	}
	if os.Getenv("CUA_DRIVER_LIB_PATH") != "" {
		t.Fatalf("CUA_DRIVER_LIB_PATH is set but the driver would not open: %v", err)
	}
	t.Skipf("no usable Cua Driver library: %v", err)
	return nil
}

// driverList reads the advertised inventory, the same call /computer-use makes
// when it builds the tool group.
func driverList(t *testing.T) ([]cua.ToolDef, error) {
	t.Helper()
	driver, err := cua.Open(cua.Options{})
	if err != nil {
		t.Fatalf("open driver: %v", err)
	}
	t.Cleanup(func() { _ = driver.Shutdown() })
	return driver.ListTools()
}

func TestDriverABIMatches(t *testing.T) {
	requireDriver(t)
	major, minor, _, err := cua.ABIVersion()
	if err != nil {
		t.Fatalf("abi version: %v", err)
	}
	if major != 1 || minor != 1 {
		t.Fatalf("ABI %d.%d, want 1.1", major, minor)
	}
}

// TestDriverAdvertisesClassifiedTools is the assertion that matters for
// tiering: every advertised tool carries a risk classification, so Console never
// has to fall back to a name heuristic.
func TestDriverAdvertisesClassifiedTools(t *testing.T) {
	requireDriver(t)
	list, err := driverList(t)
	if err != nil {
		t.Fatalf("list tools: %v", err)
	}
	if len(list) < 10 {
		t.Fatalf("only %d tools advertised, expected the full inventory", len(list))
	}

	unclassified := 0
	for _, tool := range list {
		switch tool.RiskClass() {
		case "r0", "r1", "r2", "r3", "r4":
		case "unclassified", "":
			unclassified++
			t.Logf("tool %s has no reviewed risk classification", tool.Name)
		default:
			t.Errorf("tool %s has an unknown risk class %q", tool.Name, tool.RiskClass())
		}
		// A real schema is what lets the model pass correct arguments.
		if len(tool.InputSchema) == 0 {
			t.Errorf("tool %s advertises no input schema", tool.Name)
		}
	}
	if unclassified > 0 {
		t.Errorf("%d tools are unclassified; tiering would be guessing for those", unclassified)
	}
}

// TestDriverClassifiesKnownTools pins the classifications the skill relies on:
// kill_app terminates a process, the input loop must be able to act, and
// observation carries the driver's own readOnlyHint.
func TestDriverClassifiesKnownTools(t *testing.T) {
	requireDriver(t)
	list, err := driverList(t)
	if err != nil {
		t.Fatalf("list tools: %v", err)
	}
	byName := map[string]cua.ToolDef{}
	for _, tool := range list {
		byName[tool.Name] = tool
	}

	if def, ok := byName["kill_app"]; ok {
		if def.RiskClass() != "r3" {
			t.Errorf("kill_app risk = %q, want r3", def.RiskClass())
		}
	} else {
		t.Error("kill_app not advertised")
	}
	for _, name := range []string{"click", "type_text", "press_key"} {
		def, ok := byName[name]
		if !ok {
			t.Errorf("%s not advertised", name)
			continue
		}
		if def.RiskClass() != "r1" {
			t.Errorf("%s risk = %q, want r1", name, def.RiskClass())
		}
	}
	for _, name := range []string{"get_window_state", "list_apps"} {
		def, ok := byName[name]
		if !ok {
			t.Errorf("%s not advertised", name)
			continue
		}
		if !def.ReadOnly() {
			t.Errorf("%s should carry readOnlyHint", name)
		}
	}
}

// TestDriverCallReturnsStructuredResult proves the async completion path works
// end to end, which no unit test can cover.
func TestDriverCallReturnsStructuredResult(t *testing.T) {
	driver := requireDriver(t)
	result, err := driver.Call(context.Background(), "get_screen_size", map[string]any{}, nil)
	if err != nil {
		t.Fatalf("get_screen_size: %v", err)
	}
	if result.IsError {
		t.Fatalf("get_screen_size reported an error: %+v", result.Content)
	}
	if len(result.Content) == 0 {
		t.Fatal("get_screen_state returned no content")
	}
	if result.Content[0].Type != "text" {
		t.Fatalf("first part is %q, want text", result.Content[0].Type)
	}
	if !strings.Contains(result.Content[0].Text, "display") {
		t.Errorf("unexpected text: %q", result.Content[0].Text)
	}
}

// TestDriverScreenshotReachesScriptCode is the screenshot path through the
// same conversion the model receives: an image part arrives with a mime type
// the Claude converter accepts and base64 data. It needs a real permitted
// process, so a refusal skips rather than fails.
func TestDriverScreenshotReachesScriptCode(t *testing.T) {
	requireDriver(t)
	out, err := cua.NewManager().EvalOnce(context.Background(),
		`cua.get_desktop_state().content.filter(function(p) { return p.type === "image" })`, 0)
	if err != nil {
		t.Skipf("capture refused in this environment: %v", err)
	}
	parts, ok := out.([]map[string]any)
	if !ok {
		t.Fatalf("eval returned %T", out)
	}
	var harnessImage map[string]any
	for _, p := range parts {
		if p["type"] == "image" {
			harnessImage = p
		}
	}
	if harnessImage == nil {
		// Without Screen Recording the driver returns a refusal instead of an
		// image; that is a legitimate outcome, not a binding failure.
		t.Skip("no image content: capture is not permitted for this process")
	}
	if harnessImage["mimeType"] != "image/png" {
		t.Errorf("mimeType = %v, want image/png", harnessImage["mimeType"])
	}
	data, _ := harnessImage["data"].(string)
	if len(data) == 0 {
		t.Fatal("image part carries no data")
	}
}

// TestDriverStopRefusesThenResumes covers the kill switch against the real
// runtime: a stopped driver must refuse new work with a *known* outcome, and
// must come back after Resume.
func TestDriverStopRefusesThenResumes(t *testing.T) {
	driver := requireDriver(t)
	ctx := context.Background()

	driver.Stop()
	_, err := driver.Call(ctx, "get_screen_size", map[string]any{}, nil)
	if err == nil {
		t.Fatal("a call succeeded after Stop")
	}
	var cancelled *cua.CancelledError
	if !errorsAsCua(err, &cancelled) {
		t.Fatalf("got %v, want a CancelledError", err)
	}
	if cancelled.Unknown {
		t.Error("a refused start has a known outcome and must not report unknown")
	}

	driver.Resume()
	if _, err := driver.Call(ctx, "get_screen_size", map[string]any{}, nil); err != nil {
		t.Fatalf("call after Resume failed: %v", err)
	}
}

// TestLoaderLoadsExactlyTwoTools is the option-3 surface: one invocation
// brings computer plus computer_reset and nothing else, so the per-turn
// schema cost stays flat no matter how many methods the driver has.
func TestLoaderLoadsExactlyTwoTools(t *testing.T) {
	requireDriver(t)
	registry := tools.NewRegistry()
	loader := cua.NewLoader(cua.NewManager(), "session-1")
	n, err := loader.Load(registry)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if n != 2 {
		t.Fatalf("loaded %d tools, want 2", n)
	}
	names := registry.Names()
	if len(names) != 2 || names[0] != "computer" || names[1] != "computer_reset" {
		t.Fatalf("registry holds %v", names)
	}
	if !loader.Loaded() {
		t.Error("a successful load must mark the group loaded")
	}
}

// TestComputerToolRunsCode drives the tool exactly the way the agent will:
// through the registry, with JSON arguments.
func TestComputerToolRunsCode(t *testing.T) {
	requireDriver(t)
	registry := tools.NewRegistry()
	loader := cua.NewLoader(cua.NewManager(), "session-1")
	if _, err := loader.Load(registry); err != nil {
		t.Fatalf("load: %v", err)
	}
	tool, err := registry.Get("computer")
	if err != nil {
		t.Fatalf("registry: %v", err)
	}
	out, err := tool.Execute(context.Background(), json.RawMessage(`{"code": "40 + 2"}`))
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	parts, ok := out.([]map[string]any)
	if !ok || len(parts) == 0 || parts[0]["type"] != "text" {
		t.Fatalf("unexpected result shape: %v", out)
	}
	if !strings.Contains(parts[0]["text"].(string), "42") {
		t.Errorf("got %q", parts[0]["text"])
	}
}

// TestSessionKernelsAreIsolated proves one session cannot see another's
// variables, and that reset clears without touching siblings.
func TestSessionKernelsAreIsolated(t *testing.T) {
	requireDriver(t)
	manager := cua.NewManager()
	exec := func(session, code string) string {
		t.Helper()
		tool := cua.NewComputerTool(manager, session)
		out, err := tool.Execute(context.Background(), json.RawMessage(`{"code": `+strconv.Quote(code)+`}`))
		if err != nil {
			t.Fatalf("%s: %v", session, err)
		}
		parts := out.([]map[string]any)
		text, _ := parts[0]["text"].(string)
		return text
	}

	exec("s1", `var secret = "s1-only"`)
	if got := exec("s2", `typeof secret`); !strings.Contains(got, "undefined") {
		t.Fatalf("s2 sees s1's variables: %q", got)
	}
	if got := exec("s1", `secret`); !strings.Contains(got, "s1-only") {
		t.Fatalf("s1 lost its own variable: %q", got)
	}
	if got := manager.SessionRuntimeCount(); got != 2 {
		t.Fatalf("want 2 live kernels, got %d", got)
	}

	reset := cua.NewComputerResetTool(manager, "s1")
	if _, err := reset.Execute(context.Background(), json.RawMessage(`{}`)); err != nil {
		t.Fatalf("reset: %v", err)
	}
	// Count before touching s1 again: the verification call below recreates
	// its kernel.
	if got := manager.SessionRuntimeCount(); got != 1 {
		t.Fatalf("want 1 live kernel after reset, got %d", got)
	}
	if got := exec("s1", `typeof secret`); !strings.Contains(got, "undefined") {
		t.Fatalf("reset did not clear s1: %q", got)
	}
}

func errorsAsCua(err error, target **cua.CancelledError) bool {
	for err != nil {
		if c, ok := err.(*cua.CancelledError); ok {
			*target = c
			return true
		}
		u, ok := err.(interface{ Unwrap() error })
		if !ok {
			return false
		}
		err = u.Unwrap()
	}
	return false
}
