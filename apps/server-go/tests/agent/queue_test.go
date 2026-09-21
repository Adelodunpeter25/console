// Queue + steer coverage: stage/edit/fetch/discard persistence, drain as
// next turn on settle, steer abort-and-drain, error holds the staged prompt.
package tests

import (
	"testing"
	"time"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/loop"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/run"
	"github.com/Adelodunpeter25/console/apps/server-go/tests/helpers"
)

func queueMock(texts ...string) *helpers.MockProvider {
	turns := make([]func() []loop.Event, 0, len(texts))
	for _, text := range texts {
		text := text
		turns = append(turns, func() []loop.Event {
			return []loop.Event{{Kind: loop.EventText, Text: text}}
		})
	}
	return &helpers.MockProvider{Turns: turns}
}

func TestQueuePersistence(t *testing.T) {
	sessions := helpers.NewRunSessions(t)
	svc := run.NewService(sessions)
	header := helpers.CreateRunSession(t, sessions)

	if qp, err := svc.QueuedPrompt(header.ID); err != nil || qp != nil {
		t.Fatalf("empty queue: %+v %v", qp, err)
	}
	staged, err := svc.QueuePrompt(header.ID, run.Prompt{Text: "later", ModelID: "m1"})
	if err != nil {
		t.Fatal(err)
	}
	if staged.Prompt != "later" || staged.SessionID != header.ID || staged.ID == "" {
		t.Fatalf("staged: %+v", staged)
	}
	fetched, err := svc.QueuedPrompt(header.ID)
	if err != nil || fetched == nil || fetched.ID != staged.ID {
		t.Fatalf("fetched: %+v %v", fetched, err)
	}
	edited, err := svc.EditQueuedPrompt(header.ID, run.Prompt{Text: "edited"})
	if err != nil || edited == nil || edited.Prompt != "edited" || edited.ID != staged.ID {
		t.Fatalf("edited: %+v %v", edited, err)
	}
	had, err := svc.ClearQueuedPrompt(header.ID)
	if err != nil || !had {
		t.Fatalf("clear: %v %v", had, err)
	}
	if qp, _ := svc.QueuedPrompt(header.ID); qp != nil {
		t.Fatalf("queue must be empty: %+v", qp)
	}
	had, err = svc.ClearQueuedPrompt(header.ID)
	if err != nil || had {
		t.Fatalf("second clear: %v %v", had, err)
	}
	if _, err := svc.EditQueuedPrompt(header.ID, run.Prompt{Text: "x"}); err != nil {
		t.Fatalf("edit on empty must not error: %v", err)
	}
	if _, err := svc.QueuePrompt("missing", run.Prompt{Text: "x"}); err == nil {
		t.Fatal("queue on missing session must fail")
	}
}

func TestQueueDrainsNextTurn(t *testing.T) {
	sessions := helpers.NewRunSessions(t)
	svc := run.NewService(sessions)
	// One two-script mock: turn one emits "first", the drained turn two
	// emits "second".
	svc.Lookup = func(id string) (loop.Provider, error) {
		return queueMock("first", "second"), nil
	}
	header := helpers.CreateRunSession(t, sessions)
	hub, err := svc.StartRun(header.ID, run.Prompt{Text: "one", Provider: "mock", ModelID: "m"})
	if err != nil {
		t.Fatal(err)
	}
	// Subscribe live before staging: replay after close is unavailable.
	subID, subCh, _ := hub.Subscribe(nil)
	defer hub.Unsubscribe(subID)
	// Stage while the first turn runs; it must drain as turn two on the
	// same hub (sequence keeps increasing, single terminal frame).
	if _, err := svc.QueuePrompt(header.ID, run.Prompt{Text: "two"}); err != nil {
		t.Fatal(err)
	}
	helpers.WaitSettled(t, hub)
	if hub.Outcome != run.OutcomeDone {
		t.Fatalf("outcome: %s", hub.Outcome)
	}
	var texts []string
	for f := range subCh {
		switch f.Event.Kind {
		case loop.EventText:
			texts = append(texts, f.Event.Text)
		case loop.EventQueueUpdated:
			texts = append(texts, "<queueUpdated>")
		}
	}
	joined := ""
	for _, text := range texts {
		joined += text + "|"
	}
	if joined != "<queueUpdated>|first|<queueUpdated>|second|" {
		t.Fatalf("turn order: %q", joined)
	}
	if qp, _ := svc.QueuedPrompt(header.ID); qp != nil {
		t.Fatalf("queue must drain: %+v", qp)
	}
	loaded, err := sessions.Load(header.ID, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Messages) != 4 {
		t.Fatalf("persisted messages: %d", len(loaded.Messages))
	}
}

func TestSteerAbortsAndDrains(t *testing.T) {
	sessions := helpers.NewRunSessions(t)
	svc := run.NewService(sessions)
	release := make(chan struct{})
	entered := make(chan struct{})
	var enteredOnce bool
	// The steered turn switches provider id, so the drained turn runs on a
	// fresh mock (also covering provider switching mid-chain).
	svc.Lookup = func(id string) (loop.Provider, error) {
		if id == "fast-mock" {
			return queueMock("fast"), nil
		}
		return &blockingProvider{release: release, released: new(bool), entered: entered, enteredOnce: &enteredOnce}, nil
	}
	header := helpers.CreateRunSession(t, sessions)
	hub, err := svc.StartRun(header.ID, run.Prompt{Text: "slow", Provider: "mock", ModelID: "m"})
	if err != nil {
		t.Fatal(err)
	}
	subID, subCh, _ := hub.Subscribe(nil)
	defer hub.Unsubscribe(subID)
	// Steer with no active run on another session must fail.
	other := helpers.CreateRunSession(t, sessions)
	if ok, _ := svc.Steer(other.ID, run.Prompt{Text: "x"}); ok {
		t.Fatal("steer without active run must fail")
	}
	// Wait until the first turn is inside the provider, then steer: the
	// abort must settle it and the staged prompt must drain as turn two.
	select {
	case <-entered:
	case <-time.After(10 * time.Second):
		t.Fatal("provider never entered")
	}
	if ok, err := svc.Steer(header.ID, run.Prompt{Text: "fast", Provider: "fast-mock"}); err != nil || !ok {
		t.Fatalf("steer: %v %v", ok, err)
	}
	// Leave release open: turn one can only settle via the steer cancel,
	// so the drain of the staged turn is deterministic.
	helpers.WaitSettled(t, hub)
	if hub.Outcome != run.OutcomeDone {
		t.Fatalf("outcome: %s", hub.Outcome)
	}
	var texts []string
	for f := range subCh {
		if f.Event.Kind == loop.EventText {
			texts = append(texts, f.Event.Text)
		}
	}
	joined := ""
	for _, text := range texts {
		joined += text + "|"
	}
	if joined != "fast|" {
		t.Fatalf("steer order: %q", joined)
	}
	if qp, _ := svc.QueuedPrompt(header.ID); qp != nil {
		t.Fatalf("queue must drain: %+v", qp)
	}
}
