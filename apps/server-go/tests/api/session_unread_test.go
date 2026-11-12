// Unread state: a finished run or an errored run makes a chat unread, /read
// clears it, /unread brings it back, settle-on-read leaves an unread
// needs_attention alone, and none of it reorders the sidebar.
package tests

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/loop"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/stream"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/run"
	"github.com/Adelodunpeter25/console/apps/server-go/tests/helpers"
)

// failingProvider ends every turn in an error, like a provider outage.
type failingProvider struct{}

func (failingProvider) RunTurn(_ context.Context, _ loop.TurnRequest, s *stream.Stream[loop.Event]) error {
	err := errors.New("provider down")
	// A provider closes its stream as well as returning: the turn drains the
	// stream, so a bare return would leave it waiting for events forever.
	s.Fail(err)
	return err
}

type wireHeader struct {
	Status    string `json:"status"`
	Unread    bool   `json:"unread"`
	UpdatedAt string `json:"updatedAt"`
}

func getWireHeader(t *testing.T, app *fiber.App, id string) wireHeader {
	t.Helper()
	code, body, _ := settleCall(t, app, http.MethodGet, "/api/sessions/"+id)
	if code != 200 {
		t.Fatalf("get %s: %d %v", id, code, body)
	}
	raw, _ := json.Marshal(body["header"])
	var h wireHeader
	if err := json.Unmarshal(raw, &h); err != nil {
		t.Fatal(err)
	}
	return h
}

