// File comments: durable per-project comments on files and diffs.
package tests

import (
	"encoding/json"
	"io"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/db"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/routes"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/services"
)

func fileCommentCall(t *testing.T, app *fiber.App, method, url, body string) (int, json.RawMessage) {
	t.Helper()
	req := httptest.NewRequest(method, url, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req, 10000)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := io.ReadAll(resp.Body)
	var env struct {
		Data json.RawMessage `json:"data"`
	}
	_ = json.Unmarshal(raw, &env)
	return resp.StatusCode, env.Data
}

func TestFileCommentsCRUD(t *testing.T) {
	manager, err := db.Open(db.OpenOptions{Path: ":memory:"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(manager.Close)
	app := fiber.New()
	routes.RegisterFileCommentRoutes(app, services.NewFileCommentService(manager))

	status, data := fileCommentCall(t, app, "POST", "/api/projects/p1/comments",
		`{"path":"src/a.rs","side":"FILE_COMMENT_SIDE_OLD","startLine":3,"endLine":5,"text":"  rename  ","snippet":"fn a()"}`)
	if status != 200 {
		t.Fatalf("create: %d %s", status, data)
	}
	var created struct {
		ID, Text, Side string
	}
	_ = json.Unmarshal(data, &created)
	if created.ID == "" || created.Text != "rename" || created.Side != "FILE_COMMENT_SIDE_OLD" {
		t.Fatalf("created: %s", data)
	}

	if s, _ := fileCommentCall(t, app, "POST", "/api/projects/p1/comments",
		`{"path":"src/b.rs","startLine":1,"endLine":1,"text":"other"}`); s != 200 {
		t.Fatal("second create failed")
	}
	if s, _ := fileCommentCall(t, app, "POST", "/api/projects/p1/comments",
		`{"path":"src/b.rs","startLine":2,"endLine":1,"text":"bad range"}`); s != 400 {
		t.Fatalf("bad range accepted: %d", s)
	}

	_, data = fileCommentCall(t, app, "GET", "/api/projects/p1/comments", "")
	var all []map[string]any
	_ = json.Unmarshal(data, &all)
	if len(all) != 2 {
		t.Fatalf("list all: %s", data)
	}
	_, data = fileCommentCall(t, app, "GET", "/api/projects/p1/comments?path=src/a.rs", "")
	_ = json.Unmarshal(data, &all)
	if len(all) != 1 {
		t.Fatalf("list by path: %s", data)
	}
	_, data = fileCommentCall(t, app, "GET", "/api/projects/p2/comments", "")
	_ = json.Unmarshal(data, &all)
	if len(all) != 0 {
		t.Fatalf("other project leaked: %s", data)
	}

	_, data = fileCommentCall(t, app, "PATCH", "/api/comments/"+created.ID, `{"text":"renamed"}`)
	if !strings.Contains(string(data), `"renamed"`) {
		t.Fatalf("update: %s", data)
	}
	if s, _ := fileCommentCall(t, app, "DELETE", "/api/comments/"+created.ID, ""); s != 200 {
		t.Fatalf("delete: %d", s)
	}
	if s, _ := fileCommentCall(t, app, "DELETE", "/api/comments/"+created.ID, ""); s != 404 {
		t.Fatalf("second delete: %d", s)
	}
}
