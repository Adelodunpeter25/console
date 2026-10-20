package routes

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/loop"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/run"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/services"
)

func TestWireFrameBackgroundWorkUsesWorkRecord(t *testing.T) {
	work := services.BackgroundWork{
		ID:        "work_test",
		SessionID: "session-1",
		Kind:      services.BackgroundWorkShell,
		Status:    services.BackgroundWorkCompleted,
		Metadata:  map[string]string{"job_id": "job_test", "exit_code": "0"},
	}
	name, body, drop := run.WireFrame(loop.Event{
		Kind:    loop.EventBackgroundWork,
		Message: services.BackgroundWorkEvent{Type: services.BackgroundWorkCompletedEvent, Work: work},
	})
	if drop {
		t.Fatal("background work frame was dropped")
	}
	if name != "backgroundWorkCompleted" {
		t.Fatalf("event name = %q", name)
	}

	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	var envelope map[string]any
	if err := json.Unmarshal(raw, &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope["type"] != "backgroundWorkCompleted" {
		t.Fatalf("envelope type = %v", envelope["type"])
	}
	payload, ok := envelope["work"].(map[string]any)
	if !ok {
		t.Fatalf("work payload = %#v", envelope["work"])
	}
	if payload["id"] != "work_test" || payload["kind"] != "shell" || payload["status"] != "completed" {
		t.Fatalf("work payload = %#v", payload)
	}
	if _, ok := payload["Type"]; ok {
		t.Fatalf("nested background event leaked into wire payload: %#v", payload)
	}
}

func TestBackgroundWorkOutputRouteReadsDetachedShell(t *testing.T) {
	app := fiber.New()
	runs := run.NewService(nil)
	jobs := services.NewBashJobManager(nil)
	runs.SetBashJobs(jobs)
	RegisterRunRoutes(app, runs)

	snapshot, err := jobs.Start(services.StartBashJobOptions{
		Command:        "printf hello-route",
		OwnerSessionID: "session-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(jobs.KillAll)

	var work services.BackgroundWork
	items := runs.BackgroundWorkManager().List("session-1")
	if len(items) == 0 {
		t.Fatal("background work was not registered")
	}
	work = items[0]

	deadline := time.Now().Add(2 * time.Second)
	for {
		status, err := jobs.Status(snapshot.JobID, "session-1")
		if err == nil && status.Status != services.BashJobRunning {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("background shell did not finish")
		}
		time.Sleep(10 * time.Millisecond)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/sessions/session-1/background-work/"+work.ID+"/output", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d body = %s", resp.StatusCode, body)
	}
	if !strings.Contains(string(body), "hello-route") {
		t.Fatalf("body = %s", body)
	}
}
