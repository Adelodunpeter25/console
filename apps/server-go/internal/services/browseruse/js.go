// The browser-use JavaScript runtime: one persistent interpreter per chat
// session with a preinstalled `browser` object.
//
// The shape is the console kernel's: plain results (a call returns its data
// directly), one object argument per method, unknown members named in the
// error, and the API documented once in the /browser-use skill instead of as
// schema on every turn. Methods are synchronous — goja has no microtask pump,
// so a Promise would never settle — and every one blocks on the desktop
// client answering the underlying low-level action.
//
// The desktop stays the source of truth for tabs, URLs and titles; this
// runtime tracks only what the desktop cannot know: which tabs the kernel
// opened (ownership), which one actions target by default (active), the
// per-evaluation action budget, and the trace.
package browseruse

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/tools"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/services/jskernel"
)

// Limits for one evaluation.
const (
	// maxActionsPerEval bounds host round-trips per browser call,
	// counting the identity lookups alongside the visible actions.
	maxActionsPerEval = 100
	// maxTraceEntries caps the returned trace; the newest entries win.
	maxTraceEntries = 100
	// Default and maximum wait_for budget, matching the low-level tool.
	defaultWaitTimeoutMs = 10000
	maxWaitTimeoutMs     = 60000
)

// errOwnedTabCap is thrown when browser.open would exceed the tab cap.
const errOwnedTabCap = "this kernel already owns 5 tabs; close one with browser.close({tabId}) or clear them all with the reset tool before opening another"

// traceEntry records one host action for the result.
type traceEntry struct {
	Action string `json:"action"`
	Target string `json:"target,omitempty"`
	Status string `json:"status"`
	Ms     int64  `json:"ms"`
}

// tabRow is one tab as the desktop's tabs_json action reports it.
type tabRow struct {
	ID     string `json:"id"`
	URL    string `json:"url"`
	Title  string `json:"title"`
	Active bool   `json:"active"`
}

// JSRuntime is one session's kernel. Exported so tests can drive it without a
// chat session; the agent path goes through Manager and the tools.
type JSRuntime struct {
	kernel  *jskernel.Runtime
	manager *Manager
	session string
	handler tools.BrowserHandler

	// Guarded by mu: reset runs on the tool's goroutine while the rest is
	// touched from the evaluation goroutine inside method calls.
	mu      sync.Mutex
	trace   []traceEntry
	actions int
	active  string
}

// NewJSRuntime builds a runtime with the `browser` object installed.
func NewJSRuntime(manager *Manager, sessionID string, handler tools.BrowserHandler) (*JSRuntime, error) {
	r := &JSRuntime{manager: manager, session: sessionID, handler: handler}
	kernel, err := jskernel.New(jskernel.Options{
		Root:             "browser",
		PlainResults:     true,
		InterruptMessage: "execution stopped; a browser action in flight may already have taken effect. Inspect fresh state before retrying.",
		AsyncMessage:     "async code is not supported: call browser methods directly, they already wait for the result",
	}, r.methods())
	if err != nil {
		return nil, err
	}
	r.kernel = kernel
	return r, nil
}

// methods is the `browser` API. One object argument each, plain results.
func (r *JSRuntime) methods() []jskernel.Method {
	return []jskernel.Method{
		{Name: "open", Call: r.method(r.open)},
		{Name: "page", Call: r.method(r.page)},
		{Name: "tabs", Call: r.method(r.tabs)},
		{Name: "goto", Call: r.method(r.gotoTab)},
		{Name: "snapshot", Call: r.method(r.snapshot)},
		{Name: "content", Call: r.method(r.content)},
		{Name: "query", Call: r.method(r.query)},
		{Name: "click", Call: r.method(r.click)},
		{Name: "type", Call: r.method(r.typeText)},
		{Name: "waitFor", Call: r.method(r.waitFor)},
		{Name: "close", Call: r.method(r.close)},
		{Name: "closeAll", Call: r.method(r.closeAll)},
	}
}

// method adapts one API function to the kernel's method signature and counts
// it in the trace.
func (r *JSRuntime) method(fn func(ctx context.Context, args map[string]any) (any, error)) func(ctx context.Context, args map[string]any) (*jskernel.Result, error) {
	return func(ctx context.Context, args map[string]any) (*jskernel.Result, error) {
		out, err := fn(ctx, args)
		if err != nil {
			return nil, err
		}
		raw, marshalErr := json.Marshal(out)
		if marshalErr != nil {
			return nil, jskernel.Errorf("browser method result could not be serialized: %v", marshalErr)
		}
		return &jskernel.Result{StructuredContent: raw}, nil
	}
}

