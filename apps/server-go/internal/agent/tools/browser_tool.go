// browser tool: native desktop webview control (navigate, run_js, screenshot, get_content).
package tools

import (
	"context"
	"fmt"
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
}

// BrowserActionResult is the result received from the desktop client.
type BrowserActionResult struct {
	Result string `json:"result,omitempty"`
	Error  string `json:"error,omitempty"`
}

// BrowserHandler handles one browser action by delegating to the client.
type BrowserHandler func(ctx context.Context, req BrowserActionRequest) (BrowserActionResult, error)

type browserInput struct {
	Action   string `json:"action" jsonschema:"required,enum=navigate,enum=run_js,enum=screenshot,enum=get_content,description=Browser action: 'navigate' to a URL, 'run_js' to evaluate JavaScript, 'screenshot' to capture page image, or 'get_content' to read page text."`
	URL      string `json:"url,omitempty" jsonschema:"description=URL to navigate to (required for 'navigate')."`
	Script   string `json:"script,omitempty" jsonschema:"description=JavaScript expression or function to evaluate (required for 'run_js')."`
	Selector string `json:"selector,omitempty" jsonschema:"description=Optional CSS selector for targeted inspection or DOM query."`
}

func headlessBrowserResult(in browserInput) string {
	switch in.Action {
	case "navigate":
		return fmt.Sprintf("[Browser (Headless)]: Navigated to %s", in.URL)
	case "run_js":
		return fmt.Sprintf("[Browser (Headless)]: Evaluated script: %s", in.Script)
	case "screenshot":
		return "[Browser (Headless)]: Screenshot captured."
	case "get_content":
		return "[Browser (Headless)]: Page content retrieved."
	default:
		return fmt.Sprintf("[Browser (Headless)]: Action %s executed.", in.Action)
	}
}

// NewBrowserTool builds the "browser" tool bound to handler.
func NewBrowserTool(handler BrowserHandler) Tool {
	return NewTool(
		"browser",
		"Control the built-in desktop browser view to inspect dev servers, navigate pages, execute JavaScript, capture screenshots, or extract rendered DOM content.",
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
			case "screenshot", "get_content":
				// valid
			default:
				return nil, NewToolError("Unknown action: '%s'. Supported actions: navigate, run_js, screenshot, get_content.", action)
			}

			if handler == nil {
				return textResult(headlessBrowserResult(in)), nil
			}

			req := BrowserActionRequest{
				RequestID: utils.RandomID(),
				Action:    action,
				URL:       in.URL,
				Script:    in.Script,
				Selector:  in.Selector,
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

// Browser is the unbound default instance used by DefaultTools() (headless).
var Browser = NewBrowserTool(nil)
