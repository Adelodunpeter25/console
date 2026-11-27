package tests

import (
	"io"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/routes"
	fssvc "github.com/Adelodunpeter25/console/apps/server-go/internal/services/fs"
)

func TestFsHtmlRouteServesSandboxedWithBootstrap(t *testing.T) {
	app := fiber.New()
	routes.RegisterFsRoutes(app, fssvc.NewFsService(), nil)

	dir := t.TempDir()
	page := filepath.Join(dir, "demo.html")
	if err := os.WriteFile(page, []byte("<!doctype html><html><head></head><body>hi</body></html>"), 0o644); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest("GET", "/api/fs/file/html?path="+page+"&theme=light", nil)
	resp, err := app.Test(req, 10000)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if csp := resp.Header.Get("Content-Security-Policy"); !strings.Contains(csp, "sandbox") || strings.Contains(csp, "allow-same-origin") {
		t.Fatalf("bad CSP header: %q", csp)
	}
	bodyBytes, _ := io.ReadAll(resp.Body)
	body := string(bodyBytes)
	if !strings.Contains(body, `id="console-theme"`) || !strings.Contains(body, "color-scheme:light") {
		t.Fatalf("missing light theme bootstrap in body: %s", body)
	}

	// Non-HTML extension rejected.
	txt := filepath.Join(dir, "notes.txt")
	_ = os.WriteFile(txt, []byte("<!doctype html><html></html>"), 0o644)
	resp2, _ := app.Test(httptest.NewRequest("GET", "/api/fs/file/html?path="+txt, nil), 10000)
	if resp2.StatusCode != 403 {
		t.Fatalf("non-html status = %d, want 403", resp2.StatusCode)
	}

	// Non-document content rejected even with .html extension.
	fake := filepath.Join(dir, "env.html")
	_ = os.WriteFile(fake, []byte("SECRET=123"), 0o644)
	resp3, _ := app.Test(httptest.NewRequest("GET", "/api/fs/file/html?path="+fake, nil), 10000)
	if resp3.StatusCode != 403 {
		t.Fatalf("non-doc status = %d, want 403", resp3.StatusCode)
	}
}