// Eval runs code in the session's kernel, resetting the per-evaluation budget
// and trace. A kernel error carries the trace so far, since an uncaught
// failure otherwise hides what already ran.
func (r *JSRuntime) Eval(ctx context.Context, code string, timeout time.Duration) (any, error) {
	r.mu.Lock()
	r.actions = 0
	r.trace = nil
	r.mu.Unlock()
	out, err := r.kernel.Eval(ctx, code, timeout)
	if err != nil {
		var kernelErr *jskernel.Error
		if errors.As(err, &kernelErr) {
			return nil, &jskernel.Error{Msg: kernelErr.Msg + "\n" + r.traceText()}
		}
		return nil, err
	}
	// The trace rides the result as one text part: the model sees what ran,
	// including the identity lookups, without re-printing it in code.
	if trace := r.traceText(); trace != "" {
		if parts, ok := out.([]map[string]any); ok {
			out = append(parts, map[string]any{"type": "text", "text": trace})
		}
	}
	return out, nil
}

// TraceText returns the trace as one JSON text line, for the tool result.
func (r *JSRuntime) traceText() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.trace) == 0 {
		return ""
	}
	raw, err := json.Marshal(r.trace)
	if err != nil {
		return ""
	}
	return "Trace: " + string(raw)
}

// setHandler rebinds the desktop bridge. Called when a later run fetches an
// existing runtime, so its actions ride that run's connection.
func (r *JSRuntime) setHandler(handler tools.BrowserHandler) {
	r.mu.Lock()
	r.handler = handler
	r.mu.Unlock()
}

// callAction performs one low-level browser action against the desktop
// client, counting it against the evaluation budget and the trace. Desktop
// failures throw as JavaScript errors so the model reads the client's own
// message and can react.
func (r *JSRuntime) callAction(ctx context.Context, req tools.BrowserActionRequest) (tools.BrowserActionResult, error) {
	r.mu.Lock()
	if r.actions >= maxActionsPerEval {
		r.mu.Unlock()
		return tools.BrowserActionResult{}, jskernel.Errorf("browser action limit reached (%d per call); split the work across browser calls", maxActionsPerEval)
	}
	r.actions++
	handler := r.handler
	r.mu.Unlock()

	start := time.Now()
	res, err := handler(ctx, req)
	ms := time.Since(start).Milliseconds()

	entry := traceEntry{Action: req.Action, Target: traceTarget(req), Ms: ms}
	switch {
	case err != nil:
		entry.Status = "error"
	case res.Error != "":
		entry.Status = "error"
	default:
		entry.Status = "success"
	}
	r.mu.Lock()
	r.trace = append(r.trace, entry)
	if len(r.trace) > maxTraceEntries {
		r.trace = r.trace[len(r.trace)-maxTraceEntries:]
	}
	r.mu.Unlock()

	if err != nil {
		return tools.BrowserActionResult{}, jskernel.Errorf("browser.%s failed: %v", req.Action, err)
	}
	if res.Error != "" {
		return tools.BrowserActionResult{}, jskernel.Errorf("browser.%s failed: %s", req.Action, res.Error)
	}
	return res, nil
}

// traceTarget names what an action aimed at, for the trace only.
func traceTarget(req tools.BrowserActionRequest) string {
	switch req.Action {
	case "navigate":
		return req.URL
	case "click", "type", "snapshot", "get_content", "query", "wait_for":
		if req.Ref != "" {
			return req.Ref
		}
		return req.Selector
	case "close_tab", "switch_tab":
		return req.TabID
	}
	return ""
}

// --- tab identity ---------------------------------------------------------

// tabsJSON lists this chat's tabs as structured rows.
func (r *JSRuntime) tabsJSON(ctx context.Context) ([]tabRow, error) {
	res, err := r.callAction(ctx, tools.BrowserActionRequest{Action: "tabs_json"})
	if err != nil {
		return nil, err
	}
	var rows []tabRow
	if err := json.Unmarshal([]byte(res.Result), &rows); err != nil {
		return nil, jskernel.Errorf("browser could not read the tab list: %v", err)
	}
	return rows, nil
}

