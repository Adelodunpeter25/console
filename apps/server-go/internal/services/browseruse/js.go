// The browser-use JavaScript runtime: one persistent interpreter per chat
// session with a preinstalled `browser` object.
//
// The shape is the console kernel's: plain results (a call returns its data
// directly), one object argument per method, unknown members named in the
// error, and the API documented once in the /browser-use skill instead of as
// schema on every turn. Methods block on the desktop client answering the
// underlying low-level action.
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
	"regexp"
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
	// maxHandles caps the live locator handles browser.find may keep. A
	// handle nobody uses again is dropped only by reset, so the cap keeps
	// the map bounded.
	maxHandles = 32
	// Pagination caps: how many pages one browser.paginate call may fetch,
	// how many items and bytes it may accumulate.
	maxPaginatePages = 20
	maxPaginateItems = 500
	// maxPaginateBytes bounds the accumulated item payload.
	maxPaginateBytes = 200000
	// Default and hard-cap page budget for browser.paginate.
	defaultPaginatePages = 5
	// Per-page extract budget inside paginate, and how long the driver may
	// wait for a clicked "next" to change the page.
	paginatePerPageLimit = 200
	paginateWaitMs       = 8000
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
	// handles are re-resolvable locator handles from browser.find. A handle
	// stores the locator plus the tab identity it was found on; using it
	// re-runs the find, so it survives re-renders but not navigation.
	handles    map[string]handleRec
	nextHandle int
}

// handleRec is one browser.find result: the encoded locator the driver
// resolves, the tab it was found on, and that tab's URL at find time — the
// generation check. A navigation changes the URL, which makes every handle
// stale on purpose: silent remapping can act on the wrong element.
type handleRec struct {
	selector string
	tabID    string
	url      string
	target   string
}

// NewJSRuntime builds a runtime with the `browser` object installed.
func NewJSRuntime(manager *Manager, sessionID string, handler tools.BrowserHandler) (*JSRuntime, error) {
	r := &JSRuntime{manager: manager, session: sessionID, handler: handler}
	kernel, err := jskernel.New(jskernel.Options{
		Root:             "browser",
		PlainResults:     true,
		InterruptMessage: "execution stopped; a browser action in flight may already have taken effect. Inspect fresh state before retrying.",
	}, r.methods())
	if err != nil {
		return nil, err
	}
	r.kernel = kernel
	return r, nil
}

// methods is the `browser` API. One object argument each, plain results.
func (r *JSRuntime) methods() []jskernel.Method {
	methods := []jskernel.Method{
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
		{Name: "extract", Call: r.method(r.extract)},
		{Name: "scroll", Call: r.method(r.scroll)},
		{Name: "paginate", Call: r.method(r.paginate)},
		{Name: "find", Call: r.method(r.find)},
		{Name: "close", Call: r.method(r.close)},
		{Name: "closeAll", Call: r.method(r.closeAll)},
	}
	// Every method refuses keys it does not take, so a typo such as
	// {projectId: ...} fails with the accepted names instead of being ignored.
	for i := range methods {
		allowed, known := methodArgs[methods[i].Name]
		if !known {
			continue
		}
		call := methods[i].Call
		methods[i].Call = func(ctx context.Context, args map[string]any) (*jskernel.Result, error) {
			if err := jskernel.OnlyArgs(args, allowed...); err != nil {
				return nil, err
			}
			return call(ctx, args)
		}
	}
	return methods
}

// methodArgs lists the arguments each browser method accepts. Every method
// must be listed; kernel_test pins the coverage.
var methodArgs = map[string][]string{
	"open":     {"url"},
	"page":     {"tabId"},
	"tabs":     {},
	"goto":     {"url", "tabId"},
	"snapshot": {"selector", "tabId"},
	"content":  {"selector", "tabId"},
	"query":    {"selector", "tabId"},
	"click":    {"ref", "selector", "text", "role", "name", "handle", "tabId"},
	"type":     {"text", "ref", "selector", "role", "name", "handle", "submit", "tabId"},
	"waitFor":  {"selector", "text", "url", "timeoutMs", "tabId"},
	"extract":  {"scope", "items", "fields", "limit", "waitForNew", "waitMs", "tabId"},
	"scroll":   {"direction", "amount", "tabId"},
	"paginate": {"itemSelector", "next", "maxPages", "extract", "scope", "tabId"},
	"find":     {"text", "selector", "role", "name", "tabId"},
	"close":    {"tabId"},
	"closeAll": {},
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
		return nil, jskernel.Errorf("browser.type needs {text} and a target: {ref}, {selector}, {role, name} or {handle}")
	}
	return r.act(ctx, "type", args)
}

