// Idle-session wake: finished background work continues the agent on its own
// with a hidden synthetic user turn, and announces it with a wire event.
package tests

import (
	"strings"
	"testing"
	"time"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/loop"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/run"
	backgroundsvc "github.com/Adelodunpeter25/console/apps/server-go/internal/services/background"
	"github.com/Adelodunpeter25/console/apps/server-go/tests/helpers"
)

func TestBackgroundCompletionWakesIdleSession(t *testing.T) {
	sessions := helpers.NewRunSessions(t)
	svc := run.NewService(sessions)
	work := backgroundsvc.NewManager()
	svc.SetBackgroundWorkManager(work)
	svc.Lookup = func(string) (loop.Provider, error) { return queueMock("ok, waiting", "build looks good"), nil }
	header := helpers.CreateRunSession(t, sessions)

	// A job that outlives the first run: it is registered while the run is
	// active and completes only after the run has settled.
	rec, err := work.Register(backgroundsvc.Options{
		SessionID: header.ID, Kind: backgroundsvc.KindShell,
		Metadata: map[string]string{"command": "make build", "exit_code": "0"},
	})
	if err != nil {
		t.Fatal(err)
	}
	hub, err := svc.StartRun(header.ID, run.Prompt{Text: "build it", Provider: "mock", ModelID: "m"})
	if err != nil {
		t.Fatal(err)
	}
	helpers.WaitSettled(t, hub)
	if svc.IsActive(header.ID) {
		deadline := time.Now().Add(time.Second)
		for svc.IsActive(header.ID) && time.Now().Before(deadline) {
			time.Sleep(10 * time.Millisecond)
		}
	}

	if _, err := work.Complete(rec.ID, header.ID, backgroundsvc.StatusCompleted, "done", "", nil); err != nil {
		t.Fatal(err)
	}

	deadline := time.Now().Add(3 * time.Second)
	for {
		loaded, err := sessions.Load(header.ID, 0, 0)
		if err != nil {
			t.Fatal(err)
		}
		if len(loaded.Messages) >= 4 && !svc.IsActive(header.ID) {
			wake, err := loop.MessageFromProtoBytes(loaded.Messages[2])
			if err != nil {
				t.Fatal(err)
			}
			user, ok := wake.(loop.UserMessage)
			if !ok || !user.Synthetic || !strings.Contains(user.Content, "make build") {
				t.Fatalf("wake message: %#v", wake)
			}
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("session never woke; messages=%d", len(loaded.Messages))
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TestCancelledBackgroundWorkDoesNotWake(t *testing.T) {
	sessions := helpers.NewRunSessions(t)
	svc := run.NewService(sessions)
	work := backgroundsvc.NewManager()
	svc.SetBackgroundWorkManager(work)
	svc.Lookup = func(string) (loop.Provider, error) { return queueMock("unexpected"), nil }
	header := helpers.CreateRunSession(t, sessions)

	rec, _ := work.Register(backgroundsvc.Options{SessionID: header.ID, Kind: backgroundsvc.KindShell})
	if _, err := work.Cancel(rec.ID, header.ID); err != nil {
		t.Fatal(err)
	}
	time.Sleep(200 * time.Millisecond)
	if svc.IsActive(header.ID) {
		t.Fatal("cancelled work must not wake the session")
	}
	if loaded, _ := sessions.Load(header.ID, 0, 0); len(loaded.Messages) != 0 {
		t.Fatalf("unexpected messages: %d", len(loaded.Messages))
	}
}