// resolveTarget returns the row for the tab an action should hit: the
// requested tabId, else the kernel's active tab, else the chat's current
// agent target — and errors with the tab list when nothing applies.
func (r *JSRuntime) resolveTarget(ctx context.Context, tabID string) (tabRow, error) {
	rows, err := r.tabsJSON(ctx)
	if err != nil {
		return tabRow{}, err
	}
	if tabID != "" {
		for _, row := range rows {
			if row.ID == tabID {
				return row, nil
			}
		}
		return tabRow{}, jskernel.Errorf("no tab %q in this chat. Open tabs: %s", tabID, rowSummary(rows))
	}
	r.mu.Lock()
	active := r.active
	r.mu.Unlock()
	if active != "" {
		for _, row := range rows {
			if row.ID == active {
				return row, nil
			}
		}
		// The active tab is gone (closed or navigated away from the set);
		// fall through to the chat's current target.
	}
	for _, row := range rows {
		if row.Active {
			return row, nil
		}
	}
	return tabRow{}, jskernel.Errorf("no browser tab to act on. Open one with browser.open({url}). Open tabs: %s", rowSummary(rows))
}

// rowSummary renders a tab list for an error message.
func rowSummary(rows []tabRow) string {
	if len(rows) == 0 {
		return "(none)"
	}
	parts := make([]string, 0, len(rows))
	for _, row := range rows {
		parts = append(parts, fmt.Sprintf("%s (%s)", row.ID, row.URL))
	}
	return strings.Join(parts, ", ")
}

// setActive records the tab subsequent actions target by default.
func (r *JSRuntime) setActive(tabID string) {
	r.mu.Lock()
	r.active = tabID
	r.mu.Unlock()
}

// activeTabID returns the kernel's active tab, if any.
func (r *JSRuntime) activeTabID() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.active
}

// --- the browser object ----------------------------------------------------

func (r *JSRuntime) open(ctx context.Context, args map[string]any) (any, error) {
	url, err := argString(args, "url")
	if err != nil {
		return nil, jskernel.Errorf("browser.open needs {url: \"https://...\"}")
	}
	// A forced new tab is what makes ownership work: without it the desktop
	// may reuse a tab already on this URL, and reset would close a user tab.
	res, err := r.callAction(ctx, tools.BrowserActionRequest{Action: "navigate", URL: url, NewTab: true})
	if err != nil {
		return nil, err
	}
	tabID, title, liveURL := parseHeader(res.Result)
	if tabID == "" {
		return nil, jskernel.Errorf("browser.open could not tell which tab it opened. Open tabs: use browser.tabs() to inspect")
	}
	if err := r.manager.ownTab(r.session, tabID); err != nil {
		// Over the cap: close the tab just opened so a failed call leaves
		// nothing behind.
		_, _ = r.callAction(ctx, tools.BrowserActionRequest{Action: "close_tab", TabID: tabID})
		return nil, err
	}
	r.setActive(tabID)
	return map[string]any{"tabId": tabID, "url": liveURL, "title": title}, nil
}

func (r *JSRuntime) page(ctx context.Context, args map[string]any) (any, error) {
	tabID := argStringOr(args, "tabId", "")
	row, err := r.resolveTarget(ctx, tabID)
	if err != nil {
		return nil, err
	}
	r.setActive(row.ID)
	return map[string]any{
		"tabId": row.ID, "url": row.URL, "title": row.Title,
		"active": row.Active, "owned": r.manager.isOwned(r.session, row.ID),
	}, nil
}

func (r *JSRuntime) tabs(ctx context.Context, args map[string]any) (any, error) {
	rows, err := r.tabsJSON(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		out = append(out, map[string]any{
			"tabId": row.ID, "url": row.URL, "title": row.Title,
			"active": row.Active, "owned": r.manager.isOwned(r.session, row.ID),
		})
	}
	return out, nil
}

func (r *JSRuntime) gotoTab(ctx context.Context, args map[string]any) (any, error) {
	url, err := argString(args, "url")
	if err != nil {
		return nil, jskernel.Errorf("browser.goto needs {url: \"https://...\"} and optionally {tabId}")
	}
	tabID := argStringOr(args, "tabId", "")
	row, err := r.resolveTarget(ctx, tabID)
	if err != nil {
		return nil, err
	}
	res, err := r.callAction(ctx, tools.BrowserActionRequest{Action: "navigate", URL: url, TabID: row.ID})
	if err != nil {
		return nil, err
	}
	r.setActive(row.ID)
	_, title, liveURL := parseHeader(res.Result)
	return map[string]any{"tabId": row.ID, "url": liveURL, "title": title}, nil
}