// actionTarget resolves what a click/type aims at and which tab it runs on.
// Exactly one target form may be given: a snapshot {ref}, a CSS {selector},
// a {text} locator (click only — type's text is the content), a
// {role, name} locator, or a {handle} from browser.find. Text and role
// locators ride the selector wire field with a text=/role= prefix the driver
// parses.
func (r *JSRuntime) actionTarget(ctx context.Context, action string, args map[string]any) (selector, ref, tabID string, err error) {
	given := make([]string, 0, 5)
	for _, key := range []string{"ref", "selector", "handle", "role"} {
		if _, ok := args[key]; ok {
			given = append(given, key)
		}
	}
	if action == "click" {
		if _, ok := args["text"]; ok {
			given = append(given, "text")
		}
	}
	if len(given) > 1 {
		return "", "", "", jskernel.Errorf("browser.%s takes one target; got %s. Use {ref} from browser.snapshot(), {selector}, {text}, {role, name} or {handle} from browser.find()", action, strings.Join(given, ", "))
	}
	if len(given) == 0 {
		return "", "", "", jskernel.Errorf("browser.%s needs a target: {ref} from browser.snapshot(), {selector}, {text}, {role, name} or {handle} from browser.find()", action)
	}

	switch given[0] {
	case "ref":
		ref = argStringOr(args, "ref", "")
		if strings.TrimSpace(ref) == "" {
			return "", "", "", jskernel.Errorf("browser.%s {ref} must be a non-empty string", action)
		}
	case "selector":
		selector = argStringOr(args, "selector", "")
		if strings.TrimSpace(selector) == "" {
			return "", "", "", jskernel.Errorf("browser.%s {selector} must be a non-empty string", action)
		}
	case "text":
		text := argStringOr(args, "text", "")
		if strings.TrimSpace(text) == "" {
			return "", "", "", jskernel.Errorf("browser.click {text} must be a non-empty string")
		}
		selector = encodeTextLocator(text)
	case "role":
		role := argStringOr(args, "role", "")
		selector, err = encodeRoleLocator(role, argStringOr(args, "name", ""))
		if err != nil {
			return "", "", "", err
		}
	case "handle":
		selector, tabID, err = r.resolveHandle(ctx, argStringOr(args, "handle", ""))
		if err != nil {
			return "", "", "", err
		}
	}
	if tabID == "" {
		tabID = argStringOr(args, "tabId", "")
	}
	return selector, ref, tabID, nil
}

// encodeTextLocator encodes a find-by-text target for the driver.
func encodeTextLocator(text string) string {
	return "text=" + text
}

// roleRe constrains the ARIA role name in a role= locator; anything else
// would be a typo the driver could never match.
var roleRe = regexp.MustCompile(`^[A-Za-z]+$`)

// encodeRoleLocator encodes a find-by-role target for the driver. The name is
// JSON-quoted so quotes, brackets and newlines survive the round trip.
func encodeRoleLocator(role, name string) (string, error) {
	if !roleRe.MatchString(strings.TrimSpace(role)) {
		return "", jskernel.Errorf("{role} must be an ARIA role name like \"button\" or \"link\", got %q", role)
	}
	enc := "role=" + strings.TrimSpace(role)
	if name != "" {
		quoted, marshalErr := json.Marshal(name)
		if marshalErr != nil {
			return "", jskernel.Errorf("{name} could not be encoded: %v", marshalErr)
		}
		enc += "[name=" + string(quoted) + "]"
	}
	return enc, nil
}

