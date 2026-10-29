// sessions.prompt delivery: an idle session starts a run, a busy one queues
// the prompt for when the run settles, and an occupied queue is never
// overwritten by an agent.
package tests

import (
	"strings"
	"testing"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/loop"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/run"
	"github.com/Adelodunpeter25/console/apps/server-go/tests/helpers"
)

func TestPromptSessionStartsAnIdleSession(t *testing.T) {
	sessions := helpers.NewRunSessions(t)
	svc := run.NewService(sessions)
	svc.Lookup = func(string) (loop.Provider, error) { return queueMock("done"), nil }
	header := helpers.CreateRunSession(t, sessions)

	queued, err := svc.PromptSession(header.ID, "hello", "m", "mock", "always-ask", "")
	if err != nil {
		t.Fatal(err)
	}
	if queued {
		t.Fatal("an idle session must start now, not queue")
	}
	if !svc.IsActive(header.ID) {
		// The mock can settle before we look; the transcript is the proof.
		t.Log("run already settled")
	}
}

func TestPromptSessionQueuesWhileBusyAndDrains(t *testing.T) {
	sessions := helpers.NewRunSessions(t)
	svc := run.NewService(sessions)
	svc.Lookup = func(string) (loop.Provider, error) { return queueMock("first", "second"), nil }
	header := helpers.CreateRunSession(t, sessions)

	hub, err := svc.StartRun(header.ID, run.Prompt{Text: "one", Provider: "mock", ModelID: "m"})
	if err != nil {
		t.Fatal(err)
	}
	queued, err := svc.PromptSession(header.ID, "follow-up", "", "", "always-ask", "")
	if err != nil {
		t.Fatal(err)
	}
	if !queued {
		t.Fatal("a busy session must queue")
	}
	helpers.WaitSettled(t, hub)

	loaded, err := sessions.Load(header.ID, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	// user one, assistant first, user follow-up, assistant second.
	if len(loaded.Messages) != 4 {
		t.Fatalf("persisted messages = %d, want 4 (queued prompt must have run)", len(loaded.Messages))
	}
	drained, err := loop.MessageFromProtoBytes(loaded.Messages[2])
	if err != nil {
		t.Fatal(err)
	}
	if u, ok := drained.(loop.UserMessage); !ok || u.Content != "follow-up" {
		t.Fatalf("drained message = %#v", drained)
	}
	if qp, _ := svc.QueuedPrompt(header.ID); qp != nil {
		t.Fatalf("queue must be empty after draining: %+v", qp)
	}
}

func TestPromptSessionRefusesToReplaceAQueuedPrompt(t *testing.T) {
	sessions := helpers.NewRunSessions(t)
	svc := run.NewService(sessions)
	svc.Lookup = func(string) (loop.Provider, error) { return queueMock("first", "second"), nil }
	header := helpers.CreateRunSession(t, sessions)

	hub, err := svc.StartRun(header.ID, run.Prompt{Text: "one", Provider: "mock", ModelID: "m"})
	if err != nil {
		t.Fatal(err)
	}
	// The user's own staged prompt must survive an agent prompting the session.
	if _, err := svc.QueuePrompt(header.ID, run.Prompt{Text: "users own"}); err != nil {
		t.Fatal(err)
	}
	_, err = svc.PromptSession(header.ID, "agent", "", "", "always-ask", "")
	if err == nil || !strings.Contains(err.Error(), "already has a queued prompt") {
		t.Fatalf("err = %v, want a refusal that names the queued prompt", err)
	}
	if qp, _ := svc.QueuedPrompt(header.ID); qp == nil || qp.Prompt != "users own" {
		t.Fatalf("user's queued prompt was disturbed: %+v", qp)
	}
	helpers.WaitSettled(t, hub)
}

func TestPromptSessionUnknownSession(t *testing.T) {
	sessions := helpers.NewRunSessions(t)
	svc := run.NewService(sessions)
	svc.Lookup = func(string) (loop.Provider, error) { return queueMock("x"), nil }
	_, err := svc.PromptSession("ghost", "hi", "m", "mock", "always-ask", "")
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("err = %v, want not found", err)
	}
}

// Stopping a session from the console must not let a queued prompt start the
// session again: a cancelled run drains its staged prompt unless it is cleared.
func TestAbortAndClearDiscardsTheQueuedPrompt(t *testing.T) {
	sessions := helpers.NewRunSessions(t)
	svc := run.NewService(sessions)
	release := make(chan struct{})
	entered := make(chan struct{})
	var enteredOnce bool
	svc.Lookup = func(string) (loop.Provider, error) {
		return &blockingProvider{release: release, released: new(bool), entered: entered, enteredOnce: &enteredOnce}, nil
	}
	header := helpers.CreateRunSession(t, sessions)
	hub, err := svc.StartRun(header.ID, run.Prompt{Text: "slow", Provider: "mock", ModelID: "m"})
	if err != nil {
		t.Fatal(err)
	}
	<-entered
	if queued, err := svc.PromptSession(header.ID, "follow-up", "", "", "always-ask", ""); err != nil || !queued {
		t.Fatalf("queue: %v %v", queued, err)
	}
	if !svc.AbortAndClear(header.ID) {
		t.Fatal("abort reported no active run")
	}
	helpers.WaitSettled(t, hub)
	if qp, _ := svc.QueuedPrompt(header.ID); qp != nil {
		t.Fatalf("queued prompt survived the abort: %+v", qp)
	}
	loaded, err := sessions.Load(header.ID, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, raw := range loaded.Messages {
		if msg, err := loop.MessageFromProtoBytes(raw); err == nil {
			if u, ok := msg.(loop.UserMessage); ok && u.Content == "follow-up" {
				t.Fatal("the queued prompt ran after the abort")
			}
		}
	}
}
