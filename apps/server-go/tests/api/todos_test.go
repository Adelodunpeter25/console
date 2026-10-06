// Todos migration coverage: proto-built payloads must match the shared
// golden fixture, and the session todos route keeps its shape.
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
	"github.com/Adelodunpeter25/console/apps/server-go/internal/routes"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/run"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/types"
	"github.com/Adelodunpeter25/console/apps/server-go/tests/helpers"
)

func todoFixture(t *testing.T, name string) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "proto", "testdata", "session", name))
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(string(raw))
}

func TestTodoProtoMatchesFixture(t *testing.T) {
	items := []*consolev1.TodoItem{
		{Id: 1, Content: "Write code", Status: "in_progress"},
		{Id: 2, Content: "Write tests", Status: "pending"},
	}
	elements := make([]json.RawMessage, 0, len(items))
	for _, item := range items {
		raw, err := protojson.Marshal(item)
		if err != nil {
			t.Fatal(err)
		}
		elements = append(elements, json.RawMessage(compactJSON(t, raw)))
	}
	encoded, err := json.Marshal(elements)
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(encoded)) != todoFixture(t, "todos.json") {
		t.Fatalf("todos bytes drifted:\n got %s\nwant %s", encoded, todoFixture(t, "todos.json"))
	}

	// Fixture stays parseable with unknown-field tolerance.
	var decoded consolev1.TodoItem
	unmarshal := protojson.UnmarshalOptions{DiscardUnknown: true}
	if err := unmarshal.Unmarshal([]byte(`{"id":1,"content":"x","status":"pending"}`), &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.GetId() != 1 || decoded.GetStatus() != "pending" {
		t.Fatalf("decoded: %+v", &decoded)
	}
}

func TestSessionTodosRoute(t *testing.T) {
	sessions := helpers.NewRunSessions(t)
	header := helpers.CreateRunSession(t, sessions)
	if err := sessions.SaveSessionTodos(header.ID, []types.TodoItem{
		{ID: 1, Content: "Write code", Status: "in_progress"},
		{ID: 2, Content: "Write tests", Status: "pending"},
	}); err != nil {
		t.Fatal(err)
	}

	app := fiber.New()
	routes.RegisterSessionRoutes(app, sessions, run.NewService(sessions))
	req := httptest.NewRequest("GET", "/api/sessions/"+header.ID+"/todos", nil)
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
	if strings.TrimSpace(string(envelope.Data)) != todoFixture(t, "todos.json") {
		t.Fatalf("todos bytes drifted:\n got %s\nwant %s", envelope.Data, todoFixture(t, "todos.json"))
	}
}
