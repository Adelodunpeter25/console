// Kernel tests run against a fake desktop: the handler serves canned tab
// state over the same BrowserActionRequest bridge the real client answers,
// so no browser is needed.
package browseruse

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
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

	// Pagination simulation: when pages is set, `extract` serves
	// pages[page] and a `text=Next` click advances the page. lastExtractJSON
	// makes waitForNew report unchanged when the page's content did not
	// change — the same signature idea the real driver uses.
	pages           [][]map[string]any
	page            int
	lastExtractJSON string
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
			if req.Action == "click" && len(f.pages) > 0 && req.Selector == "text=Next" {
				// Pagination simulation: the next control disappears on the
				// last page, like a disabled button would.
				if f.page >= len(f.pages)-1 {
					return tools.BrowserActionResult{Error: "Could not click text \"Next\": Element is disabled."}, nil
				}
				f.page++
			}
			row := f.tabs[req.TabID]
			verb := map[string]string{"click": "Clicked", "type": "Typed into", "wait_for": "Condition met after 1ms"}[req.Action]
			return tools.BrowserActionResult{Result: fmt.Sprintf("%s\nTab: %s\nTitle: %s\nURL: %s", verb, req.TabID, row.Title, row.URL)}, nil
		case "extract":
			var spec struct {
				Scope      string         `json:"scope"`
				Items      string         `json:"items"`
				Limit      int            `json:"limit"`
				WaitForNew bool           `json:"waitForNew"`
				WaitMs     int            `json:"waitMs"`
				Fields     map[string]any `json:"fields"`
			}
			if err := json.Unmarshal([]byte(req.Script), &spec); err != nil {
				return tools.BrowserActionResult{Error: "Invalid extract spec: " + err.Error()}, nil
			}
			if len(f.pages) == 0 {
				// Plain extract: one canned row echoing the requested fields.
				item := map[string]any{"title": "Item", "href": "https://example.com/item"}
				raw, _ := json.Marshal(map[string]any{"count": 1, "items": []any{item}})
				return tools.BrowserActionResult{Result: string(raw)}, nil
			}
			current, _ := json.Marshal(f.pages[f.page])
			unchanged := spec.WaitForNew && string(current) == f.lastExtractJSON
			f.lastExtractJSON = string(current)
			payload := map[string]any{"count": len(f.pages[f.page]), "items": f.pages[f.page]}
			if unchanged {
				payload["unchanged"] = true
			}
			raw, _ := json.Marshal(payload)
			return tools.BrowserActionResult{Result: string(raw)}, nil
		case "scroll":
			return tools.BrowserActionResult{Result: `{"scrollY": 800, "scrollHeight": 4000, "viewportHeight": 800, "atTop": false, "atBottom": false}`}, nil
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

