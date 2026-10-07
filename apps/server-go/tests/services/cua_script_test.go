// The computer-use JavaScript runtime: evaluation, persistence, images and
// failure modes. A fake driver stands in for the native library, so CI covers
// everything except the real ABI boundary (covered by the integration tests).
package tests

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/tools"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/services/cua"
)

// fakeJSCaller answers tool calls from canned results and records what it was
// asked, so tests assert the exact arguments the script produced.
type fakeJSCaller struct {
	tools   []cua.ToolDef
	results map[string]*cua.ToolResult
	errors  map[string]error
	calls   []fakeJSCall
}

type fakeJSCall struct {
	name string
	args map[string]any
}

func (f *fakeJSCaller) ListTools() ([]cua.ToolDef, error) { return f.tools, nil }

func (f *fakeJSCaller) Call(_ context.Context, name string, args map[string]any, _ func() bool) (*cua.ToolResult, error) {
	f.calls = append(f.calls, fakeJSCall{name: name, args: args})
	if err, ok := f.errors[name]; ok {
		return nil, err
	}
	if res, ok := f.results[name]; ok {
		return res, nil
	}
	return &cua.ToolResult{}, nil
}

func screenSizeCaller() *fakeJSCaller {
	return &fakeJSCaller{
		tools:  []cua.ToolDef{{Name: "get_screen_size"}, {Name: "list_apps"}},
		errors: map[string]error{},
		results: map[string]*cua.ToolResult{
			"get_screen_size": {Content: []cua.ContentPart{
				{Type: "text", Text: "Main display: 1440x900"},
			}},
		},
	}
}

func newTestRuntime(t *testing.T, caller cua.JSCaller) *cua.JSRuntime {
	t.Helper()
	runtime, err := cua.NewJSRuntime(caller)
	if err != nil {
		t.Fatalf("new runtime: %v", err)
	}
	return runtime
}

// evalParts runs code and returns the harness parts, failing the test on a
// Go-level error (a model-visible failure is a valid outcome and is returned
// for assertion instead).
func evalParts(t *testing.T, runtime *cua.JSRuntime, code string) []map[string]any {
	t.Helper()
	out, err := runtime.Eval(context.Background(), code, 0)
	if err != nil {
		t.Fatalf("eval: %v", err)
	}
	parts, ok := out.([]map[string]any)
	if !ok {
		t.Fatalf("eval returned %T, want []map[string]any", out)
	}
	return parts
}

func partText(parts []map[string]any) string {
	for _, p := range parts {
		if p["type"] == "text" {
			if text, _ := p["text"].(string); text != "" {
				return text
			}
		}
	}
	return ""
}

func TestJSEvalCallsDriverAndReturnsResult(t *testing.T) {
	caller := screenSizeCaller()
	parts := evalParts(t, newTestRuntime(t, caller), `cua.get_screen_size().content[0].text`)
	if text := partText(parts); text != `"Main display: 1440x900"` {
		t.Fatalf("got %q", text)
	}
	if len(caller.calls) != 1 || caller.calls[0].name != "get_screen_size" {
		t.Fatalf("driver saw %v", caller.calls)
	}
}

func TestJSBindingsPersistBetweenEvals(t *testing.T) {
	runtime := newTestRuntime(t, screenSizeCaller())
	evalParts(t, runtime, `var answer = 40 + 2`)
	parts := evalParts(t, runtime, `answer * 2`)
	if text := partText(parts); text != "84" {
		t.Fatalf("binding did not persist, got %q", text)
	}
}

func TestJSPrintCapturesNarration(t *testing.T) {
	runtime := newTestRuntime(t, screenSizeCaller())
	parts := evalParts(t, runtime, "print('screen is', 1440)\n'raw result'")
	text := partText(parts)
	if !strings.Contains(text, "screen is 1440") {
		t.Errorf("print output missing: %q", text)
	}
	if !strings.Contains(text, "raw result") {
		t.Errorf("completion value missing: %q", text)
	}
}

func TestJSArgumentsPassThroughAsObject(t *testing.T) {
	caller := screenSizeCaller()
	runtime := newTestRuntime(t, caller)
	evalParts(t, runtime, `cua.list_apps({})`)
	if len(caller.calls) != 1 {
		t.Fatalf("driver saw %v", caller.calls)
	}
	if len(caller.calls[0].args) != 0 {
		t.Fatalf("empty object should arrive empty, got %v", caller.calls[0].args)
	}
}

func TestJSUndefinedFieldsAreDropped(t *testing.T) {
	caller := screenSizeCaller()
	runtime := newTestRuntime(t, caller)
	// An explicit undefined must not reach the driver: it fails closed on
	// mistyped arguments, so an omitted option must stay omitted.
	evalParts(t, runtime, `cua.get_screen_size({include_screenshot: undefined})`)
	if len(caller.calls[0].args) != 0 {
		t.Fatalf("undefined field survived: %v", caller.calls[0].args)
	}
}

func TestJSNonObjectArgumentIsRejected(t *testing.T) {
	runtime := newTestRuntime(t, screenSizeCaller())
	_, err := runtime.Eval(context.Background(), `cua.get_screen_size(42)`, 0)
	if err == nil {
		t.Fatal("expected a model-visible failure")
	}
	if !strings.Contains(err.Error(), "single object argument") {
		t.Fatalf("unhelpful message: %v", err)
	}
}

