// Kernel tests run against a fake desktop: the handler serves canned tab
// state over the same BrowserActionRequest bridge the real client answers,
// so no browser is needed.
package browseruse

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/tools"
)

// fakeDesktop is an in-memory desktop client: a chat's tabs, an agent target,
// and a record of the actions it received.
type fakeDesktop struct {
	mu      sync.Mutex
	next    int
	tabs    map[string]*tabRow
	target  string // the chat's agent target tab
	actions []tools.BrowserActionRequest
}

func newFakeDesktop() *fakeDesktop {
	return &fakeDesktop{tabs: map[string]*tabRow{}}
}

// userTab adds a tab the kernel will never own (opened by the user).
func (f *fakeDesktop) userTab(url string) string {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.next++
	id := fmt.Sprintf("t%d", f.next)
	f.tabs[id] = &tabRow{ID: id, URL: url, Title: "User tab"}
	return id
}

func (f *fakeDesktop) handler() tools.BrowserHandler {
	return func(ctx context.Context, req tools.BrowserActionRequest) (tools.BrowserActionResult, error) {
		f.mu.Lock()
		defer f.mu.Unlock()
		f.actions = append(f.actions, req)
		switch req.Action {
		case "navigate":
			id := req.TabID
			if req.NewTab || id == "" {
				f.next++
				id = fmt.Sprintf("t%d", f.next)
			}
			row, ok := f.tabs[id]
			if !ok {
				row = &tabRow{ID: id}
				f.tabs[id] = row
			}
			row.URL = req.URL
			row.Title = "Page " + req.URL
			f.target = id
			return tools.BrowserActionResult{Result: fmt.Sprintf("Tab: %s\nTitle: %s\nURL: %s", id, row.Title, row.URL)}, nil
		case "tabs_json":
			rows := make([]tabRow, 0, len(f.tabs))
			for _, row := range f.tabs {
				out := *row
				out.Active = row.ID == f.target
				rows = append(rows, out)
			}
			raw, _ := json.Marshal(rows)
			return tools.BrowserActionResult{Result: string(raw)}, nil
		case "close_tab":
			if _, ok := f.tabs[req.TabID]; !ok {
				return tools.BrowserActionResult{Error: "no such tab " + req.TabID}, nil
			}
			delete(f.tabs, req.TabID)
			if f.target == req.TabID {
				f.target = ""
			}
			return tools.BrowserActionResult{Result: "closed " + req.TabID}, nil
		case "snapshot":
			return tools.BrowserActionResult{Result: "Snapshot: 1 nodes\n- button \"Go\" [ref=e1]"}, nil
		case "query":
			if req.Selector == "missing" {
				return tools.BrowserActionResult{Result: "No visible match for selector: missing"}, nil
			}
			return tools.BrowserActionResult{Result: `{"selector": ".r", "count": 1, "items": [{"tag": "div", "text": "Item"}]}`}, nil
		case "click", "type", "wait_for":
			row := f.tabs[req.TabID]
			verb := map[string]string{"click": "Clicked", "type": "Typed into", "wait_for": "Condition met after 1ms"}[req.Action]
			return tools.BrowserActionResult{Result: fmt.Sprintf("%s\nTab: %s\nTitle: %s\nURL: %s", verb, req.TabID, row.Title, row.URL)}, nil
		}
		return tools.BrowserActionResult{Error: "unsupported action " + req.Action}, nil
	}
}

func (f *fakeDesktop) tabIDs() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	ids := make([]string, 0, len(f.tabs))
	for id := range f.tabs {
		ids = append(ids, id)
	}
	return ids
}

func (f *fakeDesktop) sawNewTab() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, req := range f.actions {
		if req.Action == "navigate" && req.NewTab {
			return true
		}
	}
	return false
}

