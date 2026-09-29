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
}

// BrowserActionResult is the result received from the desktop client.
type BrowserActionResult struct {
	Result string `json:"result,omitempty"`
	Error  string `json:"error,omitempty"`
}

// BrowserHandler handles one browser action by delegating to the client.
type BrowserHandler func(ctx context.Context, req BrowserActionRequest) (BrowserActionResult, error)

type browserInput struct {
	Action   string `json:"action" jsonschema:"required,enum=navigate,enum=run_js,enum=get_content,enum=tabs,enum=screenshot,description=Browser action: 'navigate' to a URL (returns the tab id), 'run_js' to evaluate JavaScript and get its return value, 'get_content' to read page text (or the text of a selector), 'tabs' to list open tabs, or 'screenshot' (not supported yet)."`
	URL      string `json:"url,omitempty" jsonschema:"description=URL to navigate to (required for 'navigate')."`
	Script   string `json:"script,omitempty" jsonschema:"description=JavaScript expression or function to evaluate (required for 'run_js'). The value it returns (promises are awaited) is sent back as the result."`
	Selector string `json:"selector,omitempty" jsonschema:"description=Optional CSS selector for 'get_content'. Returns the text of matching elements, or a no-match message."`
	TabID    string `json:"tabId,omitempty" jsonschema:"description=Tab id from 'navigate' or 'tabs'. Omit to use the active browser tab (for 'navigate': a tab already on that URL, else a new tab)."`
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
			case "screenshot", "get_content", "tabs":
				// valid
			default:
				return nil, NewToolError("Unknown action: '%s'. Supported actions: navigate, run_js, get_content, tabs, screenshot.", action)
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
