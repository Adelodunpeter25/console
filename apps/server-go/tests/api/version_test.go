// GET /api/version golden-JSON coverage: the response must match the
// shared protojson fixture so Go, Rust, and Kotlin decode identically.
package tests

import (
	"encoding/json"
	"io"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/gofiber/fiber/v2"
	"google.golang.org/protobuf/encoding/protojson"

	consolev1 "github.com/Adelodunpeter25/console/apps/server-go/internal/gen/console/v1"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/routes"
)

func TestVersionEndpointMatchesFixture(t *testing.T) {
	app := fiber.New()
	routes.RegisterVersionRoutes(app)

	req := httptest.NewRequest("GET", "/api/version", nil)
	resp, err := app.Test(req, 10000)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("status: %d", resp.StatusCode)
	}
	raw, _ := io.ReadAll(resp.Body)

	var got consolev1.GetApiVersionResponse
	unmarshal := protojson.UnmarshalOptions{DiscardUnknown: true}
	if err := unmarshal.Unmarshal(raw, &got); err != nil {
		t.Fatalf("decode: %v body=%s", err, raw)
	}
	if got.ApiVersion != routes.ApiVersion {
		t.Fatalf("api_version: %d", got.ApiVersion)
	}

	fixture, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "proto", "testdata", "common", "version.json"))
	if err != nil {
		t.Fatal(err)
	}
	var want map[string]any
	if err := json.Unmarshal(fixture, &want); err != nil {
		t.Fatal(err)
	}
	var actual map[string]any
	if err := json.Unmarshal(raw, &actual); err != nil {
		t.Fatal(err)
	}
	if len(actual) != len(want) {
		t.Fatalf("shape drift: got %v want %v", actual, want)
	}
	for k, v := range want {
		if actual[k] != v {
			t.Fatalf("field %s: got %v want %v", k, actual[k], v)
		}
	}
}