// eval executes code in the session's runtime, as the tool would.
func eval(t *testing.T, m *Manager, session string, desktop *fakeDesktop, code string) (any, error) {
	t.Helper()
	rt, err := m.runtime(session, desktop.handler())
	if err != nil {
		t.Fatalf("runtime: %v", err)
	}
	return rt.Eval(context.Background(), code, 5*time.Second)
}

func mustEval(t *testing.T, m *Manager, session string, desktop *fakeDesktop, code string) any {
	t.Helper()
	out, err := eval(t, m, session, desktop, code)
	if err != nil {
		t.Fatalf("Eval(%q) failed: %v", code, err)
	}
	return out
}

// completionText extracts the rendered completion: the final text part minus
// any trailing trace line. That is the JSON (or plain text) the model reads.
func completionText(t *testing.T, out any) string {
	t.Helper()
	parts, ok := out.([]map[string]any)
	if !ok {
		t.Fatalf("eval returned %#v, want content parts", out)
	}
	var lines []string
	for _, part := range parts {
		if part["type"] == "text" {
			lines = append(lines, part["text"].(string))
		}
	}
	joined := strings.Join(lines, "\n")
	if i := strings.Index(joined, "\nTrace: ["); i >= 0 {
		joined = joined[:i]
	}
	return joined
}

// evalJSON runs code and decodes the rendered completion as JSON.
func evalJSON(t *testing.T, m *Manager, session string, desktop *fakeDesktop, code string) map[string]any {
	t.Helper()
	text := completionText(t, mustEval(t, m, session, desktop, code))
	var decoded map[string]any
	if err := json.Unmarshal([]byte(text), &decoded); err != nil {
		t.Fatalf("completion of %q is not JSON: %q (%v)", code, text, err)
	}
	return decoded
}

func reset(t *testing.T, m *Manager, session string, desktop *fakeDesktop, tabID string) (any, error) {
	t.Helper()
	tool := NewResetTool(m, session, desktop.handler())
	raw, _ := json.Marshal(browserResetInput{TabID: tabID})
	return tool.Execute(context.Background(), raw)
}

func TestOpenIsKernelOwnedAndFresh(t *testing.T) {
	m := NewManager()
	desktop := newFakeDesktop()
	userTab := desktop.userTab("https://user.example.com")

	row := evalJSON(t, m, "s1", desktop, `browser.open({url: "https://example.com"})`)
	tabID := row["tabId"].(string)
	if !desktop.sawNewTab() {
		t.Fatal("open did not force a new tab")
	}
	if tabID == userTab {
		t.Fatal("open reused the user's tab")
	}
	if !m.isOwned("s1", tabID) {
		t.Fatalf("opened tab %s is not owned", tabID)
	}
	if row["url"] != "https://example.com" {
		t.Fatalf("open reported url %v", row["url"])
	}
	// Same URL again must open another fresh tab, not reuse.
	next := evalJSON(t, m, "s1", desktop, `browser.open({url: "https://example.com"})`)
	if next["tabId"] == tabID {
		t.Fatal("second open reused the first kernel tab")
	}
}

func TestStatePersistsAndIsolates(t *testing.T) {
	m := NewManager()
	desktop := newFakeDesktop()
	mustEval(t, m, "s1", desktop, `var count = 0; count = count + 1;`)
	mustEval(t, m, "s1", desktop, `count = count + 1;`)
	if text := completionText(t, mustEval(t, m, "s1", desktop, `count`)); text != "2" {
		t.Fatalf("variables did not persist: %q", text)
	}
	// A different session shares nothing: no variables, no owned tabs.
	if _, err := eval(t, m, "s2", desktop, `count`); err == nil {
		t.Fatal("s2 saw s1's variables")
	}
	if len(m.ownedTabs("s2")) != 0 {
		t.Fatalf("s2 owns tabs it never opened: %v", m.ownedTabs("s2"))
	}
}

