// Settle/unsettle endpoints: 200 on success, 404 for unknown ids, 409 while
// a run is in flight, settled_at on the wire only for settled chats, and the
// unsettle-on-send hook through the real run path.
package tests

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/loop"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/routes"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/run"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/services"
	"github.com/Adelodunpeter25/console/apps/server-go/tests/helpers"
)

func settleRoutesApp(t *testing.T) (*fiber.App, *run.Service, *services.SessionService) {
	t.Helper()
	sessions := helpers.NewRunSessions(t)
	runs := run.NewService(sessions)
	app := fiber.New()
	routes.RegisterSessionRoutes(app, sessions, runs)
	return app, runs, sessions
}

func settleCall(t *testing.T, app *fiber.App, method, target string) (int, map[string]any, string) {
	t.Helper()
	req := httptest.NewRequest(method, target, nil)
	resp, err := app.Test(req, 10000)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := io.ReadAll(resp.Body)
	var envelope struct {
		Success bool            `json:"success"`
		Data    json.RawMessage `json:"data"`
		Error   string          `json:"error"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		t.Fatalf("%s %s: %s", method, target, raw)
	}
	var data map[string]any
	_ = json.Unmarshal(envelope.Data, &data)
	return resp.StatusCode, data, envelope.Error
}

func settledOnWire(t *testing.T, app *fiber.App, id string) any {
	t.Helper()
	code, body, _ := settleCall(t, app, http.MethodGet, "/api/sessions/"+id)
	if code != 200 {
		t.Fatalf("get header: %d %v", code, body)
	}
	// GET /:id nests the proto header under data.header.
	header, _ := body["header"].(map[string]any)
	if header == nil {
		t.Fatalf("no header on the wire: %v", body)
	}
	return header["settledAt"]
}

func TestSessionSettleRoutes(t *testing.T) {
	app, _, sessions := settleRoutesApp(t)
	header := helpers.CreateRunSession(t, sessions)
	id := header.ID

	// Unknown ids 404 on both endpoints.
	if code, _, _ := settleCall(t, app, "POST", "/api/sessions/ghost/settle"); code != 404 {
		t.Fatalf("settle ghost: %d", code)
	}
	if code, _, _ := settleCall(t, app, "POST", "/api/sessions/ghost/unsettle"); code != 404 {
		t.Fatalf("unsettle ghost: %d", code)
	}

	// Settle: the response matches the shared fixture shape, and settled_at
	// shows up on the wire.
	code, body, _ := settleCall(t, app, "POST", "/api/sessions/"+id+"/settle")
	if code != 200 || body["id"] != id || body["settled"] != true {
		t.Fatalf("settle: %d %v", code, body)
	}
	if settledOnWire(t, app, id) == nil {
		t.Fatal("settledAt missing from the wire header after settle")
	}

	// Unsettle: back to active, settled_at gone from the wire. protojson
	// omits default-valued fields, so an absent `settled` reads as false.
	code, body, _ = settleCall(t, app, "POST", "/api/sessions/"+id+"/unsettle")
	if code != 200 || body["id"] != id || body["settled"] == true {
		t.Fatalf("unsettle: %d %v", code, body)
	}
	if settledOnWire(t, app, id) != nil {
		t.Fatal("settledAt still on the wire after unsettle")
	}
}

func TestSessionSettleRefusesWhileRunActive(t *testing.T) {
	app, runs, sessions := settleRoutesApp(t)
	header := helpers.CreateRunSession(t, sessions)

	// Settle, then send: the send itself must unsettle (decision 3) through
	// the real StartRun path, before the run even starts.
	if code, _, _ := settleCall(t, app, "POST", "/api/sessions/"+header.ID+"/settle"); code != 200 {
		t.Fatalf("pre-send settle: %d", code)
	}
	release := make(chan struct{})
	runs.Lookup = func(id string) (loop.Provider, error) {
		return &helpers.MockProvider{Turns: []func() []loop.Event{
			func() []loop.Event { <-release; return []loop.Event{{Kind: loop.EventText, Text: "done"}} },
		}}, nil
	}
	hub, err := runs.StartRun(header.ID, run.Prompt{Text: "still needed", Provider: "mock", ModelID: "m", ApprovalMode: "full-access"})
	if err != nil {
		t.Fatalf("start run: %v", err)
	}
	if settledOnWire(t, app, header.ID) != nil {
		t.Fatal("sending did not unsettle the settled chat")
	}

	// Settling an in-flight chat is refused with the user-facing message.
	code, _, message := settleCall(t, app, "POST", "/api/sessions/"+header.ID+"/settle")
	if code != 409 {
		t.Fatalf("settle while active: %d", code)
	}
	if message != "Finish or stop the run before settling this chat." {
		t.Fatalf("409 message: %q", message)
	}

	// Once the run finishes, settling succeeds.
	close(release)
	helpers.WaitSettled(t, hub)
	deadline := time.Now().Add(5 * time.Second)
	for runs.IsActive(header.ID) && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if code, body, _ := settleCall(t, app, "POST", "/api/sessions/"+header.ID+"/settle"); code != 200 {
		t.Fatalf("settle after run: %d %v", code, body)
	}
}
