// Integration status route coverage: gh detection via a fake binary on
// PATH (installed + version), the missing-binary shape, and a present but
// unparseable `gh --version` degrading to installed-without-version.
package tests

import (
	"encoding/json"
	"io"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/gofiber/fiber/v2"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/routes"
)

// newIntegrationsApp builds a Fiber app with only the integration routes.
func newIntegrationsApp() *fiber.App {
	app := fiber.New()
	routes.RegisterIntegrationRoutes(app)
	return app
}

// getIntegrations GETs the endpoint and decodes the body.
func getIntegrations(t *testing.T, app *fiber.App) (int, map[string]any) {
	t.Helper()
	req := httptest.NewRequest("GET", "/api/integrations", nil)
	resp, err := app.Test(req, 10000)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := io.ReadAll(resp.Body)
	var out map[string]any
	_ = json.Unmarshal(raw, &out)
	return resp.StatusCode, out
}

// fakeGh writes an executable `gh` shim printing script on stdout and
// prepends its dir to PATH for the test.
func fakeGh(t *testing.T, script string) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "gh")
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+script+"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+":"+os.Getenv("PATH"))
}

func ghRow(t *testing.T, out map[string]any) map[string]any {
	t.Helper()
	data, ok := out["data"].(map[string]any)
	if !ok {
		t.Fatalf("missing data envelope: %v", out)
	}
	rows, ok := data["integrations"].([]any)
	if !ok || len(rows) != 1 {
		t.Fatalf("expected exactly the gh row: %v", data)
	}
	row, ok := rows[0].(map[string]any)
	if !ok || row["id"] != "gh" {
		t.Fatalf("expected the gh row: %v", rows)
	}
	return row
}

func TestIntegrationsGhInstalled(t *testing.T) {
	fakeGh(t, `echo "gh version 2.75.0 (2025-11-19)"`)
	code, out := getIntegrations(t, newIntegrationsApp())
	if code != 200 || out["success"] != true {
		t.Fatalf("status=%d body=%v", code, out)
	}
	row := ghRow(t, out)
	if row["installed"] != true {
		t.Fatalf("installed must be true: %v", row)
	}
	if row["version"] != "2.75.0" {
		t.Fatalf("version must parse: %v", row)
	}
	if _, ok := row["binaryPath"].(string); !ok {
		t.Fatalf("binaryPath must be set when found: %v", row)
	}
	if row["installUrl"] == "" || row["installHint"] == "" {
		t.Fatalf("install pointer must always be present: %v", row)
	}
}

func TestIntegrationsGhMissing(t *testing.T) {
	t.Setenv("PATH", t.TempDir()) // empty dir: no gh anywhere
	code, out := getIntegrations(t, newIntegrationsApp())
	if code != 200 || out["success"] != true {
		t.Fatalf("status=%d body=%v", code, out)
	}
	row := ghRow(t, out)
	if row["installed"] != false {
		t.Fatalf("installed must be false: %v", row)
	}
	// protojson drops unset optionals — a missing binary carries neither.
	if _, ok := row["version"]; ok {
		t.Fatalf("version must be absent when missing: %v", row)
	}
	if _, ok := row["binaryPath"]; ok {
		t.Fatalf("binaryPath must be absent when missing: %v", row)
	}
	if row["installUrl"] == "" || row["installHint"] == "" {
		t.Fatalf("install pointer must be present especially when missing: %v", row)
	}
}

func TestIntegrationsGhUnparseableVersion(t *testing.T) {
	fakeGh(t, `echo "totally unexpected output"`)
	_, out := getIntegrations(t, newIntegrationsApp())
	row := ghRow(t, out)
	if row["installed"] != true {
		t.Fatalf("a resolving binary is installed even without a version: %v", row)
	}
	if _, ok := row["version"]; ok {
		t.Fatalf("unparseable version must stay absent: %v", row)
	}
}

func TestIntegrationsGhVersionExitNonZero(t *testing.T) {
	fakeGh(t, `echo "boom" >&2; exit 1`)
	_, out := getIntegrations(t, newIntegrationsApp())
	row := ghRow(t, out)
	if row["installed"] != true {
		t.Fatalf("installed must survive a failing version probe: %v", row)
	}
	if _, ok := row["version"]; ok {
		t.Fatalf("failed probe must leave version absent: %v", row)
	}
}