func TestSyncOnly(t *testing.T) {
	m := NewManager()
	desktop := newFakeDesktop()
	// A Promise completion would never settle (no microtask pump), so the
	// runtime refuses it with the async message.
	if _, err := eval(t, m, "s1", desktop, `Promise.resolve(1).then(function (x) { return x })`); err == nil {
		t.Fatal("a Promise completion should fail")
	} else if !strings.Contains(err.Error(), "async") {
		t.Fatalf("Promise error should mention async, got: %v", err)
	}
}

func TestActionsTargetActiveTab(t *testing.T) {
	m := NewManager()
	desktop := newFakeDesktop()
	opened := evalJSON(t, m, "s1", desktop, `browser.open({url: "https://example.com"})`)
	res := evalJSON(t, m, "s1", desktop, `browser.click({selector: "#go"})`)
	if res["tabId"] != opened["tabId"] {
		t.Fatalf("click hit %v, want the opened tab %v", res["tabId"], opened["tabId"])
	}
	if res["url"] != "https://example.com" {
		t.Fatalf("click reported url %v", res["url"])
	}
}

func TestQueryParsesJSONAndThrowsOnNoMatch(t *testing.T) {
	m := NewManager()
	desktop := newFakeDesktop()
	mustEval(t, m, "s1", desktop, `browser.open({url: "https://example.com"})`)
	text := completionText(t, mustEval(t, m, "s1", desktop, `browser.query({selector: ".r"}).items[0].text`))
	if text != `"Item"` {
		t.Fatalf("query returned %q, want the item text", text)
	}
	if _, err := eval(t, m, "s1", desktop, `browser.query({selector: "missing"})`); err == nil || !strings.Contains(err.Error(), "No visible match") {
		t.Fatalf("no-match query should throw the driver's message, got: %v", err)
	}
}

func TestCloseRefusesUserTabs(t *testing.T) {
	m := NewManager()
	desktop := newFakeDesktop()
	userTab := desktop.userTab("https://user.example.com")
	code := fmt.Sprintf(`browser.close({tabId: %q})`, userTab)
	if _, err := eval(t, m, "s1", desktop, code); err == nil || !strings.Contains(err.Error(), "not opened by this kernel") {
		t.Fatalf("closing a user tab should fail, got: %v", err)
	}
	if ids := desktop.tabIDs(); len(ids) != 1 {
		t.Fatalf("user tab was closed: %v", ids)
	}
}

func TestResetClosesOnlyOwnedTabs(t *testing.T) {
	m := NewManager()
	desktop := newFakeDesktop()
	userTab := desktop.userTab("https://user.example.com")
	mustEval(t, m, "s1", desktop, `browser.open({url: "https://example.com"})`)
	mustEval(t, m, "s1", desktop, `browser.open({url: "https://two.example.com"})`)

	out, err := reset(t, m, "s1", desktop, "")
	if err != nil {
		t.Fatalf("reset: %v", err)
	}
	res := out.(map[string]any)
	if len(res["closed"].([]string)) != 2 {
		t.Fatalf("reset closed %v, want both kernel tabs", res["closed"])
	}
	if ids := desktop.tabIDs(); len(ids) != 1 || ids[0] != userTab {
		t.Fatalf("reset did not keep exactly the user tab: %v", ids)
	}
	// Variables are gone with the runtime: a fresh eval starts empty.
	if text := completionText(t, mustEval(t, m, "s1", desktop, `typeof probe`)); text != `"undefined"` {
		t.Fatalf("full reset kept variables: %s", text)
	}
	if len(m.ownedTabs("s1")) != 0 {
		t.Fatalf("owned set not cleared: %v", m.ownedTabs("s1"))
	}
}