func (r *JSRuntime) snapshot(ctx context.Context, args map[string]any) (any, error) {
	row, err := r.resolveTarget(ctx, argStringOr(args, "tabId", ""))
	if err != nil {
		return nil, err
	}
	res, err := r.callAction(ctx, tools.BrowserActionRequest{
		Action: "snapshot", TabID: row.ID, Selector: argStringOr(args, "selector", ""),
	})
	if err != nil {
		return nil, err
	}
	return res.Result, nil
}

func (r *JSRuntime) content(ctx context.Context, args map[string]any) (any, error) {
	row, err := r.resolveTarget(ctx, argStringOr(args, "tabId", ""))
	if err != nil {
		return nil, err
	}
	res, err := r.callAction(ctx, tools.BrowserActionRequest{
		Action: "get_content", TabID: row.ID, Selector: argStringOr(args, "selector", ""),
	})
	if err != nil {
		return nil, err
	}
	return res.Result, nil
}

func (r *JSRuntime) query(ctx context.Context, args map[string]any) (any, error) {
	row, err := r.resolveTarget(ctx, argStringOr(args, "tabId", ""))
	if err != nil {
		return nil, err
	}
	selector := argStringOr(args, "selector", "")
	if selector == "" {
		return nil, jskernel.Errorf("browser.query needs {selector}")
	}
	res, err := r.callAction(ctx, tools.BrowserActionRequest{Action: "query", TabID: row.ID, Selector: selector})
	if err != nil {
		return nil, err
	}
	// A match returns JSON ({selector, count, items, ...}); anything else is
	// the driver's no-match or invalid-selector message.
	if !strings.HasPrefix(strings.TrimSpace(res.Result), "{") {
		return nil, jskernel.Errorf("browser.query: %s", res.Result)
	}
	var decoded any
	if err := json.Unmarshal([]byte(res.Result), &decoded); err != nil {
		return nil, jskernel.Errorf("browser.query could not read its result: %v", err)
	}
	return decoded, nil
}

func (r *JSRuntime) click(ctx context.Context, args map[string]any) (any, error) {
	return r.act(ctx, "click", args)
}

func (r *JSRuntime) typeText(ctx context.Context, args map[string]any) (any, error) {
	if _, ok := args["text"]; !ok {
		return nil, jskernel.Errorf("browser.type needs {text} and a {ref} or {selector} target")
	}
	return r.act(ctx, "type", args)
}

// act is click and type: a ref or selector target, an optional submit for
// type, and the tab identity read back fresh from the desktop afterwards.
func (r *JSRuntime) act(ctx context.Context, action string, args map[string]any) (any, error) {
	row, err := r.resolveTarget(ctx, argStringOr(args, "tabId", ""))
	if err != nil {
		return nil, err
	}
	target := argStringOr(args, "ref", argStringOr(args, "selector", ""))
	if target == "" {
		return nil, jskernel.Errorf("browser.%s needs a {ref} from browser.snapshot() or a {selector}", action)
	}
	req := tools.BrowserActionRequest{
		Action: action, TabID: row.ID,
		Ref: argStringOr(args, "ref", ""), Selector: argStringOr(args, "selector", ""),
		Text: argStringOr(args, "text", ""), Submit: argBoolOr(args, "submit", false),
	}
	if _, err := r.callAction(ctx, req); err != nil {
		return nil, err
	}
	// The click may have navigated; read the live identity rather than
	// trusting the tab's pre-action URL.
	if fresh, err := r.tabsJSON(ctx); err == nil {
		for _, candidate := range fresh {
			if candidate.ID == row.ID {
				row = candidate
				break
			}
		}
	}
	return map[string]any{"tabId": row.ID, "url": row.URL, "title": row.Title}, nil
}

func (r *JSRuntime) waitFor(ctx context.Context, args map[string]any) (any, error) {
	row, err := r.resolveTarget(ctx, argStringOr(args, "tabId", ""))
	if err != nil {
		return nil, err
	}
	selector := argStringOr(args, "selector", "")
	text := argStringOr(args, "text", "")
	url := argStringOr(args, "url", "")
	if selector == "" && text == "" && url == "" {
		return nil, jskernel.Errorf("browser.waitFor needs at least one of {selector}, {text} or {url}")
	}
	timeoutMs := int(argIntOr(args, "timeoutMs", defaultWaitTimeoutMs))
	if timeoutMs <= 0 {
		timeoutMs = defaultWaitTimeoutMs
	}
	if timeoutMs > maxWaitTimeoutMs {
		timeoutMs = maxWaitTimeoutMs
	}
	if _, err := r.callAction(ctx, tools.BrowserActionRequest{
		Action: "wait_for", TabID: row.ID, Selector: selector, Text: text,
		URLContains: url, TimeoutMs: timeoutMs,
	}); err != nil {
		return nil, err
	}
	// The condition may coincide with a navigation; report live identity.
	if fresh, err := r.tabsJSON(ctx); err == nil {
		for _, candidate := range fresh {
			if candidate.ID == row.ID {
				row = candidate
				break
			}
		}
	}
	return map[string]any{"ok": true, "tabId": row.ID, "url": row.URL, "title": row.Title}, nil
}

