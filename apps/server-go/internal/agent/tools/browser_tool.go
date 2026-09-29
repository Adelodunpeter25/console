// browser tool: native desktop webview control (navigate, run_js, get_content, tabs).
package tools

import (
	"context"
	"strings"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/utils"
)

// BrowserActionRequest is dispatched to the connected desktop client.
type BrowserActionRequest struct {
	RequestID string `json:"requestId"`
	Action    string `json:"action"`
	URL       string `json:"url,omitempty"`
	Script    string `json:"script,omitempty"`
	Selector  string `json:"selector,omitempty"`
	TabID     string `json:"tabId,omitempty"`
	// wait_for conditions (all provided conditions must hold).
	URLContains string `json:"urlContains,omitempty"`
	Text        string `json:"text,omitempty"`
	TimeoutMs   int    `json:"timeoutMs,omitempty"`
	// Element refs from 'snapshot', used by click/type.
	Ref    string `json:"ref,omitempty"`
	Submit bool   `json:"submit,omitempty"`
}

// BrowserActionResult is the result received from the desktop client.
type BrowserActionResult struct {
	Result string `json:"result,omitempty"`
	Error  string `json:"error,omitempty"`
}

// BrowserHandler handles one browser action by delegating to the client.
type BrowserHandler func(ctx context.Context, req BrowserActionRequest) (BrowserActionResult, error)

type browserInput struct {
	Action      string `json:"action" jsonschema:"required,enum=navigate,enum=run_js,enum=get_content,enum=tabs,enum=snapshot,enum=click,enum=type,enum=wait_for,enum=screenshot,description=Browser action: 'navigate' to a URL (returns the tab id), 'snapshot' to list the page's interactive elements with refs like e12, 'click' or 'type' to act on a ref from the latest snapshot, 'wait_for' until a selector, URL fragment, or text appears (works for client-side navigation), 'run_js' to evaluate JavaScript and get its return value, 'get_content' to read page text (or the text of a selector), 'tabs' to list open tabs, or 'screenshot' (not supported yet)."`
	URL         string `json:"url,omitempty" jsonschema:"description=URL to navigate to (required for 'navigate')."`
	Script      string `json:"script,omitempty" jsonschema:"description=JavaScript expression or function to evaluate (required for 'run_js'). The value it returns (promises are awaited) is sent back as the result."`
	Selector    string `json:"selector,omitempty" jsonschema:"description=Optional CSS selector for 'get_content'. Returns the text of matching elements, or a no-match message."`
	TabID       string `json:"tabId,omitempty" jsonschema:"description=Tab id from 'navigate' or 'tabs'. Omit to use the tab you last navigated or worked in (for 'navigate': a tab already on that URL, else a new tab). Pass it explicitly when several tabs are open."`
	URLContains string `json:"urlContains,omitempty" jsonschema:"description=For 'wait_for': substring the tab URL must contain."`
	Text        string `json:"text,omitempty" jsonschema:"description=For 'wait_for': text the page must contain. For 'type': the text to enter."`
	Ref         string `json:"ref,omitempty" jsonschema:"description=Element ref from the latest 'snapshot' (for 'click' and 'type'). Refs expire when the page changes; a stale ref returns a fresh snapshot."`
	Submit      bool   `json:"submit,omitempty" jsonschema:"description=For 'type': press Enter after typing (e.g. to submit a search)."`
	TimeoutMs   int    `json:"timeoutMs,omitempty" jsonschema:"description=For 'wait_for': max wait in milliseconds (default 10000, max 60000)."`
}

// NewBrowserTool builds the "browser" tool bound to handler.
func NewBrowserTool(handler BrowserHandler) Tool {
	return NewTool(
		"browser",
		"Control the built-in desktop browser view to inspect dev servers, navigate pages, execute JavaScript and read its result, or extract rendered page text. Tabs are addressed by id (see the 'tabs' action).",
		TierRead,
		func(ctx context.Context, in browserInput) (any, error) {
			action := strings.TrimSpace(in.Action)
			switch action {
			case "navigate":
				if strings.TrimSpace(in.URL) == "" {
					return nil, NewToolError("URL is required for 'navigate' action.")
				}
			case "run_js":
				if strings.TrimSpace(in.Script) == "" {
					return nil, NewToolError("Script is required for 'run_js' action.")
				}
			case "wait_for":
				if strings.TrimSpace(in.Selector) == "" && strings.TrimSpace(in.URLContains) == "" && in.Text == "" {
					return nil, NewToolError("'wait_for' needs at least one of selector, urlContains, or text.")
				}
			case "click", "type":
				if strings.TrimSpace(in.Ref) == "" {
					return nil, NewToolError("'%s' needs a ref from a 'snapshot'.", action)
				}
				if action == "type" && in.Text == "" {
					return nil, NewToolError("'type' needs text.")
				}
			case "snapshot", "screenshot", "get_content", "tabs":
				// valid
			default:
				return nil, NewToolError("Unknown action: '%s'. Supported actions: navigate, run_js, get_content, tabs, snapshot, click, type, wait_for, screenshot.", action)
			}

			if handler == nil {
				return nil, NewToolError("Desktop browser is unavailable (no desktop client connected). Use webFetch or webSearch instead.")
			}

			req := BrowserActionRequest{
				RequestID: utils.RandomID(),
				Action:    action,
				URL:       in.URL,
				Script:    in.Script,
				Selector:  in.Selector,
				TabID:     in.TabID,

				URLContains: in.URLContains,
				Text:        in.Text,
				TimeoutMs:   in.TimeoutMs,
				Ref:         strings.TrimSpace(in.Ref),
				Submit:      in.Submit,
			}

			res, err := handler(ctx, req)
			if err != nil {
				return nil, err
			}
			if res.Error != "" {
				return nil, NewToolError("Browser action failed: %s", res.Error)
			}
			return textResult(res.Result), nil
		},
	)
}

// Browser is the unbound default instance used by DefaultTools() (unbound/headless).
var Browser = NewBrowserTool(nil)