func TestAsyncSupported(t *testing.T) {
	m := NewManager()
	desktop := newFakeDesktop()
	raw, err := eval(t, m, "s1", desktop, `Promise.resolve(42).then(function (x) { return x })`)
	if err != nil {
		t.Fatalf("Promise completion failed: %v", err)
	}
	parts, ok := raw.([]map[string]any)
	if !ok || len(parts) == 0 || parts[0]["text"] != "42" {
		t.Fatalf("expected 42, got: %#v", raw)
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

func TestEveryInstalledMethodIsInTheArgumentTable(t *testing.T) {
	// The installed-method list comes from the source the same way the
	// console kernel's test does: a methods() entry not in methodArgs would
	// escape argument checking by omission.
	src, err := os.ReadFile("js.go")
	if err != nil {
		t.Fatal(err)
	}
	re := regexp.MustCompile(`\{Name: "([a-zA-Z]+)", Call:`)
	installed := map[string]bool{}
	for _, m := range re.FindAllStringSubmatch(string(src), -1) {
		installed[m[1]] = true
	}
	if len(installed) < 12 {
		t.Fatalf("found only %d installed methods; the pattern is out of date", len(installed))
	}
	for name := range installed {
		if _, ok := methodArgs[name]; !ok {
			t.Errorf("%s is installed but has no entry in methodArgs", name)
		}
	}
	for name := range methodArgs {
		if !installed[name] {
			t.Errorf("methodArgs lists %s, which is not an installed method", name)
		}
	}
}

func TestUnknownArgumentIsRefusedWithTheAcceptedList(t *testing.T) {
	m := NewManager()
	desktop := newFakeDesktop()
	_, err := eval(t, m, "s1", desktop, `browser.click({selector: "#go", text: "Go"})`)
	if err == nil {
		t.Fatal("two targets should be refused")
	}
	if !strings.Contains(err.Error(), "takes one target") {
		t.Fatalf("multi-target error should say so, got: %v", err)
	}
	_, err = eval(t, m, "s1", desktop, `browser.open({url: "https://example.com", projectId: "x"})`)
	if err == nil || !strings.Contains(err.Error(), `unknown argument "projectId"`) {
		t.Fatalf("typo should be refused with the accepted list, got: %v", err)
	}
	if !strings.Contains(err.Error(), "url") {
		t.Fatalf("error should name the accepted keys, got: %v", err)
	}
}

func TestExtractPassesSpecAndParsesResult(t *testing.T) {
	m := NewManager()
	desktop := newFakeDesktop()
	mustEval(t, m, "s1", desktop, `browser.open({url: "https://example.com"})`)
	res := evalJSON(t, m, "s1", desktop,
		`browser.extract({scope: "#buybox", items: ".offer", fields: {title: "h2"}, limit: 5})`)
	if res["count"].(float64) != 1 {
		t.Fatalf("extract count = %v", res["count"])
	}
	items := res["items"].([]any)
	title := items[0].(map[string]any)["title"]
	if title != "Item" {
		t.Fatalf("extract item title = %v", title)
	}
	// The spec must carry the validated fields through to the desktop.
	for _, req := range desktop.actions {
		if req.Action != "extract" {
			continue
		}
		if !strings.Contains(req.Script, `"scope":"#buybox"`) || !strings.Contains(req.Script, `"limit":5`) {
			t.Fatalf("extract spec lost its arguments: %s", req.Script)
		}
		return
	}
	t.Fatal("no extract action reached the desktop")
}

func TestExtractRejectsBadFieldSpecs(t *testing.T) {
	m := NewManager()
	desktop := newFakeDesktop()
	mustEval(t, m, "s1", desktop, `browser.open({url: "https://example.com"})`)
	if _, err := eval(t, m, "s1", desktop, `browser.extract({fields: {price: {selector: ".p", atrribute: "href"}}})`); err == nil || !strings.Contains(err.Error(), "field \"price\" takes") {
		t.Fatalf("field typo should name the accepted keys, got: %v", err)
	}
	if _, err := eval(t, m, "s1", desktop, `browser.extract({fields: {price: {attribute: "href"}}})`); err == nil || !strings.Contains(err.Error(), "needs a {selector}") {
		t.Fatalf("field without a selector should fail, got: %v", err)
	}
}

func TestScrollPassesSpecAndReturnsPosition(t *testing.T) {
	m := NewManager()
	desktop := newFakeDesktop()
	mustEval(t, m, "s1", desktop, `browser.open({url: "https://example.com"})`)
	res := evalJSON(t, m, "s1", desktop, `browser.scroll({direction: "down", amount: 300})`)
	if res["scrollY"].(float64) != 800 {
		t.Fatalf("scroll result = %v", res)
	}
	if res["tabId"] == "" {
		t.Fatal("scroll result lost the tab id")
	}
	if _, err := eval(t, m, "s1", desktop, `browser.scroll({direction: "sideways"})`); err == nil || !strings.Contains(err.Error(), "up, down, top or bottom") {
		t.Fatalf("bad direction should fail, got: %v", err)
	}
}

func TestPaginateWalksDedupesAndStops(t *testing.T) {
	m := NewManager()
	desktop := newFakeDesktop()
	mustEval(t, m, "s1", desktop, `browser.open({url: "https://example.com/list"})`)
	// Two pages where page 2 repeats one href (the same product listed twice
	// across pages) plus one new item; the next control then disappears.
	desktop.pages = [][]map[string]any{
		{{"title": "A", "href": "https://example.com/a"}, {"title": "B", "href": "https://example.com/b"}},
		{{"title": "B", "href": "https://example.com/b"}, {"title": "C", "href": "https://example.com/c"}},
	}
	code := `browser.paginate({
		itemSelector: ".result",
		next: {text: "Next"},
		maxPages: 5,
		extract: {title: "h2", href: {selector: "a", attribute: "href"}}
	})`
	res := evalJSON(t, m, "s1", desktop, code)
	if res["pages"].(float64) != 2 {
		t.Fatalf("pages = %v, want 2", res["pages"])
	}
	items := res["items"].([]any)
	if len(items) != 3 {
		t.Fatalf("items = %v, want 3 deduped", items)
	}
	first := items[0].(map[string]any)["title"]
	if first != "A" {
		t.Fatalf("first item = %v, want A (insertion order kept)", first)
	}
	if res["truncated"].(bool) {
		t.Fatal("a two-page walk is not truncated")
	}
	warnings, _ := res["warnings"].([]any)
	if len(warnings) == 0 || !strings.Contains(warnings[0].(string), "next") {
		t.Fatalf("the missing next control should be a warning, got: %v", res["warnings"])
	}
}

func TestPaginateStopsWhenThePageNeverChanges(t *testing.T) {
	m := NewManager()
	desktop := newFakeDesktop()
	mustEval(t, m, "s1", desktop, `browser.open({url: "https://example.com/list"})`)
	// Every "next" click lands on identical content: the extract comes back
	// unchanged and the walk stops with a warning instead of looping.
	page := []map[string]any{{"title": "A", "href": "https://example.com/a"}}
	desktop.pages = [][]map[string]any{page, page, page, page, page, page}
	res := evalJSON(t, m, "s1", desktop, `browser.paginate({
		itemSelector: ".result", next: {text: "Next"}, maxPages: 5,
		extract: {title: "h2", href: {selector: "a", attribute: "href"}}
	})`)
	if res["pages"].(float64) != 1 {
		t.Fatalf("pages = %v, want 1", res["pages"])
	}
	warnings, _ := res["warnings"].([]any)
	if len(warnings) == 0 || !strings.Contains(warnings[0].(string), "did not change") {
		t.Fatalf("the unchanged page should be a warning, got: %v", res["warnings"])
	}
	if res["truncated"].(bool) {
		t.Fatal("an unchanged repeat is not truncation")
	}
}

func TestFindHandleSurvivesRerendersAndDiesOnNavigation(t *testing.T) {
	m := NewManager()
	desktop := newFakeDesktop()
	mustEval(t, m, "s1", desktop, `browser.open({url: "https://shop.example.com"})`)
	found := evalJSON(t, m, "s1", desktop, `browser.find({text: "Add to cart"})`)
	handle := found["handle"].(string)
	if found["url"] != "https://shop.example.com" {
		t.Fatalf("find reported url %v", found["url"])
	}
	// A re-render (same URL) keeps the handle usable.
	if res := evalJSON(t, m, "s1", desktop, fmt.Sprintf(`browser.click({handle: %q})`, handle)); res["url"] != "https://shop.example.com" {
		t.Fatalf("handle click reported %v", res)
	}
	// The click must carry the encoded text locator, not the handle.
	for _, req := range desktop.actions {
		if req.Action == "click" && req.Selector != "" {
			if req.Selector != "text=Add to cart" {
				t.Fatalf("handle click sent selector %q", req.Selector)
			}
			break
		}
	}
	// A navigation (the tab's URL changes) makes every handle stale.
	mustEval(t, m, "s1", desktop, `browser.goto({url: "https://shop.example.com/other"})`)
	if _, err := eval(t, m, "s1", desktop, fmt.Sprintf(`browser.click({handle: %q})`, handle)); err == nil || !strings.Contains(err.Error(), "stale") {
		t.Fatalf("stale handle should be refused, got: %v", err)
	}
	// Unknown handles error too.
	if _, err := eval(t, m, "s1", desktop, `browser.click({handle: "h99"})`); err == nil || !strings.Contains(err.Error(), "unknown handle") {
		t.Fatalf("unknown handle should fail, got: %v", err)
	}
}
