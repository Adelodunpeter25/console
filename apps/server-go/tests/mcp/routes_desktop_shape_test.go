// Wire-format tolerance for MCP server saves.
//
// The desktop client serializes its McpServerConfig struct directly, which
// spells two fields differently from the server's canonical ServerConfig and
// omits a third outright: `name` instead of `label`, a flat `auth_type`
// instead of a nested `auth` object, and no `enabled` flag. `env` also posts
// as a pair array (see env_decode_test.go). None of these are real problems
// with the server the user is describing, so the save must not reject them.
package tests

import (
	"net/http"
	"testing"

	"github.com/gofiber/fiber/v2"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/routes"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/services/mcp"
)

// The exact shape the desktop client posts for a saved server: `name`,
// `auth_type`, a pair-array `env`, and no `enabled` (its type has no such
// field). This is the payload that produced "cannot unmarshal array into
// ... env" and "label must be a non-empty string".
const desktopServerPayload = `{
  "id": "atlassian",
  "name": "Atlassian",
  "transport": "http",
  "url": "https://mcp.example.com/mcp",
  "auth_type": "static",
  "env": [["TOKEN", "abc"], ["REGION", "eu"]],
  "status": "disconnected",
  "tools": []
}`

func TestSaveAcceptsDesktopFieldNames(t *testing.T) {
	m := newManager(t)
	app := fiber.New()
	routes.RegisterMCPRoutes(app, m)

	code, out := do(t, app, "POST", "/api/mcp/servers", desktopServerPayload)
	if code != http.StatusOK || out["success"] != true {
		t.Fatalf("desktop payload should save, got %d %v", code, out)
	}

	cfg, found, err := m.Config.Get("atlassian")
	if err != nil || !found {
		t.Fatalf("server not stored: %v found=%v", err, found)
	}
	// `name` has to land in label, or the saved server is anonymous.
	if cfg.Label != "Atlassian" {
		t.Fatalf("label = %q, want %q", cfg.Label, "Atlassian")
	}
	// A flat auth_type has to become a real auth entry, or the token has
	// nowhere to attach and auth silently reverts to none.
	if cfg.Auth == nil {
		t.Fatal("auth_type was dropped: cfg.Auth is nil")
	}
	if cfg.Auth.Type != mcp.AuthStatic {
		t.Fatalf("auth type = %q, want %q", cfg.Auth.Type, mcp.AuthStatic)
	}
	// tokenRef is derived from the id so a later token write can find it.
	if cfg.Auth.TokenRef != "atlassian" {
		t.Fatalf("tokenRef = %q, want %q", cfg.Auth.TokenRef, "atlassian")
	}
	if cfg.Env["TOKEN"] != "abc" || cfg.Env["REGION"] != "eu" {
		t.Fatalf("env not decoded from pair array: %v", cfg.Env)
	}
	// The client never sends `enabled`. Left at the zero value the server
	// stores it disabled and Connect refuses with "is disabled".
	if !cfg.Enabled {
		t.Fatal("enabled = false: a payload with no `enabled` field must save usable")
	}
}

func TestSaveHonoursAnExplicitlyDisabledServer(t *testing.T) {
	// Absent means "no opinion", not "override": a client that does send the
	// flag must still be able to keep a server off.
	m := newManager(t)
	app := fiber.New()
	routes.RegisterMCPRoutes(app, m)

	body := `{"id":"off","name":"Off","transport":"http",
	  "url":"https://example.com/mcp","enabled":false}`
	if code, out := do(t, app, "POST", "/api/mcp/servers", body); code != http.StatusOK || out["success"] != true {
		t.Fatalf("save: %d %v", code, out)
	}
	cfg, found, err := m.Config.Get("off")
	if err != nil || !found {
		t.Fatalf("not stored: %v found=%v", err, found)
	}
	if cfg.Enabled {
		t.Fatal("explicit enabled=false was overridden")
	}
	if err := m.Connect("off"); err == nil {
		t.Fatal("a disabled server should not connect")
	}
}

func TestSavePrefersCanonicalFieldNamesWhenBothSent(t *testing.T) {
	m := newManager(t)
	app := fiber.New()
	routes.RegisterMCPRoutes(app, m)

	// `label` and `auth` are the canonical spellings; the aliases must not
	// override them.
	body := `{"id":"both","label":"Canonical","name":"Alias","transport":"http",
	  "url":"https://example.com/mcp","auth":{"type":"oauth2"},"auth_type":"static"}`
	if code, out := do(t, app, "POST", "/api/mcp/servers", body); code != http.StatusOK || out["success"] != true {
		t.Fatalf("save: %d %v", code, out)
	}
	cfg, found, err := m.Config.Get("both")
	if err != nil || !found {
		t.Fatalf("not stored: %v found=%v", err, found)
	}
	if cfg.Label != "Canonical" {
		t.Fatalf("label = %q, want the canonical %q", cfg.Label, "Canonical")
	}
	if cfg.Auth == nil || cfg.Auth.Type != "oauth2" {
		t.Fatalf("canonical auth object should win: %+v", cfg.Auth)
	}
}

func TestSaveStillRejectsMissingLabel(t *testing.T) {
	// The aliases are a compatibility shim, not a bypass: a payload with
	// neither spelling still has to fail validation.
	m := newManager(t)
	app := fiber.New()
	routes.RegisterMCPRoutes(app, m)

	code, out := do(t, app, "POST", "/api/mcp/servers",
		`{"id":"nameless","transport":"http","url":"https://example.com/mcp"}`)
	if code != http.StatusBadRequest {
		t.Fatalf("want 400 for a missing label, got %d %v", code, out)
	}
}
