// Element-id resolution and launch waiting: the runtime's two wrappers over
// raw driver calls. Fakes stand in for the driver throughout.
package tests

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/services/cua"
)

const resolveTree = `- [0] AXWindow "Calculator" [id=main actions=[raise]]
    - AXStaticText = "0"
    - [5] AXButton (7) [id=Seven actions=[press]]
    - [6] AXButton (8) [id=Eight actions=[press]]
    - [8] AXButton (Multiply) [id=Multiply actions=[press]]
    - [9] AXButton (see [id=Fake] docs)
    - [19] AXButton (Equals) [id=Equals actions=[press]]
`

func resolveCaller() *fakeJSCaller {
	return &fakeJSCaller{
		tools: []cua.ToolDef{
			{Name: "get_window_state"},
			{Name: "click"},
			{Name: "list_apps"},
			{Name: "launch_app"},
			{Name: "list_windows"},
		},
		results: map[string]*cua.ToolResult{
			"get_window_state": {
				StructuredContent: json.RawMessage(`{"snapshot_id": "s1", "tree_markdown": ` + quoteForJSON(resolveTree) + `}`),
			},
			"click": {Content: []cua.ContentPart{{Type: "text", Text: `Performed AXPress`}}},
		},
	}
}

func quoteForJSON(s string) string {
	raw, _ := json.Marshal(s)
	return string(raw)
}

// TestResolveElementIDInjectsFreshToken is the core promise: address by
// stable id, act by fresh token, with the driver's wire shape untouched
// (element_id stripped, never forwarded to deny_unknown_fields).
func TestResolveElementIDInjectsFreshToken(t *testing.T) {
	caller := resolveCaller()
	runtime := newTestRuntime(t, caller)
	parts := evalParts(t, runtime, `cua.click({pid: 1, window_id: 2, element_id: "Multiply"})`)

	var sawSnapshot, sawClick map[string]any
	for _, c := range caller.calls {
		switch c.name {
		case "get_window_state":
			sawSnapshot = c.args
		case "click":
			sawClick = c.args
		}
	}
	if sawSnapshot == nil {
		t.Fatal("no snapshot taken before acting")
	}
	if sawClick == nil {
		t.Fatal("click never reached the driver")
	}
	if token, _ := sawClick["element_token"].(string); token != "s1:8" {
		t.Errorf("element_token = %q, want s1:8", token)
	}
	if _, ok := sawClick["element_id"]; ok {
		t.Errorf("element_id leaked to the driver: %v", sawClick)
	}
	if text := partText(parts); !strings.Contains(text, "AXPress") {
		t.Errorf("click result lost: %q", text)
	}
}

func TestResolveElementIDMissListsAvailable(t *testing.T) {
	runtime := newTestRuntime(t, resolveCaller())
	_, err := runtime.Eval(context.Background(), `cua.click({pid: 1, window_id: 2, element_id: "Divide"})`, 0)
	if err == nil {
		t.Fatal("expected a miss to fail")
	}
	// Fail loud with what IS there: the model fixes the id instead of
	// guessing another row.
	for _, want := range []string{"Seven", "Multiply", "Equals"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not list %q", err, want)
		}
	}
	if strings.Contains(err.Error(), "Fake") {
		t.Errorf("description text [id=Fake] must not resolve as an element: %v", err)
	}
}

func TestResolveElementIDNeedsNumericTarget(t *testing.T) {
	runtime := newTestRuntime(t, resolveCaller())
	for _, code := range []string{
		`cua.click({window_id: 2, element_id: "Seven"})`,
		`cua.click({pid: 1, element_id: "Seven"})`,
		`cua.click({pid: "1", window_id: 2, element_id: "Seven"})`,
	} {
		if _, err := runtime.Eval(context.Background(), code, 0); err == nil {
			t.Errorf("%s: expected a missing-target error", code)
		} else if !strings.Contains(err.Error(), "numeric") {
			t.Errorf("%s: unhelpful error %v", code, err)
		}
	}
}

func TestResolveElementIDRejectedOffElementMethods(t *testing.T) {
	runtime := newTestRuntime(t, resolveCaller())
	_, err := runtime.Eval(context.Background(), `cua.list_apps({element_id: "Seven"})`, 0)
	if err == nil {
		t.Fatal("expected element_id off an element method to fail")
	}
	if !strings.Contains(err.Error(), "only meaningful") {
		t.Fatalf("unhelpful error: %v", err)
	}
}

