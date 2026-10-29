package console

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/services"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/types"
)

func newControlRuntime(t *testing.T, ceiling string, aborted *[]string) (*Runtime, *services.SessionService) {
	t.Helper()
	runtime, sessions := newConsoleSessionRuntime(t, nil)
	runtime.ceiling = ceiling
	runtime.sessionID = "caller"
	runtime.abort = func(id string) bool {
		*aborted = append(*aborted, id)
		return id == "running"
	}
	return runtime, sessions
}

func evalErr(runtime *Runtime, code string) error {
	_, err := runtime.Eval(context.Background(), code, time.Second)
	return err
}

func TestClampApprovalNeverRaises(t *testing.T) {
	cases := []struct {
		ceiling, req, want string
		clamped            bool
	}{
		{"always-ask", "full-access", "always-ask", true},
		{"always-ask", "accept-edits", "always-ask", true},
		{"accept-edits", "full-access", "accept-edits", true},
		{"full-access", "full-access", "full-access", false},
		{"full-access", "plan-mode", "plan-mode", false},
		{"always-ask", "plan-mode", "plan-mode", false},
		// No run behind the kernel fails closed, never open.
		{"", "full-access", "plan-mode", true},
	}
	for _, c := range cases {
		got, clamped, note, err := clampApproval(c.ceiling, c.req)
		if err != nil || got != c.want || clamped != c.clamped {
			t.Errorf("clamp(%q,%q) = %q,%v,%v; want %q,%v", c.ceiling, c.req, got, clamped, err, c.want, c.clamped)
		}
		if clamped && !strings.Contains(note, "clamped to "+c.want) {
			t.Errorf("clamp note %q does not report the clamp", note)
		}
	}
	if _, _, _, err := clampApproval("full-access", "bogus"); err == nil {
		t.Error("unknown mode must be rejected")
	}
}

func TestCreateClampsToCeilingAndReportsIt(t *testing.T) {
	var aborted []string
	runtime, _ := newControlRuntime(t, "always-ask", &aborted)
	out := evalJSON(t, runtime, `console.sessions.create({cwd:"/tmp", approval_mode:"full-access"})`).(map[string]any)
	if out["approval_mode"] != "always-ask" {
		t.Fatalf("approval_mode = %v, want always-ask", out["approval_mode"])
	}
	if note, _ := out["clamped"].(string); !strings.Contains(note, "clamped to always-ask") {
		t.Fatalf("clamp not reported: %#v", out)
	}
	header := out["session"].(map[string]any)
	if header["approvalMode"] != "always-ask" {
		t.Fatalf("stored mode = %v", header["approvalMode"])
	}
}

func TestCreateOmittedModeInheritsCeiling(t *testing.T) {
	var aborted []string
	runtime, _ := newControlRuntime(t, "accept-edits", &aborted)
	out := evalJSON(t, runtime, `console.sessions.create({cwd:"/tmp"})`).(map[string]any)
	if out["approval_mode"] != "accept-edits" {
		t.Fatalf("approval_mode = %v", out["approval_mode"])
	}
	if _, has := out["clamped"]; has {
		t.Fatal("inheriting the ceiling is not a clamp")
	}
}

func withCatalog(t *testing.T) {
	t.Helper()
	old := catalogModels
	catalogModels = func(context.Context) []types.ProviderEntry {
		return []types.ProviderEntry{{Name: "claude", Models: []types.Model{
			{ID: "thinker", Provider: "claude", ThinkingLevels: []string{"low", "high"}},
			{ID: "plain", Provider: "claude"},
		}}}
	}
	t.Cleanup(func() { catalogModels = old })
}

