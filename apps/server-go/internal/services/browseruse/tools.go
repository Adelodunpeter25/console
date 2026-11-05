// The two harness tools browser use exposes: `browser` executes code
// against the session's persistent `browser` object, `browser_reset` cleans
// up. Like the console kernel, the small schemas stay flat no matter how
// large the API grows; the methods live in the /browser-use skill.
package browseruse

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/tools"
)

type browserInput struct {
	Code      string `json:"code" jsonschema:"description=JavaScript to run with the preinstalled browser object, e.g. browser.open({url: 'https://example.com'}). Variables persist between calls; use print() for narration. The /browser-use instructions in this conversation document the methods."`
	TimeoutMs int    `json:"timeoutMs,omitempty" jsonschema:"description=Execution budget in milliseconds. Defaults to 300000 (5 minutes); a stopped script is reported as unknown completion because an action in flight may have taken effect."`
}

// browserTool runs code in the session's kernel. It embeds the declarative
// tool for its name, description and schema, and overrides Execute to bind
// the call to one session — the same pattern as the console tool.
type browserTool struct {
	tools.Tool
	manager   *Manager
	sessionID string
	handler   tools.BrowserHandler
}

// NewBrowserTool builds the `browser` tool bound to one chat session, so its
// variables and owned tabs never leak into another session's kernel.
func NewBrowserTool(manager *Manager, sessionID string, handler tools.BrowserHandler) tools.Tool {
	inner := tools.NewTool("browser",
		"Drive the built-in desktop browser with the preinstalled browser object (browser.open, browser.click, browser.query, ...). One call can navigate, wait, extract and loop; variables persist until reset. The /browser-use instructions in this conversation document the methods; when they are not loaded yet, read the browser-use skill to load them. When a browsing workflow is done, close the tabs it opened with the browser_reset tool.",
		// Read-tier, like the low-level tool it replaced: browsing is
		// observation, and plan mode may read pages. A click on a page is
		// gated by the skill's risk rules, not the approval table.
		tools.TierRead,
		func(ctx context.Context, in browserInput) (any, error) {
			return nil, tools.NewToolError("browser requires call context")
		})
	return &browserTool{Tool: inner, manager: manager, sessionID: sessionID, handler: handler}
}

// Execute dispatches to the session's runtime. The handler arrives per run,
// so a restored kernel binds to the live desktop connection of the run that
// uses it.
func (t *browserTool) Execute(ctx context.Context, arguments json.RawMessage) (any, error) {
	var in browserInput
	if err := json.Unmarshal(arguments, &in); err != nil {
		return nil, tools.NewToolError("Invalid arguments for browser: %v", err)
	}
	runtime, err := t.manager.runtime(t.sessionID, t.handler)
	if err != nil {
		return nil, tools.NewToolError("browser use is unavailable: %v", err)
	}
	return runtime.Eval(ctx, in.Code, time.Duration(in.TimeoutMs)*time.Millisecond)
}

type browserResetInput struct {
	TabID string `json:"tabId,omitempty" jsonschema:"description=Targeted cleanup: close this one kernel-owned tab and leave everything else. Without it, close every kernel-owned tab and forget all variables."`
}

// browserResetTool cleans the session kernel up. It embeds the declarative
// tool the same way browserRunTool does.
type browserResetTool struct {
	tools.Tool
	manager   *Manager
	sessionID string
	handler   tools.BrowserHandler
}

// NewResetTool builds the `browser_reset` tool. A full reset drops the
// runtime and closes every kernel-owned tab; a targeted reset ({tabId})
// closes one owned tab and leaves the runtime and its variables intact.
// Closing tabs changes what the user sees, but only among tabs the kernel
// itself opened, which is why the tool is read-tier like computer_reset.
func NewResetTool(manager *Manager, sessionID string, handler tools.BrowserHandler) tools.Tool {
	inner := tools.NewTool("browser_reset",
		"Clean up browser use: close the tabs the kernel opened (all of them, or one with tabId). A full reset also forgets every browser variable. Tabs you or the user opened another way are never closed.",
		tools.TierRead,
		func(ctx context.Context, in browserResetInput) (any, error) {
			return nil, tools.NewToolError("browser_reset requires call context")
		})
	return &browserResetTool{Tool: inner, manager: manager, sessionID: sessionID, handler: handler}
}

func (t *browserResetTool) Execute(ctx context.Context, arguments json.RawMessage) (any, error) {
	var in browserResetInput
	if err := json.Unmarshal(arguments, &in); err != nil {
		return nil, tools.NewToolError("Invalid arguments for browser_reset: %v", err)
	}
	if in.TabID != "" {
		return t.closeOne(ctx, in.TabID)
	}
	return t.closeAllAndDrop(ctx)
}

// closeOne is the targeted reset: one owned tab closes, variables stay.
func (t *browserResetTool) closeOne(ctx context.Context, tabID string) (any, error) {
	if !t.manager.isOwned(t.sessionID, tabID) {
		return nil, tools.NewToolError("tab %s was not opened by this kernel; browser_reset only closes tabs from browser's browser.open()", tabID)
	}
	if _, err := t.handler(ctx, tools.BrowserActionRequest{Action: "close_tab", TabID: tabID}); err != nil {
		return nil, tools.NewToolError("could not close tab %s: %v", tabID, err)
	}
	t.manager.forgetTab(t.sessionID, tabID)
	return map[string]any{"closed": []string{tabID}, "cleared": false}, nil
}

// closeAllAndDrop is the full reset: every owned tab closes (best-effort), the
// runtime drops, and the next browser starts fresh.
func (t *browserResetTool) closeAllAndDrop(ctx context.Context) (any, error) {
	owned := t.manager.ownedTabs(t.sessionID)
	var closed, warnings []string
	for _, tabID := range owned {
		if _, err := t.handler(ctx, tools.BrowserActionRequest{Action: "close_tab", TabID: tabID}); err != nil {
			warnings = append(warnings, err.Error())
			continue
		}
		t.manager.forgetTab(t.sessionID, tabID)
		closed = append(closed, tabID)
	}
	t.manager.dropRuntime(t.sessionID)
	out := map[string]any{"closed": closed, "cleared": true}
	if len(warnings) > 0 {
		out["warnings"] = warnings
	}
	return out, nil
}

// Loader binds the browser tools to one chat session, mirroring the CUA
// loader: the tools load only when /browser-use activates the capability.
type Loader struct {
	manager   *Manager
	sessionID string
	handler   tools.BrowserHandler

	mu     sync.Mutex
	loaded bool
}

// NewLoader returns a loader bound to manager, one session, and that run's
// desktop bridge.
func NewLoader(manager *Manager, sessionID string, handler tools.BrowserHandler) *Loader {
	return &Loader{manager: manager, sessionID: sessionID, handler: handler}
}

// Loaded reports whether the tools have been brought into some run.
func (l *Loader) Loaded() bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.loaded
}

// Load appends the two browser tools to the registry and marks the group
// loaded. Unlike computer use there is no library to open — the tools are
// always loadable; a missing desktop client surfaces per call as an action
// timeout instead of here.
func (l *Loader) Load(registry *tools.Registry) (int, error) {
	registry.Add(
		NewBrowserTool(l.manager, l.sessionID, l.handler),
		NewResetTool(l.manager, l.sessionID, l.handler),
	)
	l.mu.Lock()
	l.loaded = true
	l.mu.Unlock()
	return 2, nil
}
