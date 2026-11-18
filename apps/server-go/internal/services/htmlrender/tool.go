package htmlrender

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/tools"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/services/fs"
)

// Options binds the tools to one session's workspace.
type Options struct {
	// Cwd is the session working directory; relative paths resolve against it.
	Cwd string
	// ScratchDir is where inline html is written when Cwd has no writable
	// .console dir. Empty falls back to Cwd/.console/renders.
	ScratchDir string
}

// HtmlArtifact is the client-rendered page riding on a tool result. Path is
// the workspace .html file; Snapshot is an optional PNG for the collapsed
// card state.
type HtmlArtifact struct {
	Path     string `json:"path"`
	Title    string `json:"title"`
	Height   uint32 `json:"height"`
	Snapshot []byte `json:"snapshot,omitempty"`
}

type renderInput struct {
	Path   string `json:"path,omitempty" jsonschema:"description=Path to an existing .html file in the workspace. Mutually exclusive with html."`
	HTML   string `json:"html,omitempty" jsonschema:"description=Inline HTML document (self-contained: inline CSS/JS, images as data URIs or absolute /api/fs/file/raw URLs). Mutually exclusive with path."`
	Title  string `json:"title,omitempty" jsonschema:"description=Card title shown above the page (max 200 chars)."`
	Height int    `json:"height,omitempty" jsonschema:"description=Preferred frame height in CSS pixels, 80-2000. Omit to auto-fit."`
}

// NewRenderTool returns the render tool bound to opts.
func NewRenderTool(opts Options) tools.Tool {
	return tools.NewTool(
		"render",
		"Publish an HTML page inline in the chat: a chart, mockup, dashboard, or interactive widget. Pass html (inline document) or path (existing .html file). The page is self-contained — inline CSS/JS, no external assets except absolute /api/fs/file/raw URLs. Verify first with preview when the desktop app is connected.",
		tools.TierRead,
		func(_ context.Context, in renderInput) (any, error) {
			return executeRender(opts, in)
		},
	)
}

func executeRender(opts Options, in renderInput) (any, error) {
	path, html, err := resolvePage(opts, in.Path, in.HTML)
	if err != nil {
		return nil, err
	}
	if err := RejectViewportUnits(html); err != nil {
		return nil, tools.NewToolError("%v", err)
	}
	title := strings.TrimSpace(in.Title)
	if len([]rune(title)) > MaxTitleRunes {
		title = string([]rune(title)[:MaxTitleRunes])
	}
	var height uint32
	if in.Height > 0 {
		height = uint32(min(max(in.Height, 80), 2000))
	}
	return tools.Envelope{
		Content: []map[string]any{{"type": "text", "text": fmt.Sprintf("Rendered interactive page %q (%s).", filepath.Base(path), path)}},
		Html: &tools.HtmlArtifact{
			Path:   path,
			Title:  title,
			Height: height,
		},
	}, nil
}

// validatePage reads/validates path or inline html without writing a scratch file.
func validatePage(opts Options, pathArg, htmlArg string) (absPath string, html string, isInline bool, err error) {
	pathArg = strings.TrimSpace(pathArg)
	htmlArg = strings.TrimSpace(htmlArg)
	if (pathArg == "") == (htmlArg == "") {
		return "", "", false, tools.NewToolError("pass exactly one of path or html")
	}
	if pathArg != "" {
		if !IsHTMLPath(pathArg) {
			return "", "", false, tools.NewToolError("%q is not an .html file", pathArg)
		}
		abs := resolveWorkspacePath(opts.Cwd, pathArg)
		data, err := os.ReadFile(abs)
		if err != nil {
			return "", "", false, tools.NewToolError("cannot read %s: %v", pathArg, err)
		}
		if len(data) > MaxFileBytes {
			return "", "", false, tools.NewToolError("%s is %d bytes; pages are capped at %d", pathArg, len(data), MaxFileBytes)
		}
		doc := string(data)
		if !LooksLikeDocument(doc) {
			return "", "", false, tools.NewToolError("%s does not look like an HTML document (must start with <!doctype html> or <html>)", pathArg)
		}
		return abs, doc, false, nil
	}
	if len(htmlArg) > MaxInlineBytes {
		return "", "", false, tools.NewToolError("inline html is %d bytes; cap is %d — write it to a file and pass path instead", len(htmlArg), MaxInlineBytes)
	}
	if !LooksLikeDocument(htmlArg) {
		return "", "", false, tools.NewToolError("html must start with <!doctype html> or <html>")
	}
	return "", htmlArg, true, nil
}

// resolvePage validates the input and writes inline html to the session scratch dir.
func resolvePage(opts Options, pathArg, htmlArg string) (string, string, error) {
	abs, doc, isInline, err := validatePage(opts, pathArg, htmlArg)
	if err != nil {
		return "", "", err
	}
	if isInline {
		abs, err = writeScratchPage(opts, doc)
		if err != nil {
			return "", "", tools.NewToolError("cannot write page: %v", err)
		}
	}
	return abs, doc, nil
}

func resolveWorkspacePath(cwd, p string) string {
	p = fs.ResolvePath(p)
	if filepath.IsAbs(p) {
		return filepath.Clean(p)
	}
	if cwd != "" {
		return filepath.Join(cwd, p)
	}
	abs, _ := filepath.Abs(p)
	return abs
}

func writeScratchPage(opts Options, html string) (string, error) {
	dir := opts.ScratchDir
	if dir == "" {
		dir = filepath.Join(opts.Cwd, ".console", "renders")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	sum := sha256.Sum256([]byte(html))
	name := "page-" + time.Now().Format("20060102-150405") + "-" + hex.EncodeToString(sum[:])[:8] + ".html"
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(html), 0o644); err != nil {
		return "", err
	}
	return path, nil
}