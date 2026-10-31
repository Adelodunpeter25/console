package console

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
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

type promptCall struct {
	id, text, model, provider, approval, thinking string
}

func newPromptRuntime(t *testing.T, ceiling string, queued bool, calls *[]promptCall) (*Runtime, *services.SessionService) {
	t.Helper()
	var aborted []string
	runtime, sessions := newControlRuntime(t, ceiling, &aborted)
	runtime.prompt = func(id, text, model, provider, approval, thinking string) (bool, error) {
		*calls = append(*calls, promptCall{id, text, model, provider, approval, thinking})
		return queued, nil
	}
	return runtime, sessions
}

func TestPromptStartsIdleSession(t *testing.T) {
	var calls []promptCall
	runtime, sessions := newPromptRuntime(t, "full-access", false, &calls)
	target, _ := sessions.Create(newSource("accept-edits"))
	out := evalJSON(t, runtime, `console.sessions.prompt({session_id:"`+target.ID+`", prompt:"also add tests"})`).(map[string]any)
	if out["queued"] != false || out["note"] != nil {
		t.Fatalf("idle prompt = %#v", out)
	}
	if len(calls) != 1 || calls[0].text != "also add tests" || calls[0].id != target.ID {
		t.Fatalf("calls = %+v", calls)
	}
	// Omitted mode means the session's own.
	if calls[0].approval != "accept-edits" {
		t.Fatalf("approval = %q, want the session's accept-edits", calls[0].approval)
	}
}

func TestPromptReportsQueueingWhenBusy(t *testing.T) {
	var calls []promptCall
	runtime, sessions := newPromptRuntime(t, "full-access", true, &calls)
	target, _ := sessions.Create(newSource("always-ask"))
	out := evalJSON(t, runtime, `console.sessions.prompt({session_id:"`+target.ID+`", prompt:"next"})`).(map[string]any)
	if out["queued"] != true {
		t.Fatalf("busy prompt must report queued: %#v", out)
	}
	if note, _ := out["note"].(string); !strings.Contains(note, "when its current run finishes") {
		t.Fatalf("note = %q", note)
	}
}

// The target may store more authority than the caller holds; prompting it
// must not let a restricted run act with that higher mode.
func TestPromptCannotBorrowTheTargetsHigherMode(t *testing.T) {
	var calls []promptCall
	runtime, sessions := newPromptRuntime(t, "always-ask", false, &calls)
	target, _ := sessions.Create(newSource("full-access"))
	out := evalJSON(t, runtime, `console.sessions.prompt({session_id:"`+target.ID+`", prompt:"go"})`).(map[string]any)
	if calls[0].approval != "always-ask" || out["clamped"] == nil {
		t.Fatalf("approval = %q out = %#v", calls[0].approval, out)
	}
	// Explicitly asking for more is clamped too.
	evalJSON(t, runtime, `console.sessions.prompt({session_id:"`+target.ID+`", prompt:"go", approval_mode:"full-access"})`)
	if calls[1].approval != "always-ask" {
		t.Fatalf("explicit request approval = %q", calls[1].approval)
	}
	// The turn cap never rewrites the session's stored mode.
	if h, _ := sessions.Header(target.ID); h.ApprovalMode != "full-access" {
		t.Fatalf("stored mode changed to %q", h.ApprovalMode)
	}
}