func TestCreateThinkingLevel(t *testing.T) {
	withCatalog(t)
	var aborted []string
	runtime, sessions := newControlRuntime(t, "full-access", &aborted)

	out := evalJSON(t, runtime, `console.sessions.create({cwd:"/tmp", provider:"claude", model:"thinker", thinking_level:"high"})`).(map[string]any)
	id := out["session"].(map[string]any)["id"].(string)
	h, _ := sessions.Header(id)
	if h == nil || h.ThinkingLevel == nil || *h.ThinkingLevel != "high" {
		t.Fatalf("stored thinking level = %+v, want high", h)
	}

	for name, code := range map[string]string{
		"unsupported level":    `console.sessions.create({cwd:"/tmp", provider:"claude", model:"thinker", thinking_level:"max"})`,
		"model without levels": `console.sessions.create({cwd:"/tmp", provider:"claude", model:"plain", thinking_level:"low"})`,
		"unknown model":        `console.sessions.create({cwd:"/tmp", provider:"claude", model:"ghost", thinking_level:"low"})`,
		"no model":             `console.sessions.create({cwd:"/tmp", thinking_level:"low"})`,
	} {
		if err := evalErr(runtime, code); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
	err := evalErr(runtime, `console.sessions.create({cwd:"/tmp", provider:"claude", model:"thinker", thinking_level:"max"})`)
	if err == nil || !strings.Contains(err.Error(), "Supported levels: low, high") {
		t.Fatalf("err = %v, want the supported levels listed", err)
	}
}

func TestSetModelAcceptsCatalogModel(t *testing.T) {
	withCatalog(t)
	var aborted []string
	runtime, sessions := newControlRuntime(t, "full-access", &aborted)
	created := evalJSON(t, runtime, `console.sessions.create({cwd:"/tmp"})`).(map[string]any)
	id := created["session"].(map[string]any)["id"].(string)
	evalJSON(t, runtime, `console.sessions.set_model({session_id:"`+id+`", model:"thinker", provider:"claude"})`)
	if h, _ := sessions.Header(id); h.ModelID != "thinker" || h.Provider != "claude" {
		t.Fatalf("header = %+v", h)
	}
}

func TestSetApprovalClampedToCallingRun(t *testing.T) {
	var aborted []string
	runtime, sessions := newControlRuntime(t, "accept-edits", &aborted)
	created := evalJSON(t, runtime, `console.sessions.create({cwd:"/tmp", approval_mode:"plan-mode"})`).(map[string]any)
	id := created["session"].(map[string]any)["id"].(string)

	out := evalJSON(t, runtime, `console.sessions.set_approval({session_id:"`+id+`", mode:"full-access"})`).(map[string]any)
	if out["approval_mode"] != "accept-edits" || out["clamped"] == nil {
		t.Fatalf("set_approval = %#v", out)
	}
	if h, _ := sessions.Header(id); h.ApprovalMode != "accept-edits" {
		t.Fatalf("stored mode = %q", h.ApprovalMode)
	}
	// Moving down is always allowed.
	evalJSON(t, runtime, `console.sessions.set_approval({session_id:"`+id+`", mode:"plan-mode"})`)
	if h, _ := sessions.Header(id); h.ApprovalMode != "plan-mode" {
		t.Fatalf("stored mode = %q", h.ApprovalMode)
	}
	if err := evalErr(runtime, `console.sessions.set_approval({session_id:"caller", mode:"plan-mode"})`); err == nil {
		t.Fatal("a run must not change its own approval mode")
	}
}

func TestAbort(t *testing.T) {
	var aborted []string
	runtime, _ := newControlRuntime(t, "full-access", &aborted)
	evalJSON(t, runtime, `console.sessions.abort({session_id:"running"})`)
	if len(aborted) != 1 || aborted[0] != "running" {
		t.Fatalf("aborted = %v", aborted)
	}
	if err := evalErr(runtime, `console.sessions.abort({session_id:"idle"})`); err == nil {
		t.Fatal("aborting a session with no run must fail")
	}
	if err := evalErr(runtime, `console.sessions.abort({session_id:"caller"})`); err == nil {
		t.Fatal("a run must not abort itself")
	}
}

func TestSetModelRejectsUnknownModel(t *testing.T) {
	var aborted []string
	runtime, _ := newControlRuntime(t, "full-access", &aborted)
	created := evalJSON(t, runtime, `console.sessions.create({cwd:"/tmp"})`).(map[string]any)
	id := created["session"].(map[string]any)["id"].(string)
	err := evalErr(runtime, `console.sessions.set_model({session_id:"`+id+`", model:"definitely-not-real", provider:"claude"})`)
	if err == nil || !strings.Contains(err.Error(), "console.models()") {
		t.Fatalf("err = %v, want a pointer to console.models()", err)
	}
}

func TestForkClampsInheritedApproval(t *testing.T) {
	var aborted []string
	runtime, sessions := newControlRuntime(t, "always-ask", &aborted)
	// The source was created by someone with more authority than this run.
	src, err := sessions.Create(newSource("full-access"))
	if err != nil {
		t.Fatal(err)
	}
	if err := sessions.AppendMessage(src.ID, types.AgentMessage{
		ID: "m1", Role: "user", Data: []byte(`{"user":{"content":"hi"}}`),
	}); err != nil {
		t.Fatal(err)
	}
	out := evalJSON(t, runtime, `console.sessions.fork({session_id:"`+src.ID+`", message_id:"m1"})`).(map[string]any)
	forked := out["session"].(map[string]any)
	if forked["approvalMode"] != "always-ask" {
		t.Fatalf("forked approvalMode = %v, want always-ask", forked["approvalMode"])
	}
	if out["clamped"] == nil {
		t.Fatalf("clamp not reported: %#v", out)
	}
	if h, _ := sessions.Header(forked["id"].(string)); h == nil || h.ApprovalMode != "always-ask" {
		t.Fatalf("stored fork mode = %+v", h)
	}
	// The source is untouched.
	if h, _ := sessions.Header(src.ID); h.ApprovalMode != "full-access" {
		t.Fatalf("source mode changed to %q", h.ApprovalMode)
	}

	if err := evalErr(runtime, `console.sessions.fork({session_id:"`+src.ID+`", message_id:"missing"})`); err == nil {
		t.Fatal("fork at an unknown message must fail")
	}
	if err := evalErr(runtime, `console.sessions.fork({session_id:"`+src.ID+`"})`); err == nil {
		t.Fatal("message_id is required")
	}
}

func TestForkWithinCeilingIsNotClamped(t *testing.T) {
	var aborted []string
	runtime, sessions := newControlRuntime(t, "full-access", &aborted)
	src, _ := sessions.Create(newSource("accept-edits"))
	_ = sessions.AppendMessage(src.ID, types.AgentMessage{ID: "m1", Role: "user", Data: []byte(`{"user":{"content":"hi"}}`)})
	out := evalJSON(t, runtime, `console.sessions.fork({session_id:"`+src.ID+`", message_id:"m1"})`).(map[string]any)
	if out["clamped"] != nil {
		t.Fatalf("unexpected clamp: %#v", out)
	}
	if out["session"].(map[string]any)["approvalMode"] != "accept-edits" {
		t.Fatalf("fork mode = %v", out["session"])
	}
}

func newSource(mode string) types.CreateSessionOptions {
	return types.CreateSessionOptions{Title: "src", Cwd: "/tmp", ApprovalMode: mode}
}
