package htmlrender

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/tools"
)

type previewClick struct {
	Selector string `json:"selector,omitempty" jsonschema:"description=CSS selector to click."`
	Text     string `json:"text,omitempty" jsonschema:"description=Visible text of element to click."`
	Ref      string `json:"ref,omitempty" jsonschema:"description=Snapshot ref to click."`
}

type previewType struct {
	Selector string `json:"selector,omitempty" jsonschema:"description=CSS selector of input."`
	Ref      string `json:"ref,omitempty" jsonschema:"description=Snapshot ref of input."`
	Text     string `json:"text" jsonschema:"required,description=Text to type."`
}

type previewInput struct {
	Path       string        `json:"path,omitempty" jsonschema:"description=Path to an existing .html file in the workspace. Mutually exclusive with html."`
	HTML       string        `json:"html,omitempty" jsonschema:"description=Inline HTML document. Mutually exclusive with path."`
	Appearance string        `json:"appearance,omitempty" jsonschema:"description=Theme appearance: dark (default) or light."`
	Click      *previewClick `json:"click,omitempty" jsonschema:"description=Optional element to click after load before taking the screenshot, to verify interactivity."`
	Type       *previewType  `json:"type,omitempty" jsonschema:"description=Optional text to type after load before taking the screenshot."`
}

// NewPreviewTool returns the preview tool bound to opts and the session's
// desktop browser bridge. When browser is nil (no desktop connected), calls
// return a clear error telling the model it can still publish with render.
func NewPreviewTool(opts Options, browser tools.BrowserHandler) tools.Tool {
	return tools.NewTool(
		"preview",
		"Verify an HTML page before calling render: opens it in the desktop browser, optionally clicks/types to test interactivity, and returns a screenshot PNG, measured contentHeight, and any console errors.",
		tools.TierRead,
		func(ctx context.Context, in previewInput) (any, error) {
			return executePreview(ctx, opts, browser, in)
		},
	)
}

func executePreview(ctx context.Context, opts Options, browser tools.BrowserHandler, in previewInput) (any, error) {
	if browser == nil {
		return nil, tools.NewToolError("preview requires the desktop app to be connected; you can still publish directly with render")
	}
	_, rawHTML, _, err := validatePage(opts, in.Path, in.HTML)
	if err != nil {
		return nil, err
	}
	if err := RejectViewportUnits(rawHTML); err != nil {
		return nil, tools.NewToolError("%v", err)
	}
	theme := ThemeFor(in.Appearance)
	prepared := InjectBootstrap(rawHTML, BootstrapMarkup(theme))
	dataURL := "data:text/html;charset=utf-8;base64," + base64.StdEncoding.EncodeToString([]byte(prepared))

	navRes, err := browser(ctx, tools.BrowserActionRequest{
		Action: "navigate",
		URL:    dataURL,
		NewTab: true,
	})
	if err != nil {
		return nil, tools.NewToolError("preview failed to open browser tab: %v", err)
	}
	if navRes.Error != "" {
		return nil, tools.NewToolError("preview failed: %s", navRes.Error)
	}
	tabID := parseTabID(navRes.Result)
	if tabID != "" {
		defer func() {
			_, _ = browser(context.Background(), tools.BrowserActionRequest{
				Action: "close_tab",
				TabID:  tabID,
			})
		}()
	}

	if in.Type != nil && in.Type.Text != "" {
		_, _ = browser(ctx, tools.BrowserActionRequest{
			Action:   "type",
			TabID:    tabID,
			Selector: in.Type.Selector,
			Ref:      in.Type.Ref,
			Text:     in.Type.Text,
		})
	}
	if in.Click != nil && (in.Click.Selector != "" || in.Click.Text != "" || in.Click.Ref != "") {
		sel := in.Click.Selector
		if sel == "" && in.Click.Text != "" {
			sel = "text=" + in.Click.Text
		}
		_, _ = browser(ctx, tools.BrowserActionRequest{
			Action:   "click",
			TabID:    tabID,
			Selector: sel,
			Ref:      in.Click.Ref,
		})
	}

	metricsRes, _ := browser(ctx, tools.BrowserActionRequest{
		Action: "run_js",
		TabID:  tabID,
		Script: `JSON.stringify({height: Math.ceil(Math.max(document.documentElement.scrollHeight, document.documentElement.getBoundingClientRect().height)), title: document.title || "", errors: window.__consoleErrors || []})`,
	})

	shotRes, err := browser(ctx, tools.BrowserActionRequest{
		Action: "screenshot",
		TabID:  tabID,
	})
	if err != nil {
		return nil, tools.NewToolError("preview screenshot failed: %v", err)
	}
	if shotRes.Error != "" {
		return nil, tools.NewToolError("preview screenshot failed: %s", shotRes.Error)
	}

	summary := formatPreviewSummary(metricsRes.Result)
	parts := []map[string]any{{"type": "text", "text": summary}}
	if shotRes.ImageBase64 != "" {
		parts = append(parts, map[string]any{
			"type":     "image",
			"data":     shotRes.ImageBase64,
			"mimeType": "image/png",
		})
	}
	return parts, nil
}

func parseTabID(text string) string {
	for _, line := range strings.Split(text, "\n") {
		if strings.HasPrefix(line, "Tab: ") {
			return strings.TrimSpace(strings.TrimPrefix(line, "Tab: "))
		}
	}
	return ""
}

func formatPreviewSummary(raw string) string {
	var m struct {
		Height int    `json:"height"`
		Title  string `json:"title"`
		Errors []struct {
			Level string `json:"level"`
			Text  string `json:"text"`
		} `json:"errors"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(raw)), &m); err != nil {
		return "Preview captured."
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Preview captured (contentHeight: %dpx", m.Height)
	if m.Title != "" {
		fmt.Fprintf(&b, ", title: %q", m.Title)
	}
	b.WriteString(").")
	if len(m.Errors) > 0 {
		b.WriteString("\nConsole messages:")
		for _, e := range m.Errors {
			fmt.Fprintf(&b, "\n- [%s] %s", e.Level, e.Text)
		}
	}
	return b.String()
}