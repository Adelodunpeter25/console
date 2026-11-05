package tests

import (
	"io"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/routes"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/run"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/types"
	"github.com/Adelodunpeter25/console/apps/server-go/tests/helpers"
)

func TestSessionPromptsRoute(t *testing.T) {
	sessions := helpers.NewRunSessions(t)
	header := helpers.CreateRunSession(t, sessions)
	app := fiber.New()
	routes.RegisterSessionRoutes(app, sessions, run.NewService(sessions))

	get := func(id string) (int, string) {
		resp, err := app.Test(httptest.NewRequest("GET", "/api/sessions/"+id+"/prompts", nil), 10000)
		if err != nil {
			t.Fatal(err)
		}
		raw, _ := io.ReadAll(resp.Body)
		return resp.StatusCode, string(raw)
	}

	// No prompts yet: an empty array, never null.
	if code, body := get(header.ID); code != 200 || !strings.Contains(body, `"data":[]`) {
		t.Fatalf("empty chat: %d %s", code, body)
	}

	if err := sessions.AppendMessages(header.ID, []types.AgentMessage{
		{ID: "u1", Role: "user", Data: []byte(`{"user":{"content":"hello there"}}`)},
		{ID: "a1", Role: "assistant", Data: []byte(`{"assistant":{"id":"a1","content":[{"text":{"text":"hi"}}],"stopReason":"stop"}}`)},
	}); err != nil {
		t.Fatal(err)
	}
	code, body := get(header.ID)
	if code != 200 || !strings.Contains(body, `"id":"u1"`) || !strings.Contains(body, `"preview":"hello there"`) || strings.Contains(body, "a1") {
		t.Fatalf("prompts: %d %s", code, body)
	}

	if code, _ := get("no-such-session"); code != 404 {
		t.Fatalf("unknown session = %d, want 404", code)
	}
}
