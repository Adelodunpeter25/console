// /api/mcp route coverage through a real Fiber app.
package tests

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/routes"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/services/mcp"
)

func do(t *testing.T, app *fiber.App, method, path, body string) (int, map[string]any) {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req, 10000)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := io.ReadAll(resp.Body)
	var out map[string]any
	_ = json.Unmarshal(raw, &out)
	return resp.StatusCode, out
}

func TestMCPRoutesLifecycle(t *testing.T) {
	ts := startHTTPMCP(t, requireBearer("Bearer tok"))
	m := newManager(t)
	app := fiber.New()
	routes.RegisterMCPRoutes(app, m)

	code, _ := do(t, app, "POST", "/api/mcp/servers", `{"id":"bad id","label":"x","transport":"http","url":"u"}`)
	if code != http.StatusBadRequest {
		t.Fatalf("invalid config should 400, got %d", code)
	}

	body := `{"id":"echo","label":"Echo","transport":"http","url":"` + ts.URL + `","enabled":true,"auth":{"type":"static"},"token":"tok"}`
	if code, out := do(t, app, "POST", "/api/mcp/servers", body); code != 200 || out["success"] != true {
		t.Fatalf("create: %d %v", code, out)
	}
	// Secrets must never come back over the API or land in the main config.
	_, out := do(t, app, "GET", "/api/mcp/servers/echo", "")
	if strings.Contains(toJSON(out), "tok\"") && strings.Contains(toJSON(out), "Bearer") {
		t.Fatalf("token leaked in response: %v", out)
	}
	if cred, ok, _ := m.Credentials.Get("echo"); !ok || cred.Header != "Bearer tok" {
		t.Fatalf("bare token should be stored as Bearer header: %+v", cred)
	}

	if code, _ := do(t, app, "POST", "/api/mcp/servers/echo/connect", ""); code != http.StatusAccepted {
		t.Fatalf("connect: %d", code)
	}
	if err := m.Ensure(ctx5(t), "echo"); err != nil {
		t.Fatal(err)
	}
	_, out = do(t, app, "GET", "/api/mcp/servers", "")
	list := out["data"].([]any)
	if len(list) != 1 || list[0].(map[string]any)["status"] != mcp.StatusConnected {
		t.Fatalf("list: %v", out)
	}

	if code, _ := do(t, app, "DELETE", "/api/mcp/servers/echo", ""); code != 200 {
		t.Fatalf("delete: %d", code)
	}
	if _, ok, _ := m.Credentials.Get("echo"); ok {
		t.Fatal("delete should remove the credential")
	}
	if code, _ := do(t, app, "GET", "/api/mcp/servers/echo", ""); code != http.StatusNotFound {
		t.Fatalf("after delete want 404, got %d", code)
	}
}