// resolveHandle validates a handle id and returns its stored locator and tab.
// The handle is only usable while its tab still shows the URL it was found
// on; a navigation makes it stale so a changed page can never be acted on
// through an old reference.
func (r *JSRuntime) resolveHandle(ctx context.Context, handle string) (selector, tabID string, err error) {
	r.mu.Lock()
	rec, ok := r.handles[handle]
	r.mu.Unlock()
	if !ok {
		return "", "", jskernel.Errorf("unknown handle %q; handles come from browser.find() and are dropped on reset", handle)
	}
	row, rowErr := r.resolveTarget(ctx, rec.tabID)
	if rowErr != nil {
		return "", "", jskernel.Errorf("handle %s is stale: its tab is gone (%v)", handle, rowErr)
	}
	if row.URL != rec.url {
		return "", "", jskernel.Errorf("handle %s is stale: the page navigated from %s to %s. Call browser.find() again", handle, rec.url, row.URL)
	}
	return rec.selector, rec.tabID, nil
}

// act is click and type: a locator target, an optional submit for type, and
// the tab identity read back fresh from the desktop afterwards.
func (r *JSRuntime) act(ctx context.Context, action string, args map[string]any) (any, error) {
	selector, ref, tabID, err := r.actionTarget(ctx, action, args)
	if err != nil {
		return nil, err
	}
	row, err := r.resolveTarget(ctx, tabID)
	if err != nil {
		return nil, err
	}
	req := tools.BrowserActionRequest{
		Action: action, TabID: row.ID, Ref: ref, Selector: selector,
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

// extract runs one scoped structured extraction on the tab: items matching
// `items` inside `scope`, each reduced to `fields`. One host call per page —
// the driver does the element work in-page, so extracting 30 offers does not
// cost 30 round-trips.
func (r *JSRuntime) extract(ctx context.Context, args map[string]any) (any, error) {
	row, err := r.resolveTarget(ctx, argStringOr(args, "tabId", ""))
	if err != nil {
		return nil, err
	}
	spec, err := extractSpec(args)
	if err != nil {
		return nil, err
	}
	return r.extractOn(ctx, row.ID, spec)
}

// extractSpec validates the extract arguments into the JSON spec the driver
// runs. Shape errors name the offending key; the driver only sees valid specs.
func extractSpec(args map[string]any) ([]byte, error) {
	spec := map[string]any{}
	if scope := argStringOr(args, "scope", ""); scope != "" {
		spec["scope"] = scope
	}
	if items := argStringOr(args, "items", ""); items != "" {
		spec["items"] = items
	}
	if raw, ok := args["fields"]; ok {
		fields, err := validateFields(raw)
		if err != nil {
			return nil, err
		}
		spec["fields"] = fields
	}
	if limit := argIntOr(args, "limit", 0); limit > 0 {
		if limit > maxExtractItems {
			limit = maxExtractItems
		}
		spec["limit"] = limit
	}
	if argBoolOr(args, "waitForNew", false) {
		spec["waitForNew"] = true
		if waitMs := argIntOr(args, "waitMs", 0); waitMs > 0 {
			if waitMs > maxExtractWaitMs {
				waitMs = maxExtractWaitMs
			}
			spec["waitMs"] = waitMs
		}
	}
	raw, err := json.Marshal(spec)
	if err != nil {
		return nil, jskernel.Errorf("browser.extract spec could not be encoded: %v", err)
	}
	return raw, nil
}

// extract field-spec limits: how many fields one extract may define, and how
// long the driver may wait for a page's items to change (waitForNew).
const (
	maxExtractItems  = 200
	maxExtractFields = 16
	maxExtractWaitMs = 15000
)

// fieldSpecKeys are the keys a field spec object may take.
var fieldSpecKeys = []string{"selector", "attribute", "all", "optional", "trim"}

// validateFields checks the `fields` map: every value is a CSS selector
// string or an object with `selector` (required) plus `attribute`, `all`,
// `optional`, `trim`. Unknown keys inside a field spec are refused, like
// method arguments are.
func validateFields(raw any) (map[string]any, error) {
	fields, ok := raw.(map[string]any)
	if !ok {
		return nil, jskernel.Errorf("browser.extract {fields} must be an object of {name: selector or {selector, attribute, all, optional, trim}}")
	}
	if len(fields) > maxExtractFields {
		return nil, jskernel.Errorf("browser.extract takes at most %d fields, got %d", maxExtractFields, len(fields))
	}
	for name, value := range fields {
		if text, isString := value.(string); isString {
			if strings.TrimSpace(text) == "" {
				return nil, jskernel.Errorf("field %q has an empty selector", name)
			}
			continue
		}
		spec, isMap := value.(map[string]any)
		if !isMap {
			return nil, jskernel.Errorf("field %q must be a selector string or an object", name)
		}
		for key := range spec {
			known := false
			for _, allowed := range fieldSpecKeys {
				if key == allowed {
					known = true
					break
				}
			}
			if !known {
				return nil, jskernel.Errorf("field %q takes: %s", name, strings.Join(fieldSpecKeys, ", "))
			}
		}
		selector, hasSelector := spec["selector"].(string)
		if !hasSelector || strings.TrimSpace(selector) == "" {
			return nil, jskernel.Errorf("field %q needs a {selector}", name)
		}
	}
	return fields, nil
}

// extractOn sends one extract action and decodes the driver's JSON result.
// A non-JSON reply is the driver's plain-text failure (no scope match, an
// invalid selector) and throws with that message.
func (r *JSRuntime) extractOn(ctx context.Context, tabID string, spec []byte) (map[string]any, error) {
	res, err := r.callAction(ctx, tools.BrowserActionRequest{Action: "extract", TabID: tabID, Script: string(spec)})
	if err != nil {
		return nil, err
	}
	if !strings.HasPrefix(strings.TrimSpace(res.Result), "{") {
		return nil, jskernel.Errorf("browser.extract: %s", res.Result)
	}
	var decoded map[string]any
	if err := json.Unmarshal([]byte(res.Result), &decoded); err != nil {
		return nil, jskernel.Errorf("browser.extract could not read its result: %v", err)
	}
	return decoded, nil
}

// scroll scrolls the page. Direction is up, down, top or bottom; amount is
// the pixel distance for up/down (default: most of a viewport).
func (r *JSRuntime) scroll(ctx context.Context, args map[string]any) (any, error) {
	row, err := r.resolveTarget(ctx, argStringOr(args, "tabId", ""))
	if err != nil {
		return nil, err
	}
	direction := argStringOr(args, "direction", "down")
	switch direction {
	case "up", "down", "top", "bottom":
	default:
		return nil, jskernel.Errorf("browser.scroll {direction} must be up, down, top or bottom, got %q", direction)
	}
	spec := map[string]any{"direction": direction}
	if amount := argIntOr(args, "amount", 0); amount > 0 {
		if amount > 20000 {
			amount = 20000
		}
		spec["amount"] = amount
	}
	raw, err := json.Marshal(spec)
	if err != nil {
		return nil, jskernel.Errorf("browser.scroll spec could not be encoded: %v", err)
	}
	res, err := r.callAction(ctx, tools.BrowserActionRequest{Action: "scroll", TabID: row.ID, Script: string(raw)})
	if err != nil {
		return nil, err
	}
	if !strings.HasPrefix(strings.TrimSpace(res.Result), "{") {
		return nil, jskernel.Errorf("browser.scroll: %s", res.Result)
	}
	var decoded map[string]any
	if err := json.Unmarshal([]byte(res.Result), &decoded); err != nil {
		return nil, jskernel.Errorf("browser.scroll could not read its result: %v", err)
	}
	decoded["tabId"] = row.ID
	return decoded, nil
}

// paginate walks a paginated list: extract each page, click the next
// control, repeat. Stops on the page budget, a missing/disabled next
// control, a page that never changes after the click, or the item/byte caps.
// Items are deduplicated by href when a field of that name exists, else by
// their full content.
func (r *JSRuntime) paginate(ctx context.Context, args map[string]any) (any, error) {
	itemSelector := argStringOr(args, "itemSelector", "")
	if strings.TrimSpace(itemSelector) == "" {
		return nil, jskernel.Errorf("browser.paginate needs {itemSelector, next, extract}; next is the next-page control ({text}, {selector} or {role, name}) and extract the per-item fields")
	}
	next, ok := args["next"].(map[string]any)
	if !ok {
		return nil, jskernel.Errorf("browser.paginate needs {next}: the next-page control as {text}, {selector} or {role, name}")
	}
	rawFields, hasFields := args["extract"]
	if !hasFields {
		return nil, jskernel.Errorf("browser.paginate needs {extract}: the per-item fields, same shape as browser.extract's {fields}")
	}
	fields, err := validateFields(rawFields)
	if err != nil {
		return nil, err
	}
	for key := range next {
		switch key {
		case "text", "selector", "role", "name":
		default:
			return nil, jskernel.Errorf("browser.paginate {next} takes: text, selector, role, name")
		}
	}
	maxPages := int(argIntOr(args, "maxPages", defaultPaginatePages))
	if maxPages < 1 {
		maxPages = 1
	}
	if maxPages > maxPaginatePages {
		maxPages = maxPaginatePages
	}
	scope := argStringOr(args, "scope", "")

	row, err := r.resolveTarget(ctx, argStringOr(args, "tabId", ""))
	if err != nil {
		return nil, err
	}

	var (
		all       []any
		seen      = map[string]bool{}
		warnings  []string
		pages     int
		truncated bool
		totalSize int
	)
	for page := 1; page <= maxPages && !truncated; page++ {
		specArgs := map[string]any{"fields": fields, "limit": int64(paginatePerPageLimit)}
		if scope != "" {
			specArgs["scope"] = scope
		}
		if page > 1 {
			// After the click: give the new page up to the wait budget to
			// appear instead of polling with host calls.
			specArgs["waitForNew"] = true
			specArgs["waitMs"] = int64(paginateWaitMs)
		}
		spec, specErr := extractSpec(specArgs)
		if specErr != nil {
			return nil, specErr
		}
		result, extractErr := r.extractOn(ctx, row.ID, spec)
		if extractErr != nil {
			if page == 1 {
				return nil, extractErr
			}
			warnings = append(warnings, fmt.Sprintf("stopped on page %d: %v", page, extractErr))
			break
		}
		if unchanged, _ := result["unchanged"].(bool); unchanged {
			warnings = append(warnings, fmt.Sprintf("page %d did not change after clicking next; stopped", page))
			break
		}
		items, _ := result["items"].([]any)
		fresh := 0
		for _, item := range items {
			key := dedupeKey(item)
			if seen[key] {
				continue
			}
			seen[key] = true
			itemJSON, marshalErr := json.Marshal(item)
			if marshalErr != nil {
				continue
			}
			if len(all) >= maxPaginateItems || totalSize+len(itemJSON) > maxPaginateBytes {
				truncated = true
				break
			}
			all = append(all, item)
			totalSize += len(itemJSON)
			fresh++
		}
		pages++
		if truncated {
			warnings = append(warnings, fmt.Sprintf("stopped at %d items or %d bytes", maxPaginateItems, maxPaginateBytes))
			break
		}
		if page == maxPages {
			break
		}
		if fresh == 0 {
			// This page repeated the last one exactly: further clicks would
			// only loop the same content.
			warnings = append(warnings, fmt.Sprintf("page %d repeated the previous page; stopped", page))
			break
		}
		clickArgs := map[string]any{"tabId": row.ID}
		for _, key := range []string{"text", "selector", "role", "name"} {
			if value, ok := next[key]; ok {
				clickArgs[key] = value
			}
		}
		if _, clickErr := r.act(ctx, "click", clickArgs); clickErr != nil {
			warnings = append(warnings, fmt.Sprintf("no next control after page %d: %v", page, clickErr))
			break
		}
	}
	out := map[string]any{"items": all, "pages": pages, "truncated": truncated}
	if len(warnings) > 0 {
		out["warnings"] = warnings
	}
	return out, nil
}

// dedupeKey prefers an href field (the same product on two pages is one
// item), falling back to the item's full content.
func dedupeKey(item any) string {
	if obj, ok := item.(map[string]any); ok {
		if href, ok := obj["href"].(string); ok && href != "" {
			return "href:" + href
		}
	}
	raw, err := json.Marshal(item)
	if err != nil {
		return fmt.Sprintf("%v", item)
	}
	return string(raw)
}

// find resolves a locator now and returns a handle that can be re-resolved
// on later calls ({handle: "h1"} in click/type). The handle survives
// re-renders (the driver re-runs the find) but not navigation: the tab must
// still show the URL it was found on.
func (r *JSRuntime) find(ctx context.Context, args map[string]any) (any, error) {
	selector, err := findLocator(args)
	if err != nil {
		return nil, err
	}
	row, err := r.resolveTarget(ctx, argStringOr(args, "tabId", ""))
	if err != nil {
		return nil, err
	}
	target := describeLocator(args, selector)

	r.mu.Lock()
	if len(r.handles) >= maxHandles {
		r.mu.Unlock()
		return nil, jskernel.Errorf("too many live handles (%d); they are dropped on reset", maxHandles)
	}
	if r.handles == nil {
		r.handles = map[string]handleRec{}
	}
	r.nextHandle++
	id := fmt.Sprintf("h%d", r.nextHandle)
	r.handles[id] = handleRec{selector: selector, tabID: row.ID, url: row.URL, target: target}
	r.mu.Unlock()
	return map[string]any{"handle": id, "tabId": row.ID, "url": row.URL, "target": target}, nil
}

// findLocator encodes a browser.find target: one of {text}, {selector} or
// {role, name}. Refs are deliberately not findable — they already identify
// one element, but die at navigation, which is what find exists to fix.
func findLocator(args map[string]any) (string, error) {
	text, hasText := args["text"]
	selector, hasSelector := args["selector"]
	role, hasRole := args["role"]
	given := 0
	for _, ok := range []bool{hasText, hasSelector, hasRole} {
		if ok {
			given++
		}
	}
	if given > 1 {
		return "", jskernel.Errorf("browser.find takes one target: {text}, {selector} or {role, name}")
	}
	if hasText {
		value, isString := text.(string)
		if !isString || strings.TrimSpace(value) == "" {
			return "", jskernel.Errorf("browser.find {text} must be a non-empty string")
		}
		return encodeTextLocator(value), nil
	}
	if hasSelector {
		value, isString := selector.(string)
		if !isString || strings.TrimSpace(value) == "" {
			return "", jskernel.Errorf("browser.find {selector} must be a non-empty string")
		}
		return value, nil
	}
	if hasRole {
		value, isString := role.(string)
		if !isString {
			return "", jskernel.Errorf("browser.find {role} must be an ARIA role name")
		}
		return encodeRoleLocator(value, argStringOr(args, "name", ""))
	}
	return "", jskernel.Errorf("browser.find needs a target: {text}, {selector} or {role, name}")
}

// describeLocator renders what a handle points at, for the find result.
func describeLocator(args map[string]any, selector string) string {
	if text, ok := args["text"].(string); ok && text != "" {
		return fmt.Sprintf("text %q", text)
	}
	if role, ok := args["role"].(string); ok && role != "" {
		name, _ := args["name"].(string)
		return fmt.Sprintf("%s %q", role, name)
	}
	return "selector " + selector
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
	r.mu.Lock()
	for id, rec := range r.handles {
		if rec.tabID == tabID {
			delete(r.handles, id)
		}
	}
	r.mu.Unlock()
	if r.activeTabID() == tabID {
		r.setActive("")
	}
	return map[string]any{"closed": 1, "tabId": tabID}, nil
}

// closeAll closes every kernel-owned tab and keeps the runtime: variables
// survive, page references (handles included) do not.
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
	r.mu.Lock()
	r.handles = nil
	r.mu.Unlock()
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
