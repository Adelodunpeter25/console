// Settings migration coverage: proto-built responses must match the shared
// golden fixtures byte-for-byte, and PATCH keeps its null-clears semantics
// while tolerating unknown fields.
package tests

import (
	"encoding/json"
	"io"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/routes"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/services"
)

func settingsFixture(t *testing.T, name string) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "proto", "testdata", "settings", name))
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(string(raw))
}

func settingsApp(t *testing.T, settingsFile string) *fiber.App {
	t.Helper()
	t.Setenv("CONSOLE_SETTINGS_PATH", settingsFile)
	app := fiber.New()
	routes.RegisterSettingsRoutes(app, services.NewSettingsService())
	return app
}

func settingsData(t *testing.T, raw []byte) string {
	t.Helper()
	var envelope struct {
		Success bool            `json:"success"`
		Data    json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil || !envelope.Success {
		t.Fatalf("envelope: %s", raw)
	}
	return strings.TrimSpace(string(envelope.Data))
}

func TestSettingsProtoMatchesLegacyShape(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	if err := os.WriteFile(path, []byte(`{"modelRoles":{"vision":"openai/gpt-5","smol":"anthropic/claude-opus-4"}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	app := settingsApp(t, path)

	req := httptest.NewRequest("GET", "/api/settings", nil)
	resp, err := app.Test(req, 10000)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := io.ReadAll(resp.Body)
	if got := settingsData(t, raw); got != settingsFixture(t, "full.json") {
		t.Fatalf("full bytes drifted:\n got %s\nwant %s", got, settingsFixture(t, "full.json"))
	}

	// Missing file behaves like empty settings: {"modelRoles":{}}.
	app = settingsApp(t, filepath.Join(t.TempDir(), "missing.json"))
	req = httptest.NewRequest("GET", "/api/settings", nil)
	resp, err = app.Test(req, 10000)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ = io.ReadAll(resp.Body)
	if got := settingsData(t, raw); got != settingsFixture(t, "empty.json") {
		t.Fatalf("empty bytes drifted:\n got %s\nwant %s", got, settingsFixture(t, "empty.json"))
	}
}

func TestSettingsPatchNullClears(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	if err := os.WriteFile(path, []byte(`{"modelRoles":{"vision":"openai/gpt-5","smol":"anthropic/claude-opus-4"}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	app := settingsApp(t, path)

	patch := func(body string) string {
		req := httptest.NewRequest("PATCH", "/api/settings", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		resp, err := app.Test(req, 10000)
		if err != nil {
			t.Fatal(err)
		}
		if resp.StatusCode != 200 {
			raw, _ := io.ReadAll(resp.Body)
			t.Fatalf("status %d: %s", resp.StatusCode, raw)
		}
		raw, _ := io.ReadAll(resp.Body)
		return settingsData(t, raw)
	}

	// Null clears vision, smol survives untouched.
	if got := patch(`{"modelRoles":{"vision":null}}`); got != `{"modelRoles":{"smol":"anthropic/claude-opus-4"}}` {
		t.Fatalf("clear vision: %s", got)
	}
	// Missing modelRoles is still a 400.
	req := httptest.NewRequest("PATCH", "/api/settings", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	if resp, err := app.Test(req, 10000); err != nil || resp.StatusCode != 400 {
		t.Fatalf("missing modelRoles must 400: %v", resp)
	}
	// Unknown roles are still rejected.
	req = httptest.NewRequest("PATCH", "/api/settings", strings.NewReader(`{"modelRoles":{"nope":"x"}}`))
	req.Header.Set("Content-Type", "application/json")
	if resp, err := app.Test(req, 10000); err != nil || resp.StatusCode != 400 {
		t.Fatalf("unknown role must 400: %v", resp)
	}
}
