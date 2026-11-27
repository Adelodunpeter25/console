package htmlrender

import (
	"context"
	"github.com/MoeForge/moego/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/tools"
)

func TestRejectViewportUnits(t *testing.T) {
	bad := []string{
		`<style>body{height:100vh}</style>`,
		`<style>.box{min-height:50dvh}</style>`,
		`<div style="width:80vw"></div>`,
	}
	for _, html := range bad {
		if err := RejectViewportUnits(html); err == nil {
			t.Errorf("expected viewport unit error for %s", html)
		}
	}
	good := []string{
		`<style>body{height:400px}</style>`,
		`<img src="data:image/png;base64,abc1vhdef">`,
		`<script>const s = "100vh";</script>`,
	}
	for _, html := range good {
		if err := RejectViewportUnits(html); err != nil {
			t.Errorf("unexpected error for %s: %v", html, err)
		}
	}
}

func TestInjectBootstrapSkipsScriptLiterals(t *testing.T) {
	src := `<!doctype html><html><script>const x = "<head>";</script><head><title>Hi</title></head><body></body></html>`
	out := InjectBootstrap(src, `<style id="console-theme"></style>`)
	if strings.Contains(out, `const x = "<head><style`) {
		t.Fatalf("bootstrap injected inside script literal: %s", out)
	}
	if !strings.Contains(out, `<head><style id="console-theme"></style><title>Hi</title>`) {
		t.Fatalf("bootstrap not injected into real <head>: %s", out)
	}
}

func TestRenderToolInlineAndPath(t *testing.T) {
	dir := t.TempDir()
	tool := NewRenderTool(Options{Cwd: dir, ScratchDir: filepath.Join(dir, "scratch")})

	// Inline HTML writes scratch file and returns HtmlArtifact.
	args, _ := json.Marshal(map[string]any{
		"html":   "<!doctype html><html><body>hi</body></html>",
		"title":  "Demo",
		"height": 420,
	})
	out, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("inline render: %v", err)
	}
	env, ok := out.(tools.Envelope)
	if !ok || env.Html == nil {
		t.Fatalf("expected Envelope with Html, got %#v", out)
	}
	if !strings.HasPrefix(env.Html.Path, dir) {
		t.Fatalf("scratch path %q not under %q", env.Html.Path, dir)
	}
	if _, err := os.Stat(env.Html.Path); err != nil {
		t.Fatalf("scratch file missing: %v", err)
	}

	// Path mode reads existing file.
	page := filepath.Join(dir, "custom.html")
	if err := os.WriteFile(page, []byte("<html><body>ok</body></html>"), 0o644); err != nil {
		t.Fatal(err)
	}
	args2, _ := json.Marshal(map[string]any{"path": "custom.html", "title": "Custom"})
	out2, err := tool.Execute(context.Background(), args2)
	if err != nil {
		t.Fatalf("path render: %v", err)
	}
	if out2.(tools.Envelope).Html.Path != page {
		t.Fatalf("path = %q, want %q", out2.(tools.Envelope).Html.Path, page)
	}

	// Non-HTML document rejected.
	envFile := filepath.Join(dir, "secret.html")
	_ = os.WriteFile(envFile, []byte("API_KEY=secret"), 0o644)
	args3, _ := json.Marshal(map[string]any{"path": "secret.html"})
	if _, err := tool.Execute(context.Background(), args3); err == nil {
		t.Fatal("expected non-HTML document to be rejected")
	}
}

func TestPreviewToolDrivesBrowserBridge(t *testing.T) {
	var actions []string
	bridge := func(_ context.Context, req tools.BrowserActionRequest) (tools.BrowserActionResult, error) {
		actions = append(actions, req.Action)
		switch req.Action {
		case "navigate":
			return tools.BrowserActionResult{Result: "Tab: tab-1\nTitle: Test\nURL: data:..."}, nil
		case "run_js":
			return tools.BrowserActionResult{Result: `{"height":512,"title":"Test","errors":[]}`}, nil
		case "screenshot":
			return tools.BrowserActionResult{ImageBase64: "iVBORw0KGgo="}, nil
		default:
			return tools.BrowserActionResult{}, nil
		}
	}
	tool := NewPreviewTool(Options{Cwd: t.TempDir()}, bridge)
	args, _ := json.Marshal(map[string]any{
		"html":  "<!doctype html><html><body><button>Go</button></body></html>",
		"click": map[string]any{"text": "Go"},
	})
	out, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("preview: %v", err)
	}
	parts, ok := out.([]map[string]any)
	if !ok || len(parts) != 2 || parts[1]["type"] != "image" {
		t.Fatalf("unexpected preview parts: %#v", out)
	}
	if !strings.Contains(parts[0]["text"].(string), "512px") {
		t.Fatalf("summary missing height: %v", parts[0]["text"])
	}
	want := []string{"navigate", "click", "run_js", "screenshot", "close_tab"}
	if strings.Join(actions, ",") != strings.Join(want, ",") {
		t.Fatalf("actions = %v, want %v", actions, want)
	}
}