func TestJSUnknownMethodIsAModelError(t *testing.T) {
	runtime := newTestRuntime(t, screenSizeCaller())
	_, err := runtime.Eval(context.Background(), `cua.dance()`, 0)
	if err == nil {
		t.Fatal("expected a failure for an unknown method")
	}
	// A script bug must be model-visible (so it can fix the code), never a
	// harness abort.
	var toolErr *tools.ToolError
	if !isToolError(err, &toolErr) {
		t.Fatalf("got %T, want a model-visible tool error", err)
	}
}

func TestJSSyntaxErrorIsAModelError(t *testing.T) {
	runtime := newTestRuntime(t, screenSizeCaller())
	_, err := runtime.Eval(context.Background(), `cua.get_screen_size(`, 0)
	if err == nil {
		t.Fatal("expected a failure for broken syntax")
	}
	if !strings.Contains(err.Error(), "JavaScript error") {
		t.Fatalf("got %v", err)
	}
}

func TestJSDriverFailureThrowsWithItsMessage(t *testing.T) {
	caller := screenSizeCaller()
	caller.errors["list_apps"] = &cua.StatusError{Status: 6, Detail: "boom"}
	runtime := newTestRuntime(t, caller)
	_, err := runtime.Eval(context.Background(), `cua.list_apps()`, 0)
	if err == nil {
		t.Fatal("expected the driver failure to surface")
	}
	if !strings.Contains(err.Error(), "boom") || !strings.Contains(err.Error(), "cua.list_apps") {
		t.Fatalf("driver message lost: %v", err)
	}
}

func TestJSImagesBecomeNativeParts(t *testing.T) {
	png := []byte{0x89, 'P', 'N', 'G', 1, 2, 3}
	caller := screenSizeCaller()
	caller.results["get_desktop_state"] = &cua.ToolResult{Content: []cua.ContentPart{
		{Type: "image", MIMEType: "image/png", Data: png},
	}}
	caller.tools = append(caller.tools, cua.ToolDef{Name: "get_desktop_state"})
	runtime := newTestRuntime(t, caller)

	parts := evalParts(t, runtime, `cua.get_desktop_state().content[0]`)
	var img map[string]any
	for _, p := range parts {
		if p["type"] == "image" {
			img = p
		}
	}
	if img == nil {
		t.Fatalf("no image part in %v", parts)
	}
	if img["mimeType"] != "image/png" {
		t.Errorf("mimeType = %v", img["mimeType"])
	}
	if img["data"] != base64.StdEncoding.EncodeToString(png) {
		t.Errorf("image bytes did not survive the round trip")
	}
}

func TestJSHandBuiltGarbageImageFailsLoudly(t *testing.T) {
	runtime := newTestRuntime(t, screenSizeCaller())
	_, err := runtime.Eval(context.Background(), `({type: 'image', data: '!!! not base64 !!!'})`, 0)
	if err == nil {
		t.Fatal("expected corrupt image data to fail")
	}
	if !strings.Contains(err.Error(), "base64") {
		t.Fatalf("got %v", err)
	}
}

func TestJSAsyncCompletionFailsLoudly(t *testing.T) {
	runtime := newTestRuntime(t, screenSizeCaller())
	// Without a microtask pump a .then callback never runs, so a promise
	// completion would be a silent no-op. Fail instead of letting the model
	// believe an action happened.
	_, err := runtime.Eval(context.Background(), `Promise.resolve(1).then(x => x)`, 0)
	if err == nil {
		t.Fatal("expected async completion to be refused")
	}
	if !strings.Contains(err.Error(), "async") {
		t.Fatalf("got %v", err)
	}
}

func TestJSInfiniteLoopIsInterrupted(t *testing.T) {
	runtime := newTestRuntime(t, screenSizeCaller())
	start := time.Now()
	_, err := runtime.Eval(context.Background(), `while (true) {}`, 200*time.Millisecond)
	elapsed := time.Since(start)
	if err == nil {
		t.Fatal("expected the runaway script to be stopped")
	}
	if !strings.Contains(err.Error(), "unknown") || !strings.Contains(err.Error(), "before retrying") {
		t.Fatalf("wrong stop wording: %v", err)
	}
	if elapsed > 10*time.Second {
		t.Fatalf("interrupt took %v", elapsed)
	}
}

func TestJSEmptyCodeIsRejected(t *testing.T) {
	runtime := newTestRuntime(t, screenSizeCaller())
	if _, err := runtime.Eval(context.Background(), "   \n ", 0); err == nil {
		t.Fatal("expected empty code to be rejected")
	}
}

func TestJSStructuredContentReachesCode(t *testing.T) {
	caller := screenSizeCaller()
	caller.results["get_window_state"] = &cua.ToolResult{
		StructuredContent: json.RawMessage(`{"window_id": 42, "pid": 7}`),
	}
	caller.tools = append(caller.tools, cua.ToolDef{Name: "get_window_state"})
	runtime := newTestRuntime(t, caller)

	parts := evalParts(t, runtime, `var s = cua.get_window_state({pid: 7, window_id: 42}).structuredContent; s.window_id + s.pid`)
	if text := partText(parts); text != "49" {
		t.Fatalf("structured content did not reach code, got %q", text)
	}
	var sawArgs map[string]any
	for _, c := range caller.calls {
		if c.name == "get_window_state" {
			sawArgs = c.args
		}
	}
	if sawArgs["pid"] == nil || sawArgs["window_id"] == nil {
		t.Fatalf("arguments lost: %v", sawArgs)
	}
}

func isToolError(err error, target **tools.ToolError) bool {
	for err != nil {
		if e, ok := err.(*tools.ToolError); ok {
			*target = e
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
