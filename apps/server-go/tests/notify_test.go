// Run notification coverage: attention banners for approvals/questions,
// done banners with excerpts, and silence on abort.
package tests

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/loop"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/tools"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/run"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/services"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/types"
)

func collectNotifications(t *testing.T, bus *services.NotificationService, stop <-chan struct{}) chan types.NotificationEvent {
	t.Helper()
	out := make(chan types.NotificationEvent, 16)
	ch := bus.Subscribe()
	go func() {
		defer bus.Unsubscribe(ch)
		for {
			select {
			case ev := <-ch:
				select {
				case out <- ev:
				default:
				}
			case <-stop:
				return
			}
		}
	}()
	return out
}

func waitNotification(t *testing.T, ch chan types.NotificationEvent, kind string) types.NotificationEvent {
	t.Helper()
	deadline := time.After(10 * time.Second)
	for {
		select {
		case ev := <-ch:
			if ev.Kind == kind {
				return ev
			}
		case <-deadline:
			t.Fatalf("timed out waiting for %s notification", kind)
		}
	}
}

func TestNotifyApprovalAndDone(t *testing.T) {
	sessions := newRunSessions(t)
	svc := run.NewService(sessions)
	bus := services.NewNotificationService()
	svc.SetNotifications(bus)
	stop := make(chan struct{})
	defer close(stop)
	notes := collectNotifications(t, bus, stop)

	args, _ := json.Marshal(map[string]any{"path": "/tmp/notify-test.txt", "content": "hi"})
	svc.Lookup = func(id string) (loop.Provider, error) {
		return &mockProvider{turns: []func() []loop.Event{
			func() []loop.Event {
				return []loop.Event{{Kind: loop.EventToolCall, Call: &tools.ToolCall{ID: "c1", Name: "write_file", Arguments: args}}}
			},
			func() []loop.Event { return []loop.Event{{Kind: loop.EventText, Text: "all done here"}} },
		}}, nil
	}
	header := createRunSession(t, sessions)
	hub, err := svc.StartRun(header.ID, run.Prompt{Text: "write", Provider: "mock", ModelID: "m"})
	if err != nil {
		t.Fatal(err)
	}
	req := waitHubFrame(t, hub, loop.EventPermissionRequest, 10*time.Second)
	attention := waitNotification(t, notes, "needs_attention")
	if !strings.Contains(attention.Body, "write_file") {
		t.Fatalf("attention body: %q", attention.Body)
	}
	if !svc.ApprovePermission(header.ID, req.Permission.RequestID, true) {
		t.Fatal("approve must resolve")
	}
	waitSettled(t, hub)
	done := waitNotification(t, notes, "done")
	if done.Title != "Done" || !contains(done.Body, "all done here") {
		t.Fatalf("done: %+v", done)
	}
}

func TestNotifySilentOnAbort(t *testing.T) {
	sessions := newRunSessions(t)
	svc := run.NewService(sessions)
	bus := services.NewNotificationService()
	svc.SetNotifications(bus)
	stop := make(chan struct{})
	defer close(stop)
	notes := collectNotifications(t, bus, stop)

	release := make(chan struct{})
	svc.Lookup = func(id string) (loop.Provider, error) {
		return &blockingProvider{release: release, released: new(bool)}, nil
	}
	header := createRunSession(t, sessions)
	hub, err := svc.StartRun(header.ID, run.Prompt{Text: "slow", Provider: "mock", ModelID: "m"})
	if err != nil {
		t.Fatal(err)
	}
	if !svc.Abort(header.ID) {
		t.Fatal("abort must succeed")
	}
	close(release)
	waitSettled(t, hub)
	select {
	case ev := <-notes:
		if ev.Kind == "done" {
			t.Fatalf("aborted run must not send done: %+v", ev)
		}
	case <-time.After(500 * time.Millisecond):
	}
}
