package tests

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/loop"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/tools"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/run"
	"github.com/Adelodunpeter25/console/apps/server-go/tests/helpers"
)

func TestBrowserTool_HeadlessDefaults(t *testing.T) {
	tool := tools.NewBrowserTool(nil)

	// navigate
	args := helpers.MustJSONRaw(t, map[string]any{"action": "navigate", "url": "http://localhost:3000"})
	res, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	text := resultText(t, res)
	if !strings.Contains(text, "Navigated to http://localhost:3000") {
		t.Fatalf("unexpected output: %s", text)
	}

	// run_js
	args = helpers.MustJSONRaw(t, map[string]any{"action": "run_js", "script": "document.title"})
	res, err = tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	text = resultText(t, res)
	if !strings.Contains(text, "Evaluated script: document.title") {
		t.Fatalf("unexpected output: %s", text)
	}

	// screenshot
	args = helpers.MustJSONRaw(t, map[string]any{"action": "screenshot"})
	res, err = tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	text = resultText(t, res)
	if !strings.Contains(text, "Screenshot captured") {
		t.Fatalf("unexpected output: %s", text)
	}

	// get_content
	args = helpers.MustJSONRaw(t, map[string]any{"action": "get_content"})
	res, err = tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	text = resultText(t, res)
	if !strings.Contains(text, "Page content retrieved") {
		t.Fatalf("unexpected output: %s", text)
	}
}

func TestBrowserTool_Validation(t *testing.T) {
	tool := tools.NewBrowserTool(nil)

	// Missing URL on navigate
	args := helpers.MustJSONRaw(t, map[string]any{"action": "navigate"})
	_, err := tool.Execute(context.Background(), args)
	if err == nil || !strings.Contains(err.Error(), "URL is required") {
		t.Fatalf("expected URL error, got %v", err)
	}

	// Missing script on run_js
	args = helpers.MustJSONRaw(t, map[string]any{"action": "run_js"})
	_, err = tool.Execute(context.Background(), args)
	if err == nil || !strings.Contains(err.Error(), "Script is required") {
		t.Fatalf("expected script error, got %v", err)
	}

	// Unknown action
	args = helpers.MustJSONRaw(t, map[string]any{"action": "invalid_action"})
	_, err = tool.Execute(context.Background(), args)
	if err == nil {
		t.Fatalf("expected error for invalid action, got nil")
	}
}

func TestBrowserTool_WithHandler(t *testing.T) {
	var capturedReq tools.BrowserActionRequest
	handler := func(ctx context.Context, req tools.BrowserActionRequest) (tools.BrowserActionResult, error) {
		capturedReq = req
		return tools.BrowserActionResult{
			Result: "Console Title - 200 OK",
		}, nil
	}

	tool := tools.NewBrowserTool(handler)
	args := helpers.MustJSONRaw(t, map[string]any{
		"action":   "run_js",
		"script":   "document.title",
		"selector": "#root",
	})
	res, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if capturedReq.Action != "run_js" || capturedReq.Script != "document.title" || capturedReq.Selector != "#root" {
		t.Fatalf("unexpected captured request: %+v", capturedReq)
	}
	text := resultText(t, res)
	if text != "Console Title - 200 OK" {
		t.Fatalf("unexpected result: %s", text)
	}
}

func TestBrowserTool_HandlerError(t *testing.T) {
	handler := func(ctx context.Context, req tools.BrowserActionRequest) (tools.BrowserActionResult, error) {
		return tools.BrowserActionResult{
			Error: "Element not found",
		}, nil
	}

	tool := tools.NewBrowserTool(handler)
	args := helpers.MustJSONRaw(t, map[string]any{
		"action": "run_js",
		"script": "document.querySelector('#nonexistent').click()",
	})
	_, err := tool.Execute(context.Background(), args)
	if err == nil || !strings.Contains(err.Error(), "Element not found") {
		t.Fatalf("expected element not found error, got %v", err)
	}
}

func TestBrowserDecisionsFlow(t *testing.T) {
	decisions := run.NewDecisions()
	decisions.Timeout = 2 * time.Second

	hub := run.NewHub()
	defer hub.Close("done")

	zero := int64(0)
	id, ch, _ := hub.Subscribe(&zero)
	defer hub.Unsubscribe(id)

	handler := decisions.BrowserHandlerFor("sess_1", hub)

	go func() {
		// Wait for hub broadcast
		for f := range ch {
			if f.Event.Kind == loop.EventBrowserAction && f.Event.Browser != nil {
				decisions.ResolveBrowserAction("sess_1", f.Event.Browser.RequestID, tools.BrowserActionResult{
					Result: "Navigation successful",
				})
				return
			}
		}
	}()

	res, err := handler(context.Background(), tools.BrowserActionRequest{
		RequestID: "req_test",
		Action:    "navigate",
		URL:       "http://localhost:5173",
	})
	if err != nil {
		t.Fatalf("handler failed: %v", err)
	}
	if res.Result != "Navigation successful" {
		t.Fatalf("unexpected result: %s", res.Result)
	}
}
