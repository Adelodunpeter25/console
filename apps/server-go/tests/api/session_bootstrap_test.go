// The session bootstrap route bundles the chat header, first message page,
// and every side panel into one response. This test pins the envelope shape
// the desktop client relies on when it opens a chat.
package tests

import (
	"github.com/MoeForge/moego/json"
	"io"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
	"google.golang.org/protobuf/proto"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/db"
	consolev1 "github.com/Adelodunpeter25/console/apps/server-go/internal/gen/console/v1"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/routes"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/run"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/services"
	"github.com/Adelodunpeter25/console/apps/server-go/tests/helpers"
)

func TestSessionBootstrapRoute(t *testing.T) {
	manager, err := db.Open(db.OpenOptions{Path: ":memory:"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(manager.Close)

	sessions := services.NewSessionService(manager)
	header := helpers.CreateRunSession(t, sessions)
	runs := run.NewService(sessions)
	app := fiber.New()
	routes.RegisterBootstrapRoutes(app, sessions, runs, services.NewDraftService(manager))

	req := httptest.NewRequest("GET", "/api/sessions/"+header.ID+"/bootstrap", nil)
	req.Header.Set("Accept", "application/json")
	resp, err := app.Test(req, 10000)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("status %d: %s", resp.StatusCode, raw)
	}

	var envelope struct {
		Success bool                       `json:"success"`
		Data    map[string]json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		t.Fatalf("decode envelope: %v (%s)", err, raw)
	}
	if !envelope.Success {
		t.Fatalf("success false: %s", raw)
	}
	for _, key := range []string{
		"header", "turns", "hasMore", "nextCursor", "changes",
		"todos", "suggestions", "queuedPrompts", "backgroundWork", "draft",
	} {
		if _, ok := envelope.Data[key]; !ok {
			t.Fatalf("missing %q: %s", key, raw)
		}
	}
	for _, key := range []string{
		"turns", "changes", "todos", "suggestions", "queuedPrompts", "backgroundWork",
	} {
		var items []json.RawMessage
		if err := json.Unmarshal(envelope.Data[key], &items); err != nil {
			t.Fatalf("%s is not an array: %s (%v)", key, envelope.Data[key], err)
		}
	}
	if string(envelope.Data["draft"]) != "null" {
		t.Fatalf("draft = %s, want null", envelope.Data["draft"])
	}
}

func TestSessionBootstrapBinary(t *testing.T) {
	manager, err := db.Open(db.OpenOptions{Path: ":memory:"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(manager.Close)

	sessions := services.NewSessionService(manager)
	header := helpers.CreateRunSession(t, sessions)
	runs := run.NewService(sessions)
	app := fiber.New()
	routes.RegisterBootstrapRoutes(app, sessions, runs, services.NewDraftService(manager))

	req := httptest.NewRequest("GET", "/api/sessions/"+header.ID+"/bootstrap", nil)
	req.Header.Set("Accept", "application/x-protobuf")
	resp, err := app.Test(req, 10000)
	if err != nil {
		t.Fatal(err)
	}
	if got := resp.Header.Get("Content-Type"); got != "application/x-protobuf" {
		t.Fatalf("content type = %q, want application/x-protobuf", got)
	}
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	var body consolev1.SessionBootstrapResponse
	if err := proto.Unmarshal(raw, &body); err != nil {
		t.Fatalf("decode protobuf: %v", err)
	}
	if body.Header == nil || body.Header.Id != header.ID {
		t.Fatalf("header = %v, want id %s", body.Header, header.ID)
	}
	if len(body.Turns) != 0 || len(body.Todos) != 0 || body.Draft != nil {
		t.Fatalf("empty session returned unexpected payload: %v", &body)
	}
}
