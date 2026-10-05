// Model favorites migration coverage: proto-built payloads must match the
// shared golden fixtures byte-for-byte, and the routes must keep their
// validation semantics while tolerating unknown fields.
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

	"github.com/Adelodunpeter25/console/apps/server-go/internal/db"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/routes"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/services"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/types"
)

func readFixture(t *testing.T, name string) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "proto", "testdata", "favorites", name))
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(string(raw))
}

func TestFavoritesProtoMatchesLegacyShape(t *testing.T) {
	// Single seed: List() orders by created_at millis, so two seeds in the
	// same millisecond would make array order nondeterministic.
	list := []types.ModelFavorite{
		{Provider: "anthropic", ModelID: "claude-opus-4"},
	}
	// Exercise the same conversion the GET handler uses.
	app := fiber.New()
	manager, err := db.Open(db.OpenOptions{Path: ":memory:"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(manager.Close)
	svc := services.NewFavoriteService(manager)
	for _, f := range list {
		if err := svc.Set(f, true); err != nil {
			t.Fatal(err)
		}
	}
	routes.RegisterFavoriteRoutes(app, svc)

	req := httptest.NewRequest("GET", "/api/model-favorites", nil)
	resp, err := app.Test(req, 10000)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := io.ReadAll(resp.Body)
	var envelope struct {
		Success bool            `json:"success"`
		Data    json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil || !envelope.Success {
		t.Fatalf("envelope: %s", raw)
	}
	if strings.TrimSpace(string(envelope.Data)) != readFixture(t, "list.json") {
		t.Fatalf("list bytes drifted:\n got %s\nwant %s", envelope.Data, readFixture(t, "list.json"))
	}

	// PUT a favorite off then on; the on-response must match its fixture.
	off := httptest.NewRequest("PUT", "/api/model-favorites",
		strings.NewReader(`{"provider":"anthropic","modelId":"claude-opus-4","favorite":false}`))
	off.Header.Set("Content-Type", "application/json")
	if resp, err := app.Test(off, 10000); err != nil || resp.StatusCode != 200 {
		t.Fatalf("unfavorite: %v", resp)
	}
	on := httptest.NewRequest("PUT", "/api/model-favorites",
		strings.NewReader(`{"provider":"anthropic","modelId":"claude-opus-4","favorite":true}`))
	on.Header.Set("Content-Type", "application/json")
	resp, err = app.Test(on, 10000)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ = io.ReadAll(resp.Body)
	envelope = struct {
		Success bool            `json:"success"`
		Data    json.RawMessage `json:"data"`
	}{}
	if err := json.Unmarshal(raw, &envelope); err != nil || !envelope.Success {
		t.Fatalf("envelope: %s", raw)
	}
	if strings.TrimSpace(string(envelope.Data)) != readFixture(t, "set_response.json") {
		t.Fatalf("set bytes drifted:\n got %s\nwant %s", envelope.Data, readFixture(t, "set_response.json"))
	}
}

func TestFavoritesValidationAndTolerance(t *testing.T) {
	app := fiber.New()
	manager, err := db.Open(db.OpenOptions{Path: ":memory:"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(manager.Close)
	routes.RegisterFavoriteRoutes(app, services.NewFavoriteService(manager))

	put := func(body string) int {
		req := httptest.NewRequest("PUT", "/api/model-favorites", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		resp, err := app.Test(req, 10000)
		if err != nil {
			t.Fatal(err)
		}
		return resp.StatusCode
	}

	if code := put(`{"provider":"a","modelId":"b"}`); code != 400 {
		t.Fatalf("missing favorite must 400, got %d", code)
	}
	if code := put(`{"provider":"","modelId":"b","favorite":true}`); code != 400 {
		t.Fatalf("empty provider must 400, got %d", code)
	}
	if code := put(`{"provider":"a","modelId":"b","favorite":true,"futureField":"x"}`); code != 200 {
		t.Fatalf("unknown fields must be tolerated, got %d", code)
	}
}
