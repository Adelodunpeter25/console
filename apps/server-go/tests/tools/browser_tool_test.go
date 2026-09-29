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

func TestBrowserTool_UnboundFailsExplicitly(t *testing.T) {
	tool := tools.NewBrowserTool(nil)

	// navigate
	args := helpers.MustJSONRaw(t, map[string]any{"action": "navigate", "url": "http://localhost:3000"})
	_, err := tool.Execute(context.Background(), args)
	if err == nil || !strings.Contains(err.Error(), "Desktop browser is unavailable") {
		t.Fatalf("expected unavailable error, got %v", err)
	}

	// run_js
	args = helpers.MustJSONRaw(t, map[string]any{"action": "run_js", "script": "document.title"})
	_, err = tool.Execute(context.Background(), args)
	if err == nil || !strings.Contains(err.Error(), "Desktop browser is unavailable") {
		t.Fatalf("expected unavailable error, got %v", err)
	}

	// screenshot
	args = helpers.MustJSONRaw(t, map[string]any{"action": "screenshot"})
	_, err = tool.Execute(context.Background(), args)
	if err == nil || !strings.Contains(err.Error(), "Desktop browser is unavailable") {
		t.Fatalf("expected unavailable error, got %v", err)
	}

	// get_content
	args = helpers.MustJSONRaw(t, map[string]any{"action": "get_content"})
	_, err = tool.Execute(context.Background(), args)
	if err == nil || !strings.Contains(err.Error(), "Desktop browser is unavailable") {
		t.Fatalf("expected unavailable error, got %v", err)
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

func TestBrowserTool_TabsAndTabID(t *testing.T) {
	var captured tools.BrowserActionRequest
	handler := func(ctx context.Context, req tools.BrowserActionRequest) (tools.BrowserActionResult, error) {
		captured = req
		return tools.BrowserActionResult{Result: "- browser-1: \"Home\" http://localhost:3000"}, nil
	}
	tool := tools.NewBrowserTool(handler)

	if _, err := tool.Execute(context.Background(), helpers.MustJSONRaw(t, map[string]any{"action": "tabs"})); err != nil {
		t.Fatalf("tabs action failed: %v", err)
	}
	if captured.Action != "tabs" {
		t.Fatalf("expected tabs action, got %+v", captured)
	}

	args := helpers.MustJSONRaw(t, map[string]any{"action": "get_content", "tabId": "browser-1"})
	if _, err := tool.Execute(context.Background(), args); err != nil {
		t.Fatalf("get_content failed: %v", err)
	}
	if captured.TabID != "browser-1" {
		t.Fatalf("tabId not forwarded: %+v", captured)
	}
}

func TestBrowserTool_WaitForValidatesAndForwards(t *testing.T) {
	var captured tools.BrowserActionRequest
	handler := func(ctx context.Context, req tools.BrowserActionRequest) (tools.BrowserActionResult, error) {
		captured = req
		return tools.BrowserActionResult{Result: "Condition met"}, nil
	}
	tool := tools.NewBrowserTool(handler)

	_, err := tool.Execute(context.Background(), helpers.MustJSONRaw(t, map[string]any{"action": "wait_for"}))
	if err == nil || !strings.Contains(err.Error(), "wait_for") {
		t.Fatalf("expected wait_for condition error, got %v", err)
	}

	_, err = tool.Execute(context.Background(), helpers.MustJSONRaw(t, map[string]any{
		"action": "wait_for", "urlContains": "/s?k=", "text": "results", "selector": "#search", "timeoutMs": 5000,
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if captured.URLContains != "/s?k=" || captured.Text != "results" || captured.Selector != "#search" || captured.TimeoutMs != 5000 {
		t.Fatalf("wait_for fields not forwarded: %+v", captured)
	}
}

func TestBrowserTool_SnapshotClickTypeValidateAndForward(t *testing.T) {
	var captured tools.BrowserActionRequest
	handler := func(ctx context.Context, req tools.BrowserActionRequest) (tools.BrowserActionResult, error) {
		captured = req
		return tools.BrowserActionResult{Result: "ok"}, nil
	}
	tool := tools.NewBrowserTool(handler)

	for _, args := range []map[string]any{
		{"action": "click"},
		{"action": "type", "ref": "e1"},
		{"action": "type", "text": "hi"},
	} {
		if _, err := tool.Execute(context.Background(), helpers.MustJSONRaw(t, args)); err == nil {
			t.Fatalf("expected validation error for %v", args)
		}
	}

	if _, err := tool.Execute(context.Background(), helpers.MustJSONRaw(t, map[string]any{"action": "snapshot"})); err != nil {
		t.Fatalf("snapshot should need no args: %v", err)
	}
	if _, err := tool.Execute(context.Background(), helpers.MustJSONRaw(t, map[string]any{
		"action": "type", "ref": " e3 ", "text": "headphones", "submit": true,
	})); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if captured.Ref != "e3" || captured.Text != "headphones" || !captured.Submit {
		t.Fatalf("type fields not forwarded: %+v", captured)
	}
}

func TestBrowserTool_ScreenshotReturnsImagePart(t *testing.T) {
	tool := tools.NewBrowserTool(func(ctx context.Context, req tools.BrowserActionRequest) (tools.BrowserActionResult, error) {
		return tools.BrowserActionResult{Result: "Screenshot attached.", ImageBase64: "iVBORw0KGgo="}, nil
	})
	args := helpers.MustJSONRaw(t, map[string]any{"action": "screenshot"})
	out, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	parts, ok := out.([]map[string]any)
	if !ok || len(parts) != 2 {
		t.Fatalf("expected text + image parts, got %#v", out)
	}
	if parts[1]["type"] != "image" || parts[1]["data"] != "iVBORw0KGgo=" || parts[1]["mimeType"] != "image/png" {
		t.Fatalf("bad image part: %#v", parts[1])
	}
}