func TestResolveExplicitTokenWinsOverID(t *testing.T) {
	caller := resolveCaller()
	runtime := newTestRuntime(t, caller)
	evalParts(t, runtime, `cua.click({pid: 1, window_id: 2, element_token: "s9:1", element_id: "Seven"})`)
	for _, c := range caller.calls {
		if c.name == "get_window_state" {
			t.Fatal("explicit token must not trigger a snapshot")
		}
		if c.name == "click" {
			if token, _ := c.args["element_token"].(string); token != "s9:1" {
				t.Fatalf("token = %q, want the explicit s9:1", token)
			}
		}
	}
}

func TestParseSnapshotRowsIgnoresNonRows(t *testing.T) {
	markdown := "- [5] AXButton (7) [id=Seven actions=[press]]\n" +
		"- AXStaticText = \"0\"\n" +
		"- [9] AXButton (see [id=Fake] docs)\n" +
		"element_token for row [N] = s1:N\n" +
		"- [19] AXButton (Equals) [id=Equals actions=[press]]\n"
	rows := cua.ParseSnapshotRowsForTest(markdown)
	if len(rows) != 2 {
		t.Fatalf("got %v", rows)
	}
	if rows[0].Row != 5 || rows[0].ID != "Seven" {
		t.Errorf("first = %+v", rows[0])
	}
	if rows[1].Row != 19 || rows[1].ID != "Equals" {
		t.Errorf("second = %+v", rows[1])
	}
}

func launchCaller(windowsOnCall int) *fakeJSCaller {
	caller := &fakeJSCaller{
		tools: []cua.ToolDef{
			{Name: "launch_app"},
			{Name: "list_windows"},
		},
		results: map[string]*cua.ToolResult{
			"launch_app": {
				StructuredContent: json.RawMessage(`{"pid": 42, "windows": []}`),
				Content:           []cua.ContentPart{{Type: "text", Text: "Launched."}},
			},
		},
	}
	caller.onCall = func(name string, args map[string]any, call int) (*cua.ToolResult, error, bool) {
		if name != "list_windows" {
			return nil, nil, false
		}
		if call >= windowsOnCall {
			return &cua.ToolResult{
				StructuredContent: json.RawMessage(`{"windows": [{"window_id": 7, "title": "App"}]}`),
			}, nil, true
		}
		return &cua.ToolResult{
			StructuredContent: json.RawMessage(`{"windows": []}`),
		}, nil, true
	}
	return caller
}

// TestLaunchAppHappyPathAddsNothing verifies the wait only runs on the
// failure case: a launch that already has windows returns untouched.
func TestLaunchAppHappyPathAddsNothing(t *testing.T) {
	caller := launchCaller(0)
	caller.results["launch_app"] = &cua.ToolResult{
		StructuredContent: json.RawMessage(`{"pid": 42, "windows": [{"window_id": 7}]}`),
		Content:           []cua.ContentPart{{Type: "text", Text: "Launched."}},
	}
	runtime := newTestRuntime(t, caller)
	parts := evalParts(t, runtime, `cua.launch_app({bundle_id: "x"})`)
	for _, c := range caller.calls {
		if c.name == "list_windows" {
			t.Fatal("happy-path launch must not poll")
		}
	}
	if text := partText(parts); !strings.Contains(text, "Launched.") {
		t.Fatalf("result altered: %q", text)
	}
}

// TestLaunchAppWaitsForWindow is the reported race: launch says no window,
// the runtime polls, the window appears, the note says exactly that.
func TestLaunchAppWaitsForWindow(t *testing.T) {
	caller := launchCaller(3)
	runtime := newTestRuntime(t, caller)
	parts := evalParts(t, runtime, `cua.launch_app({bundle_id: "x"})`)
	text := partText(parts)
	if !strings.Contains(text, "appeared") || !strings.Contains(text, "get_window_state") {
		t.Fatalf("no appearance note: %q", text)
	}
	polls := 0
	for _, c := range caller.calls {
		if c.name == "list_windows" {
			polls++
		}
	}
	if polls == 0 {
		t.Fatal("never polled")
	}
}

// TestLaunchAppTimeoutSaysSpace names the likely cause on timeout: the window
// may exist on another Space, which is the single most confusing shape this
// feature produces.
func TestLaunchAppTimeoutSaysSpace(t *testing.T) {
	caller := launchCaller(1000)
	runtime := newTestRuntime(t, caller)
	parts := evalParts(t, runtime, `cua.launch_app({bundle_id: "x", wait_timeout_ms: 600})`)
	text := partText(parts)
	if !strings.Contains(text, "Space") || !strings.Contains(text, "bring_to_front") {
		t.Fatalf("timeout note must name the Space cause: %q", text)
	}
}