func TestPromptRefusals(t *testing.T) {
	var calls []promptCall
	runtime, sessions := newPromptRuntime(t, "full-access", false, &calls)
	target, _ := sessions.Create(newSource("always-ask"))
	for name, code := range map[string]string{
		"missing session":  `console.sessions.prompt({session_id:"ghost", prompt:"x"})`,
		"missing prompt":   `console.sessions.prompt({session_id:"` + target.ID + `"})`,
		"missing id":       `console.sessions.prompt({prompt:"x"})`,
		"own session":      `console.sessions.prompt({session_id:"caller", prompt:"x"})`,
		"unknown approval": `console.sessions.prompt({session_id:"` + target.ID + `", prompt:"x", approval_mode:"bogus"})`,
	} {
		if err := evalErr(runtime, code); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
	if len(calls) != 0 {
		t.Fatalf("refused prompts must not reach the run service: %+v", calls)
	}
}

func TestPromptValidatesThinkingAgainstTheSessionsModel(t *testing.T) {
	withCatalog(t)
	var calls []promptCall
	runtime, sessions := newPromptRuntime(t, "full-access", false, &calls)
	opts := newSource("always-ask")
	opts.ModelID, opts.Provider = "thinker", "claude"
	target, _ := sessions.Create(opts)
	evalJSON(t, runtime, `console.sessions.prompt({session_id:"`+target.ID+`", prompt:"x", thinking_level:"high"})`)
	if calls[0].thinking != "high" {
		t.Fatalf("thinking = %q", calls[0].thinking)
	}
	if err := evalErr(runtime, `console.sessions.prompt({session_id:"`+target.ID+`", prompt:"x", thinking_level:"max"})`); err == nil {
		t.Fatal("unsupported level must be refused")
	}
}

func TestUnknownAndWrongTypedArgumentsAreRefused(t *testing.T) {
	var aborted []string
	runtime, _ := newControlRuntime(t, "full-access", &aborted)
	for name, code := range map[string]string{
		"unknown key":      `console.sessions.list({bogus: 1})`,
		"misspelled key":   `console.sessions.list({projectId: "p"})`,
		"wrong type":       `console.sessions.list({project_id: 42})`,
		"unknown on read":  `console.sessions.read({session_id: "x", nope: true})`,
		"limit as string":  `console.sessions.read({session_id: "x", limit: "5"})`,
		"limit fractional": `console.sessions.read({session_id: "x", limit: 2.5})`,
	} {
		if err := evalErr(runtime, code); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
	err := evalErr(runtime, `console.sessions.list({projectId: "p"})`)
	if err == nil || !strings.Contains(err.Error(), "project_id, cwd") {
		t.Fatalf("err = %v, want the accepted argument names listed", err)
	}
	// Valid calls are unaffected (an empty list renders as "(no output)", so
	// only the absence of an error is checked here).
	for _, code := range []string{`console.sessions.list({cwd: "/tmp"})`, `console.sessions.list()`} {
		if err := evalErr(runtime, code); err != nil {
			t.Errorf("%s: %v", code, err)
		}
	}
}

func TestMethodArgsCoverEveryInstalledMethod(t *testing.T) {
	runtime, _ := newConsoleSessionRuntime(t, nil)
	_ = runtime
	// Every method must be listed so none escapes argument checking by omission.
	for _, name := range []string{
		"ports.list", "ports.forward", "ports.unforward", "scripts.list", "scripts.run",
		"scripts.status", "scripts.output", "scripts.stop", "models", "approvalModes",
		"projects.list", "projects.add", "sessions.list", "sessions.read", "sessions.create",
		"sessions.delete", "sessions.abort", "sessions.set_model", "sessions.set_approval",
		"sessions.prompt", "sessions.queue", "sessions.clear_queue", "sessions.fork",
	} {
		if _, ok := methodArgs[name]; !ok {
			t.Errorf("%s is not in methodArgs", name)
		}
	}
}

func TestCreateWithPromptReportsRunning(t *testing.T) {
	var aborted []string
	runtime, _ := newControlRuntime(t, "full-access", &aborted)
	runtime.start = func(string, string, string, string, string, string) error { return nil }
	out := evalJSON(t, runtime, `console.sessions.create({cwd:"/tmp", prompt:"go"})`).(map[string]any)
	if out["started"] != true {
		t.Fatalf("started = %v", out["started"])
	}
	if out["session"].(map[string]any)["status"] != "working" {
		t.Fatalf("status = %v, want working for a session whose run just started", out["session"].(map[string]any)["status"])
	}
	// No prompt: nothing started, still idle.
	idle := evalJSON(t, runtime, `console.sessions.create({cwd:"/tmp"})`).(map[string]any)
	if idle["started"] != false || idle["session"].(map[string]any)["status"] != "idle" {
		t.Fatalf("idle create = %#v", idle)
	}
}

func TestQueueViewAndClear(t *testing.T) {
	var aborted []string
	runtime, _ := newControlRuntime(t, "full-access", &aborted)
	staged := map[string]string{"s1": "run the tests"}
	runtime.queue = QueueControl{
		Get: func(id string) (string, string, bool, error) {
			p, ok := staged[id]
			return p, "2026-01-01T00:00:00Z", ok, nil
		},
		Clear: func(id string) (bool, error) {
			_, ok := staged[id]
			delete(staged, id)
			return ok, nil
		},
	}
	q := evalJSON(t, runtime, `console.sessions.queue({session_id:"s1"})`).(map[string]any)
	if q["queued"] != true || q["prompt"] != "run the tests" {
		t.Fatalf("queue = %#v", q)
	}
	c := evalJSON(t, runtime, `console.sessions.clear_queue({session_id:"s1"})`).(map[string]any)
	if c["cleared"] != true {
		t.Fatalf("clear = %#v", c)
	}
	after := evalJSON(t, runtime, `console.sessions.queue({session_id:"s1"})`).(map[string]any)
	if after["queued"] != false {
		t.Fatalf("queue after clear = %#v", after)
	}
	again := evalJSON(t, runtime, `console.sessions.clear_queue({session_id:"s1"})`).(map[string]any)
	if again["cleared"] != false {
		t.Fatalf("clearing an empty queue = %#v", again)
	}
	if err := evalErr(runtime, `console.sessions.queue({})`); err == nil {
		t.Fatal("session_id is required")
	}
}

func TestUnknownMethodNamesTheValidOnes(t *testing.T) {
	var aborted []string
	runtime, _ := newControlRuntime(t, "full-access", &aborted)
	err := evalErr(runtime, `console.sessions.nope({})`)
	if err == nil || !strings.Contains(err.Error(), "console.sessions.nope does not exist") || !strings.Contains(err.Error(), "prompt") {
		t.Fatalf("err = %v, want the valid session methods listed", err)
	}
	err = evalErr(runtime, `console.nope()`)
	if err == nil || !strings.Contains(err.Error(), "sessions") {
		t.Fatalf("err = %v, want the valid namespaces listed", err)
	}
	// The guard must not disturb ordinary use of the objects.
	for _, code := range []string{
		`Object.keys(console.sessions).length`,
		`typeof console.sessions.list`,
		`JSON.stringify(console.approvalModes())`,
	} {
		if err := evalErr(runtime, code); err != nil {
			t.Errorf("%s: %v", code, err)
		}
	}
}

func TestTimeoutMessageSaysWhatToCheck(t *testing.T) {
	var aborted []string
	runtime, _ := newControlRuntime(t, "full-access", &aborted)
	_, err := runtime.Eval(context.Background(), `while(true){}`, 50*time.Millisecond)
	if err == nil || !strings.Contains(err.Error(), "console.sessions.list()") || !strings.Contains(err.Error(), "console.scripts.status()") {
		t.Fatalf("err = %v, want the read methods to check named", err)
	}
}

// ---- sessions.wait

// activeSet is a thread-safe stand-in for "which sessions have a live run".
type activeSet struct {
	mu  sync.Mutex
	ids map[string]bool
}

func newActiveSet(ids ...string) *activeSet {
	a := &activeSet{ids: map[string]bool{}}
	for _, id := range ids {
		a.ids[id] = true
	}
	return a
}
func (a *activeSet) has(id string) bool { a.mu.Lock(); defer a.mu.Unlock(); return a.ids[id] }
func (a *activeSet) finish(id string)   { a.mu.Lock(); delete(a.ids, id); a.mu.Unlock() }

func newWaitRuntime(t *testing.T, ceiling string, active *activeSet) (*Runtime, *services.SessionService) {
	t.Helper()
	runtime, sessions := newConsoleSessionRuntime(t, active.has)
	runtime.ceiling = ceiling
	runtime.sessionID = "caller"
	return runtime, sessions
}

func TestWaitReturnsImmediatelyWhenAlreadyFinished(t *testing.T) {
	active := newActiveSet()
	runtime, sessions := newWaitRuntime(t, "full-access", active)
	target, _ := sessions.Create(newSource("always-ask"))
	out := evalJSON(t, runtime, `console.sessions.wait({session_id:"`+target.ID+`"})`).(map[string]any)
	if out["done"] != true || out["timed_out"] != false {
		t.Fatalf("a finished session must return done: %#v", out)
	}
	if ms, _ := out["waited_ms"].(float64); ms > 1000 {
		t.Fatalf("waited %vms for a session that was already finished", ms)
	}
}

func TestWaitBlocksUntilTheSessionFinishes(t *testing.T) {
	active := newActiveSet("busy")
	runtime, sessions := newWaitRuntime(t, "full-access", active)
	target, _ := sessions.Create(types.CreateSessionOptions{ID: "busy", Title: "b", Cwd: "/tmp"})
	if target.ID != "busy" {
		t.Fatalf("id = %q", target.ID)
	}
	go func() {
		time.Sleep(400 * time.Millisecond)
		active.finish("busy")
	}()
	start := time.Now()
	out := evalJSON(t, runtime, `console.sessions.wait({session_id:"busy", timeout_ms: 10000})`).(map[string]any)
	if out["done"] != true || out["timed_out"] != false {
		t.Fatalf("expected done after the run ended: %#v", out)
	}
	if elapsed := time.Since(start); elapsed < 300*time.Millisecond || elapsed > 5*time.Second {
		t.Fatalf("returned after %v; expected to block for roughly 400ms", elapsed)
	}
}

func TestWaitTimesOutAndSaysToWaitAgain(t *testing.T) {
	active := newActiveSet("forever")
	runtime, sessions := newWaitRuntime(t, "full-access", active)
	_, _ = sessions.Create(types.CreateSessionOptions{ID: "forever", Title: "f", Cwd: "/tmp"})
	out := evalJSON(t, runtime, `console.sessions.wait({session_id:"forever", timeout_ms: 300})`).(map[string]any)
	if out["done"] != false || out["timed_out"] != true {
		t.Fatalf("expected a timeout: %#v", out)
	}
	if note, _ := out["note"].(string); !strings.Contains(note, "wait again") {
		t.Fatalf("a timeout must tell the caller what to do: %#v", out)
	}
	first := out["sessions"].([]any)[0].(map[string]any)
	if first["status"] != "working" || first["done"] != false {
		t.Fatalf("a still-running session must report working: %#v", first)
	}
}

func TestWaitAnyVersusAll(t *testing.T) {
	active := newActiveSet("a", "b")
	runtime, sessions := newWaitRuntime(t, "full-access", active)
	for _, id := range []string{"a", "b"} {
		_, _ = sessions.Create(types.CreateSessionOptions{ID: id, Title: id, Cwd: "/tmp"})
	}
	go func() { time.Sleep(300 * time.Millisecond); active.finish("a") }()

	// "any" returns as soon as one finishes, while the other is still going.
	anyOut := evalJSON(t, runtime, `console.sessions.wait({session_ids:["a","b"], mode:"any", timeout_ms: 5000})`).(map[string]any)
	if anyOut["done"] != true {
		t.Fatalf("any: %#v", anyOut)
	}
	byID := map[string]map[string]any{}
	for _, s := range anyOut["sessions"].([]any) {
		m := s.(map[string]any)
		byID[m["session_id"].(string)] = m
	}
	if byID["a"]["done"] != true || byID["b"]["done"] != false {
		t.Fatalf("any must report a finished and b still running: %#v", byID)
	}

	// "all" with b still running times out rather than returning early.
	allOut := evalJSON(t, runtime, `console.sessions.wait({session_ids:["a","b"], mode:"all", timeout_ms: 300})`).(map[string]any)
	if allOut["done"] != false || allOut["timed_out"] != true {
		t.Fatalf("all must wait for every session: %#v", allOut)
	}
	go func() { time.Sleep(200 * time.Millisecond); active.finish("b") }()
	allOut = evalJSON(t, runtime, `console.sessions.wait({session_ids:["a","b"], timeout_ms: 5000})`).(map[string]any)
	if allOut["done"] != true {
		t.Fatalf("all must return once every session has finished: %#v", allOut)
	}
}

func TestWaitRefusals(t *testing.T) {
	active := newActiveSet()
	runtime, sessions := newWaitRuntime(t, "full-access", active)
	target, _ := sessions.Create(newSource("always-ask"))
	many := make([]string, 25)
	for i := range many {
		many[i] = fmt.Sprintf("%q", fmt.Sprintf("s%d", i))
	}
	for name, code := range map[string]string{
		"own session":        `console.sessions.wait({session_id:"caller"})`,
		"own among several":  `console.sessions.wait({session_ids:["` + target.ID + `","caller"]})`,
		"unknown session":    `console.sessions.wait({session_id:"ghost"})`,
		"no id":              `console.sessions.wait({})`,
		"empty list":         `console.sessions.wait({session_ids:[]})`,
		"too many":           `console.sessions.wait({session_ids:[` + strings.Join(many, ",") + `]})`,
		"bad mode":           `console.sessions.wait({session_id:"` + target.ID + `", mode:"first"})`,
		"negative timeout":   `console.sessions.wait({session_id:"` + target.ID + `", timeout_ms:-5})`,
		"timeout as string":  `console.sessions.wait({session_id:"` + target.ID + `", timeout_ms:"5"})`,
		"ids not an array":   `console.sessions.wait({session_ids:"abc"})`,
		"non-string in list": `console.sessions.wait({session_ids:[1,2]})`,
		"unknown argument":   `console.sessions.wait({session_id:"` + target.ID + `", wait:true})`,
	} {
		if err := evalErr(runtime, code); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
}

func TestWaitTimeoutIsCapped(t *testing.T) {
	if got := clampWait(0, false); got != waitDefault {
		t.Errorf("default = %v", got)
	}
	if got := clampWait(0, true); got != waitDefault {
		t.Errorf("explicit zero = %v, want the default", got)
	}
	if got := clampWait(10*60*1000, true); got != waitMax {
		t.Errorf("a ten-minute request must be capped at %v, got %v", waitMax, got)
	}
	if got := clampWait(1500, true); got != 1500*time.Millisecond {
		t.Errorf("in-range value changed: %v", got)
	}
	if waitMax >= 5*time.Minute {
		t.Fatalf("one wait (%v) must fit inside the kernel's own timeout", waitMax)
	}
}

func TestWaitDeduplicatesIDs(t *testing.T) {
	active := newActiveSet()
	runtime, sessions := newWaitRuntime(t, "full-access", active)
	target, _ := sessions.Create(newSource("always-ask"))
	out := evalJSON(t, runtime, `console.sessions.wait({session_id:"`+target.ID+`", session_ids:["`+target.ID+`","`+target.ID+`"]})`).(map[string]any)
	if n := len(out["sessions"].([]any)); n != 1 {
		t.Fatalf("a repeated id must be reported once, got %d entries", n)
	}
}

func TestWaitReturnsTheLastReply(t *testing.T) {
	active := newActiveSet()
	runtime, sessions := newWaitRuntime(t, "full-access", active)
	target, _ := sessions.Create(newSource("always-ask"))
	for _, m := range []types.AgentMessage{
		{ID: "u1", Role: "user", Data: []byte(`{"user":{"content":"run the tests"}}`)},
		{ID: "a1", Role: "assistant", Data: []byte(`{"assistant":{"id":"a1","content":[{"text":{"text":"All 42 tests passed."}}],"stopReason":"stop"}}`)},
		// A trailing tool-call-only turn must not hide the last words.
		{ID: "a2", Role: "assistant", Data: []byte(`{"assistant":{"id":"a2","content":[{"toolCall":{"id":"c","name":"bash"}}],"stopReason":"toolUse"}}`)},
	} {
		if err := sessions.AppendMessage(target.ID, m); err != nil {
			t.Fatal(err)
		}
	}
	out := evalJSON(t, runtime, `console.sessions.wait({session_id:"`+target.ID+`"})`).(map[string]any)
	first := out["sessions"].([]any)[0].(map[string]any)
	if first["last_reply"] != "All 42 tests passed." {
		t.Fatalf("last_reply = %#v", first["last_reply"])
	}
}

func TestWaitCutsAVeryLongReply(t *testing.T) {
	active := newActiveSet()
	runtime, sessions := newWaitRuntime(t, "full-access", active)
	target, _ := sessions.Create(newSource("always-ask"))
	long := strings.Repeat("word ", replyMaxText) // far over the cap
	data, _ := json.Marshal(map[string]any{"assistant": map[string]any{
		"id": "a1", "stopReason": "stop",
		"content": []any{map[string]any{"text": map[string]any{"text": long}}},
	}})
	_ = sessions.AppendMessage(target.ID, types.AgentMessage{ID: "a1", Role: "assistant", Data: data})
	out := evalJSON(t, runtime, `console.sessions.wait({session_id:"`+target.ID+`"})`).(map[string]any)
	reply := out["sessions"].([]any)[0].(map[string]any)["last_reply"].(string)
	if !strings.Contains(reply, "reply cut") || len(reply) > replyMaxText+300 {
		t.Fatalf("a long reply must be cut with a note (len %d)", len(reply))
	}
}

func TestWaitStopsWhenTheCallIsCancelled(t *testing.T) {
	active := newActiveSet("busy")
	runtime, sessions := newWaitRuntime(t, "full-access", active)
	_, _ = sessions.Create(types.CreateSessionOptions{ID: "busy", Title: "b", Cwd: "/tmp"})
	ctx, cancel := context.WithCancel(context.Background())
	go func() { time.Sleep(200 * time.Millisecond); cancel() }()
	start := time.Now()
	_, err := runtime.Eval(ctx, `console.sessions.wait({session_id:"busy", timeout_ms: 60000})`, 0)
	if err == nil {
		t.Fatal("a cancelled wait must fail, not report success")
	}
	if time.Since(start) > 10*time.Second {
		t.Fatalf("cancellation took %v; the wait ignored it", time.Since(start))
	}
}