func listWireHeaders(t *testing.T, app *fiber.App) map[string]wireHeader {
	t.Helper()
	resp, err := app.Test(httptest.NewRequest(http.MethodGet, "/api/sessions", nil), 10000)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := io.ReadAll(resp.Body)
	var envelope struct {
		Data []struct {
			ID string `json:"id"`
			wireHeader
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		t.Fatalf("list: %s", raw)
	}
	out := map[string]wireHeader{}
	for _, h := range envelope.Data {
		out[h.ID] = h.wireHeader
	}
	return out
}

func waitIdle(t *testing.T, active func(string) bool, id string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for active(id) && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if active(id) {
		t.Fatal("run never released the session")
	}
}

func runPrompt() run.Prompt {
	return run.Prompt{Text: "go", Provider: "mock", ModelID: "m", ApprovalMode: "full-access"}
}

func TestFreshChatIsNotUnread(t *testing.T) {
	app, _, sessions := settleRoutesApp(t)
	header := helpers.CreateRunSession(t, sessions)
	if getWireHeader(t, app, header.ID).Unread {
		t.Fatal("a chat that never ran must not be unread")
	}
	if listWireHeaders(t, app)[header.ID].Unread {
		t.Fatal("a chat that never ran must not be unread in the list either")
	}
}

func TestFinishedRunIsUnreadUntilRead(t *testing.T) {
	app, runs, sessions := settleRoutesApp(t)
	runs.Lookup = func(string) (loop.Provider, error) {
		return &helpers.MockProvider{Turns: []func() []loop.Event{
			func() []loop.Event { return []loop.Event{{Kind: loop.EventText, Text: "done"}} },
		}}, nil
	}
	header := helpers.CreateRunSession(t, sessions)
	hub, err := runs.StartRun(header.ID, runPrompt())
	if err != nil {
		t.Fatal(err)
	}
	helpers.WaitSettled(t, hub)
	waitIdle(t, runs.IsActive, header.ID)

	before := getWireHeader(t, app, header.ID)
	if before.Status != "done" || !before.Unread {
		t.Fatalf("finished run: status=%q unread=%v, want done/true", before.Status, before.Unread)
	}

	code, body, _ := settleCall(t, app, "POST", "/api/sessions/"+header.ID+"/read")
	if code != 200 {
		t.Fatalf("read: %d %v", code, body)
	}
	after := getWireHeader(t, app, header.ID)
	if after.Unread {
		t.Fatal("still unread after /read")
	}
	if after.UpdatedAt != before.UpdatedAt {
		t.Fatalf("/read reordered the sidebar: updatedAt %s -> %s", before.UpdatedAt, after.UpdatedAt)
	}
	// Reading twice is harmless.
	if code, _, _ := settleCall(t, app, "POST", "/api/sessions/"+header.ID+"/read"); code != 200 {
		t.Fatalf("second read: %d", code)
	}

	// Mark as unread brings the dot back without touching updated_at.
	if code, _, _ := settleCall(t, app, "POST", "/api/sessions/"+header.ID+"/unread"); code != 200 {
		t.Fatalf("unread: %d", code)
	}
	again := getWireHeader(t, app, header.ID)
	if !again.Unread {
		t.Fatal("not unread after /unread")
	}
	if again.UpdatedAt != before.UpdatedAt {
		t.Fatalf("/unread reordered the sidebar: updatedAt %s -> %s", before.UpdatedAt, again.UpdatedAt)
	}
}

func TestMarkUnreadRefusals(t *testing.T) {
	app, runs, sessions := settleRoutesApp(t)
	if code, _, _ := settleCall(t, app, "POST", "/api/sessions/ghost/read"); code != 404 {
		t.Fatalf("read ghost: %d", code)
	}
	if code, _, _ := settleCall(t, app, "POST", "/api/sessions/ghost/unread"); code != 404 {
		t.Fatalf("unread ghost: %d", code)
	}

	// A settled chat is filed away, so it cannot be unread.
	settled := helpers.CreateRunSession(t, sessions)
	if code, _, _ := settleCall(t, app, "POST", "/api/sessions/"+settled.ID+"/settle"); code != 200 {
		t.Fatalf("settle: %d", code)
	}
	if code, _, _ := settleCall(t, app, "POST", "/api/sessions/"+settled.ID+"/unread"); code != 409 {
		t.Fatalf("unread on settled: %d, want 409", code)
	}

	// A running chat shows its spinner instead.
	release := make(chan struct{})
	runs.Lookup = func(string) (loop.Provider, error) {
		return &helpers.MockProvider{Turns: []func() []loop.Event{
			func() []loop.Event { <-release; return []loop.Event{{Kind: loop.EventText, Text: "x"}} },
		}}, nil
	}
	busy := helpers.CreateRunSession(t, sessions)
	hub, err := runs.StartRun(busy.ID, runPrompt())
	if err != nil {
		t.Fatal(err)
	}
	if code, _, _ := settleCall(t, app, "POST", "/api/sessions/"+busy.ID+"/unread"); code != 409 {
		t.Fatalf("unread while running: %d, want 409", code)
	}
	close(release)
	helpers.WaitSettled(t, hub)
}

func TestErroredRunKeepsNeedsAttentionUntilRead(t *testing.T) {
	app, runs, sessions := settleRoutesApp(t)
	runs.Lookup = func(string) (loop.Provider, error) { return failingProvider{}, nil }
	header := helpers.CreateRunSession(t, sessions)
	hub, err := runs.StartRun(header.ID, runPrompt())
	if err != nil {
		t.Fatal(err)
	}
	helpers.WaitSettled(t, hub)
	waitIdle(t, runs.IsActive, header.ID)

	// Reading the chat or the list must not wipe the alert: that is the
	// settle-on-read bug this guards against.
	for i := 0; i < 2; i++ {
		if h := getWireHeader(t, app, header.ID); h.Status != "needs_attention" || !h.Unread {
			t.Fatalf("detail read %d: status=%q unread=%v, want needs_attention/true", i, h.Status, h.Unread)
		}
		if h := listWireHeaders(t, app)[header.ID]; h.Status != "needs_attention" || !h.Unread {
			t.Fatalf("list read %d: status=%q unread=%v, want needs_attention/true", i, h.Status, h.Unread)
		}
	}

	// Opening the chat clears it.
	if code, _, _ := settleCall(t, app, "POST", "/api/sessions/"+header.ID+"/read"); code != 200 {
		t.Fatalf("read: %d", code)
	}
	if h := getWireHeader(t, app, header.ID); h.Status != "done" || h.Unread {
		t.Fatalf("after read: status=%q unread=%v, want done/false", h.Status, h.Unread)
	}
}

func TestStaleWorkingStillSettlesDone(t *testing.T) {
	app, _, sessions := settleRoutesApp(t)
	header := helpers.CreateRunSession(t, sessions)
	// A crash mid-run leaves "working" with no run behind it.
	if err := sessions.UpdateStatusOnly(header.ID, "working"); err != nil {
		t.Fatal(err)
	}
	if h := getWireHeader(t, app, header.ID); h.Status != "done" {
		t.Fatalf("stale working: status=%q, want done", h.Status)
	}
}

func TestSettledChatIsNeverUnread(t *testing.T) {
	app, runs, sessions := settleRoutesApp(t)
	runs.Lookup = func(string) (loop.Provider, error) {
		return &helpers.MockProvider{Turns: []func() []loop.Event{
			func() []loop.Event { return []loop.Event{{Kind: loop.EventText, Text: "done"}} },
		}}, nil
	}
	header := helpers.CreateRunSession(t, sessions)
	hub, err := runs.StartRun(header.ID, runPrompt())
	if err != nil {
		t.Fatal(err)
	}
	helpers.WaitSettled(t, hub)
	waitIdle(t, runs.IsActive, header.ID)
	if !getWireHeader(t, app, header.ID).Unread {
		t.Fatal("precondition: finished chat should be unread")
	}
	if code, _, _ := settleCall(t, app, "POST", "/api/sessions/"+header.ID+"/settle"); code != 200 {
		t.Fatalf("settle: %d", code)
	}
	if getWireHeader(t, app, header.ID).Unread {
		t.Fatal("a settled chat must never be unread")
	}
}
