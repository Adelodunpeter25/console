package tests

import (
	"sync"
	"testing"
	"time"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/services"
)

func TestBackgroundWorkLifecycle(t *testing.T) {
	manager := services.NewBackgroundWorkManager()
	var mu sync.Mutex
	var events []services.BackgroundWorkEvent
	manager.SetListener(func(event services.BackgroundWorkEvent) {
		mu.Lock()
		events = append(events, event)
		mu.Unlock()
	})

	work, err := manager.Register(services.BackgroundWorkOptions{
		SessionID: "session-1",
		Kind:      services.BackgroundWorkShell,
		Metadata:  map[string]string{"secret": "not in summary"},
	})
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	if work.Status != services.BackgroundWorkRunning || work.ID == "" {
		t.Fatalf("unexpected running work: %+v", work)
	}
	if _, err := manager.Get(work.ID, "session-2"); err == nil {
		t.Fatal("expected ownership error")
	}
	if _, err := manager.Complete(work.ID, "session-1", services.BackgroundWorkCompleted, "done", "", map[string]string{"large": string(make([]byte, 1000))}); err != nil {
		t.Fatalf("complete: %v", err)
	}
	final, err := manager.Get(work.ID, "session-1")
	if err != nil {
		t.Fatalf("get final: %v", err)
	}
	if final.Status != services.BackgroundWorkCompleted || final.FinishedAt == "" || len(final.Metadata["large"]) != 500 {
		t.Fatalf("unexpected final work: %+v", final)
	}
	if _, err := manager.Complete(work.ID, "session-1", services.BackgroundWorkFailed, "wrong", "duplicate", nil); err != nil {
		t.Fatalf("duplicate complete: %v", err)
	}
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		mu.Lock()
		n := len(events)
		mu.Unlock()
		if n == 2 {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("expected start and terminal events, got %d", len(events))
}

func TestBackgroundWorkCancelInvokesBackend(t *testing.T) {
	manager := services.NewBackgroundWorkManager()
	called := make(chan struct{}, 1)
	work, err := manager.Register(services.BackgroundWorkOptions{SessionID: "session-1", Kind: services.BackgroundWorkShell})
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	if err := manager.SetCancel(work.ID, func() { called <- struct{}{} }); err != nil {
		t.Fatalf("set cancel: %v", err)
	}
	final, err := manager.Cancel(work.ID, "session-1")
	if err != nil {
		t.Fatalf("cancel: %v", err)
	}
	if final.Status != services.BackgroundWorkCancelled {
		t.Fatalf("unexpected cancellation: %+v", final)
	}
	select {
	case <-called:
	case <-time.After(time.Second):
		t.Fatal("backend cancel was not invoked")
	}
}
