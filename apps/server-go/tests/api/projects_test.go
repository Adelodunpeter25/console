// Projects migration coverage: proto-built payloads must match the shared
// golden fixtures, timestamps encode as protojson strings, and CRUD keeps
// its validation semantics.
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
	"google.golang.org/protobuf/encoding/protojson"

	consolev1 "github.com/Adelodunpeter25/console/apps/server-go/internal/gen/console/v1"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/db"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/routes"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/services"
)

func projectFixture(t *testing.T, name string) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "proto", "testdata", "projects", name))
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(string(raw))
}

func TestProjectProtoMatchesFixture(t *testing.T) {
	// Canonical encoding of a fixed message must equal the fixture: this is
	// the shape all three clients decode.
	raw, err := protojson.Marshal(&consolev1.ProjectInfo{
		Id: "p1", Name: "demo", Path: "/tmp/demo",
		CreatedAt: 1700000000000, UpdatedAt: 1700000000001,
	})
	if err != nil {
		t.Fatal(err)
	}
	if compactJSON(t, raw) != projectFixture(t, "project.json") {
		t.Fatalf("project bytes drifted:\n got %s\nwant %s", compactJSON(t, raw), projectFixture(t, "project.json"))
	}

	// Fixtures must stay parseable with unknown-field tolerance.
	var decoded consolev1.ProjectInfo
	unmarshal := protojson.UnmarshalOptions{DiscardUnknown: true}
	if err := unmarshal.Unmarshal([]byte(projectFixture(t, "project.json")), &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.GetCreatedAt() != 1700000000000 || decoded.GetName() != "demo" {
		t.Fatalf("decoded: %+v", &decoded)
	}
	var deleted consolev1.DeleteProjectResponse
	if err := unmarshal.Unmarshal([]byte(projectFixture(t, "delete.json")), &deleted); err != nil {
		t.Fatal(err)
	}
	if !deleted.GetDeleted() || deleted.GetId() != "p1" {
		t.Fatalf("decoded: %+v", &deleted)
	}
}

func TestProjectRoutesRoundTrip(t *testing.T) {
	manager, err := db.Open(db.OpenOptions{Path: ":memory:"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(manager.Close)
	svc := services.NewProjectService(manager)
	app := fiber.New()
	routes.RegisterProjectRoutes(app, svc)

	get := func() (int, map[string]any) {
		req := httptest.NewRequest("GET", "/api/projects", nil)
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
		var items []map[string]any
		if err := json.Unmarshal(envelope.Data, &items); err != nil {
			t.Fatalf("data: %s", envelope.Data)
		}
		if len(items) != 1 {
			t.Fatalf("want 1 project: %s", envelope.Data)
		}
		return resp.StatusCode, items[0]
	}

	// Create via the route: dir must exist.
	dir := t.TempDir()
	req := httptest.NewRequest("POST", "/api/projects", strings.NewReader(`{"path":"`+dir+`"}`))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req, 10000)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 200 {
		raw, _ := io.ReadAll(resp.Body)
		t.Fatalf("create: %d %s", resp.StatusCode, raw)
	}

	// Timestamps must be strings now (protojson int64), not numbers.
	_, item := get()
	created, ok := item["createdAt"].(string)
	if !ok || created == "" {
		t.Fatalf("createdAt must be a string: %v", item)
	}
	if _, ok := item["updatedAt"].(string); !ok {
		t.Fatalf("updatedAt must be a string: %v", item)
	}

	// Validation preserved.
	bad := httptest.NewRequest("POST", "/api/projects", strings.NewReader(`{}`))
	bad.Header.Set("Content-Type", "application/json")
	if resp, err := app.Test(bad, 10000); err != nil || resp.StatusCode != 400 {
		t.Fatalf("missing path must 400: %v", resp)
	}
	missing := httptest.NewRequest("POST", "/api/projects", strings.NewReader(`{"path":"/no/such/dir"}`))
	missing.Header.Set("Content-Type", "application/json")
	if resp, err := app.Test(missing, 10000); err != nil || resp.StatusCode != 400 {
		t.Fatalf("missing dir must 400: %v", resp)
	}

	// Delete round trip.
	if resp, err := app.Test(httptest.NewRequest("DELETE", "/api/projects/nope", nil), 10000); err != nil || resp.StatusCode != 404 {
		t.Fatalf("unknown delete must 404: %v", resp)
	}
	id, _ := item["id"].(string)
	delReq := httptest.NewRequest("DELETE", "/api/projects/"+id, nil)
	delResp, err := app.Test(delReq, 10000)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := io.ReadAll(delResp.Body)
	var envelope struct {
		Success bool            `json:"success"`
		Data    json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil || !envelope.Success {
		t.Fatalf("delete envelope: %s", raw)
	}
	var deleted map[string]any
	if err := json.Unmarshal(envelope.Data, &deleted); err != nil {
		t.Fatalf("delete data: %s", envelope.Data)
	}
	if deleted["id"] != id || deleted["deleted"] != true {
		t.Fatalf("delete data: %v", deleted)
	}
}
