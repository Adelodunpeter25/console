// GET /api/mcp/servers must hand the desktop everything the settings page
// needs in one round trip.
//
// The list entries are what the edit form is populated from, so a summary
// without url/env/auth would re-save the server with those fields blank. The
// same entry also carries the live status and tool list the row renders.
package tests

import (
	"net/http"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/routes"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/services/mcp"
)

func TestListCarriesFullConfigAndLiveState(t *testing.T) {
	ts := startHTTPMCP(t, requireBearer("Bearer s3cret"))
	m := newManager(t)
	app := fiber.New()
	routes.RegisterMCPRoutes(app, m)

	// Saved through the desktop's field names — `name` for the label,
	// `auth_type` for auth, env as a pair array — so this exercises the same
	// payload the client sends.
	body := `{"id":"atlassian","name":"Atlassian","transport":"http","url":"` + ts.URL +
		`","auth_type":"static","env":[["REGION","eu"]],"enabled":true,"token":"s3cret"}`
	if code, out := do(t, app, "POST", "/api/mcp/servers", body); code != http.StatusOK || out["success"] != true {
		t.Fatalf("save: %d %v", code, out)
	}
	if err := m.Ensure(ctx5(t), "atlassian"); err != nil {
		t.Fatalf("ensure: %v", err)
	}

	code, out := do(t, app, "GET", "/api/mcp/servers", "")
	if code != http.StatusOK || out["success"] != true {
		t.Fatalf("list: %d %v", code, out)
	}
	raw := toJSON(out)
	data, ok := out["data"].([]any)
	if !ok || len(data) != 1 {
		t.Fatalf("data: %v", out)
	}
	entry, ok := data[0].(map[string]any)
	if !ok {
		t.Fatalf("entry: %T", data[0])
	}

	// Identity and live state, which the row already showed before.
	if entry["id"] != "atlassian" || entry["label"] != "Atlassian" {
		t.Fatalf("identity: %v", entry)
	}
	if entry["status"] != mcp.StatusConnected {
		t.Fatalf("status = %v, want %v", entry["status"], mcp.StatusConnected)
	}

	// The stored config, read back by the edit form before re-saving.
	if entry["url"] != ts.URL {
		t.Fatalf("url = %v, want %q — the edit form would blank it", entry["url"], ts.URL)
	}
	if entry["enabled"] != true {
		t.Fatalf("enabled = %v, want true", entry["enabled"])
	}
	env, _ := entry["env"].(map[string]any)
	if env["REGION"] != "eu" {
		t.Fatalf("env = %v, want REGION=eu", entry["env"])
	}
	auth, _ := entry["auth"].(map[string]any)
	if auth["type"] != mcp.AuthStatic || auth["tokenRef"] != "atlassian" {
		t.Fatalf("auth = %v, want static with tokenRef atlassian", entry["auth"])
	}

	// The tool list the expanded row renders.
	tools, _ := entry["tools"].([]any)
	if len(tools) == 0 {
		t.Fatalf("tools missing from list entry: %v", entry)
	}
	if entry["toolCount"] != float64(len(tools)) {
		t.Fatalf("toolCount = %v, want %d", entry["toolCount"], len(tools))
	}
	if name := tools[0].(map[string]any)["name"]; name == "" {
		t.Fatalf("tool has no name: %v", tools[0])
	}

	// Embedding the config must not start echoing credentials back.
	if strings.Contains(raw, "Bearer s3cret") {
		t.Fatalf("credential leaked in list response: %s", raw)
	}
}