// close closes one kernel-owned tab. A tab the kernel did not open is an
// error: closing user tabs is what the low-level browser tool is for.
func (r *JSRuntime) close(ctx context.Context, args map[string]any) (any, error) {
	tabID := argStringOr(args, "tabId", "")
	if tabID == "" {
		return nil, jskernel.Errorf("browser.close needs {tabId}; open tabs are listed by browser.tabs()")
	}
	if !r.manager.isOwned(r.session, tabID) {
		return nil, jskernel.Errorf("tab %s was not opened by this kernel; only tabs from browser.open() can be closed here", tabID)
	}
	if _, err := r.callAction(ctx, tools.BrowserActionRequest{Action: "close_tab", TabID: tabID}); err != nil {
		return nil, err
	}
	r.manager.forgetTab(r.session, tabID)
	if r.activeTabID() == tabID {
		r.setActive("")
	}
	return map[string]any{"closed": 1, "tabId": tabID}, nil
}

// closeAll closes every kernel-owned tab and keeps the runtime: variables
// survive, page references do not.
func (r *JSRuntime) closeAll(ctx context.Context, args map[string]any) (any, error) {
	owned := r.manager.ownedTabs(r.session)
	var warnings []string
	closed := 0
	for _, tabID := range owned {
		if _, err := r.callAction(ctx, tools.BrowserActionRequest{Action: "close_tab", TabID: tabID}); err != nil {
			warnings = append(warnings, err.Error())
			continue
		}
		r.manager.forgetTab(r.session, tabID)
		closed++
	}
	if r.activeTabID() != "" && !r.manager.isOwned(r.session, r.activeTabID()) {
		r.setActive("")
	}
	out := map[string]any{"closed": closed}
	if len(warnings) > 0 {
		out["warnings"] = warnings
	}
	return out, nil
}

// --- argument helpers -------------------------------------------------------

func argString(args map[string]any, key string) (string, error) {
	value, ok := args[key]
	if !ok {
		return "", fmt.Errorf("missing %q", key)
	}
	text, ok := value.(string)
	if !ok || strings.TrimSpace(text) == "" {
		return "", fmt.Errorf("%q must be a non-empty string", key)
	}
	return text, nil
}

func argStringOr(args map[string]any, key, fallback string) string {
	if value, ok := args[key]; ok {
		if text, ok := value.(string); ok {
			return text
		}
	}
	return fallback
}

func argBoolOr(args map[string]any, key string, fallback bool) bool {
	if value, ok := args[key]; ok {
		if b, ok := value.(bool); ok {
			return b
		}
	}
	return fallback
}

func argIntOr(args map[string]any, key string, fallback int64) int64 {
	if value, ok := args[key]; ok {
		switch n := value.(type) {
		case int64:
			return n
		case float64:
			return int64(n)
		case int:
			return int64(n)
		}
	}
	return fallback
}

// parseHeader reads the desktop's "Tab: <id>\nTitle: <t>\nURL: <u>" action
// header. Parsing our own wire text is the one place the kernel leans on the
// desktop's result format; the navigate result has no structured alternative.
func parseHeader(text string) (tabID, title, url string) {
	inTitle := false
	var titleParts []string
	for _, line := range strings.Split(text, "\n") {
		switch {
		case strings.HasPrefix(line, "Tab: "):
			tabID = strings.TrimSpace(strings.TrimPrefix(line, "Tab: "))
		case strings.HasPrefix(line, "Title: "):
			titleParts = append(titleParts, strings.TrimPrefix(line, "Title: "))
			inTitle = true
		case strings.HasPrefix(line, "URL: "):
			url = strings.TrimSpace(strings.TrimPrefix(line, "URL: "))
			inTitle = false
		case inTitle:
			titleParts = append(titleParts, line)
		}
	}
	return tabID, strings.Join(titleParts, "\n"), url
}
