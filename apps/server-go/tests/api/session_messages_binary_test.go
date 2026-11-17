package tests

import (
	"encoding/json"
	"io"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
	"google.golang.org/protobuf/proto"

	consolev1 "github.com/Adelodunpeter25/console/apps/server-go/internal/gen/console/v1"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/db"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/routes"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/run"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/services"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/types"
	"github.com/Adelodunpeter25/console/apps/server-go/tests/helpers"
)

func TestSessionDetailBinary(t *testing.T) {
	manager, err := db.Open(db.OpenOptions{Path: ":memory:"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(manager.Close)

	sessions := services.NewSessionService(manager)
	header := helpers.CreateRunSession(t, sessions)
	if err := sessions.AppendMessages(header.ID, []types.AgentMessage{{
		ID: "m1", Role: "user", Data: json.RawMessage(`{"user":{"content":"Hello"}}`),
	}}); err != nil {
		t.Fatal(err)
	}
	app := fiber.New()
	routes.RegisterSessionRoutes(app, sessions, run.NewService(sessions))

	req := httptest.NewRequest("GET", "/api/sessions/"+header.ID, nil)
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
	var page consolev1.SessionMessagesPage
	if err := proto.Unmarshal(raw, &page); err != nil {
		t.Fatalf("decode protobuf: %v", err)
	}
	if page.Header == nil || page.Header.Id != header.ID {
		t.Fatalf("header = %v, want id %s", page.Header, header.ID)
	}
	if len(page.Messages) != 1 {
		t.Fatalf("got %d messages, want 1", len(page.Messages))
	}
	user := page.Messages[0].GetUser()
	if user == nil || user.Content != "Hello" {
		t.Fatalf("message = %v, want user content Hello", page.Messages[0])
	}
}
