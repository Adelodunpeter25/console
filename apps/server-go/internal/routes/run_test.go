package routes

import (
	"encoding/json"
	"testing"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/loop"
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
	name, body, drop := wireFrame(loop.Event{
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