func TestTargetedResetKeepsVariables(t *testing.T) {
	m := NewManager()
	desktop := newFakeDesktop()
	mustEval(t, m, "s1", desktop, `browser.open({url: "https://one.example.com"})`)
	opened := evalJSON(t, m, "s1", desktop, `browser.open({url: "https://two.example.com"})`)
	mustEval(t, m, "s1", desktop, `var kept = 41; kept`)

	if _, err := reset(t, m, "s1", desktop, opened["tabId"].(string)); err != nil {
		t.Fatalf("targeted reset: %v", err)
	}
	if ids := desktop.tabIDs(); len(ids) != 1 {
		t.Fatalf("targeted reset left %v tabs, want one", ids)
	}
	if text := completionText(t, mustEval(t, m, "s1", desktop, `kept + 1`)); text != "42" {
		t.Fatalf("variables did not survive a targeted reset: %s", text)
	}
	// A tab the kernel does not own is refused.
	if _, err := reset(t, m, "s1", desktop, "t99"); err == nil {
		t.Fatal("targeted reset of an unknown tab should fail")
	}
}

func TestCloseAllKeepsRuntime(t *testing.T) {
	m := NewManager()
	desktop := newFakeDesktop()
	mustEval(t, m, "s1", desktop, `browser.open({url: "https://example.com"})`)
	mustEval(t, m, "s1", desktop, `browser.closeAll()`)
	if ids := desktop.tabIDs(); len(ids) != 0 {
		t.Fatalf("closeAll left tabs: %v", ids)
	}
	if text := completionText(t, mustEval(t, m, "s1", desktop, `var kept = "kept"; kept`)); text != `"kept"` {
		t.Fatalf("closeAll dropped the runtime: %s", text)
	}
}

func TestOpenTabCap(t *testing.T) {
	m := NewManager()
	desktop := newFakeDesktop()
	for i := 0; i < maxOwnedTabs; i++ {
		mustEval(t, m, "s1", desktop, fmt.Sprintf(`browser.open({url: "https://%d.example.com"})`, i))
	}
	_, err := eval(t, m, "s1", desktop, `browser.open({url: "https://six.example.com"})`)
	if err == nil || !strings.Contains(err.Error(), "5 tabs") {
		t.Fatalf("sixth open should hit the cap, got: %v", err)
	}
	// The tab the failed open created must be gone again.
	if len(desktop.tabIDs()) != maxOwnedTabs {
		t.Fatalf("failed open left tabs behind: %v", desktop.tabIDs())
	}
}

func TestActionBudgetAndTraceCap(t *testing.T) {
	m := NewManager()
	desktop := newFakeDesktop()
	_, err := eval(t, m, "s1", desktop, `for (var i = 0; i < 101; i++) browser.tabs();`)
	if err == nil || !strings.Contains(err.Error(), "action limit") {
		t.Fatalf("over-budget evaluation should fail with the limit, got: %v", err)
	}
	if !strings.Contains(err.Error(), "Trace: [") {
		t.Fatal("a failed evaluation should carry the trace so far")
	}
	rt, _ := m.runtime("s1", desktop.handler())
	if trace := rt.traceText(); strings.Count(trace, `"action"`) > maxTraceEntries {
		t.Fatalf("trace exceeded its cap")
	}
}

func TestTraceRidesTheResult(t *testing.T) {
	m := NewManager()
	desktop := newFakeDesktop()
	out := mustEval(t, m, "s1", desktop, `browser.open({url: "https://example.com"})`).([]map[string]any)
	last := out[len(out)-1]
	if last["type"] != "text" || !strings.HasPrefix(last["text"].(string), "Trace: [") {
		t.Fatalf("trace part missing from result: %#v", last)
	}
}

func TestSplitInvocation(t *testing.T) {
	if task, ok := SplitInvocation("/browser-use research the docs"); !ok || task != "research the docs" {
		t.Fatalf("split failed: %q %v", task, ok)
	}
	if _, ok := SplitInvocation("/browser-useful things"); ok {
		t.Fatal("/browser-useful must not trigger")
	}
	if _, ok := SplitInvocation("see /browser-use for details"); ok {
		t.Fatal("inline mention must not trigger")
	}
	if _, ok := SplitInvocation("/browser-use"); !ok {
		t.Fatal("bare command should activate")
	}
}
