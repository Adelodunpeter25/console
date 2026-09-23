// Run pipeline coverage: provider registry, hub replay, history decode,
// and end-to-end StartRun/Abort against scripted mock providers.
package tests

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/loop"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/stream"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/providers"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/run"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/types"
	"github.com/Adelodunpeter25/console/apps/server-go/tests/helpers"
)

func TestProviderRegistry(t *testing.T) {
	p, err := providers.Lookup("codex")
	if err != nil || p == nil {
		t.Fatalf("codex lookup: %v", p)
	}
	if _, err := providers.Lookup("claude"); err != nil {
		t.Fatalf("claude lookup: %v", err)
	}
	if _, err := providers.Lookup("antigravity"); err != nil {
		t.Fatalf("antigravity lookup: %v", err)
	}
	if _, err := providers.Lookup("opencode"); err != nil {
		t.Fatalf("opencode lookup: %v", err)
	}
	if _, err := providers.Lookup("devin"); err == nil {
		t.Fatal("unported provider must fail")
	}
	if _, err := providers.Lookup("nope"); err == nil {
		t.Fatal("unknown provider must fail")
	}
}

func TestHubReplay(t *testing.T) {
	hub := run.NewHub()
	hub.Broadcast(loop.Event{Kind: loop.EventText, Text: "a"})
	hub.Broadcast(loop.Event{Kind: loop.EventText, Text: "b"})
	hub.Broadcast(loop.Event{Kind: loop.EventTurnDone})
	// Live subscriber receives new frames.
	since := int64(0)
	id, ch, replay := hub.Subscribe(&since)
	if ch == nil {
		t.Fatal("subscribe must succeed while open")
	}
	defer hub.Unsubscribe(id)
	if len(replay) != 3 || replay[0].Seq != 1 || replay[2].Event.Kind != loop.EventTurnDone {
		t.Fatalf("replay: %+v", replay)
	}
	// Nothing newer than the latest seq.
	latest := replay[len(replay)-1].Seq
	_, _, empty := hub.Subscribe(&latest)
	if len(empty) != 0 {
		t.Fatalf("expected no frames after %d, got %d", latest, len(empty))
	}
	// Live delivery.
	hub.Broadcast(loop.Event{Kind: loop.EventText, Text: "c"})
	select {
	case f := <-ch:
		if f.Event.Text != "c" {
			t.Fatalf("live frame: %+v", f)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("live frame not delivered")
	}
	hub.Close(run.OutcomeDone)
	if _, ch, _ := hub.Subscribe(nil); ch != nil {
		t.Fatal("subscribe after close must fail")
	}
}

func TestRunHistoryRoundTrip(t *testing.T) {
	sessions := helpers.NewRunSessions(t)
	svc := run.NewService(sessions)
	svc.Lookup = func(id string) (loop.Provider, error) {
		return &helpers.MockProvider{Turns: []func() []loop.Event{
			func() []loop.Event { return []loop.Event{{Kind: loop.EventText, Text: "second"}} },
		}}, nil
	}
	header := helpers.CreateRunSession(t, sessions)
	// Seed a prior turn directly, as stored by the loop.
	seedUser, _ := json.Marshal(map[string]any{"role": "user", "content": "first"})
	seedAssistant, _ := json.Marshal(map[string]any{
		"role": "assistant", "id": "m1", "stopReason": "stop",
		"content": []any{map[string]any{"type": "text", "text": "reply"}},
	})
	if err := sessions.AppendMessages(header.ID, []types.AgentMessage{
		{ID: "u1", Role: "user", Data: seedUser},
		{ID: "m1", Role: "assistant", Data: seedAssistant},
	}); err != nil {
		t.Fatal(err)
	}
	hub, err := svc.StartRun(header.ID, run.Prompt{Text: "follow-up", Provider: "mock", ModelID: "m"})
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-hub.Done():
	case <-time.After(10 * time.Second):
		t.Fatal("run did not settle")
	}
	loaded, err := sessions.Load(header.ID, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	// Seed (2) + new user + new assistant = 4 messages.
	if len(loaded.Messages) != 4 {
		t.Fatalf("persisted messages: %d", len(loaded.Messages))
	}
	if svc.IsActive(header.ID) {
		t.Fatal("run must be inactive after settle")
	}
}

func TestRunDoubleStartAndAbort(t *testing.T) {
	sessions := helpers.NewRunSessions(t)
	svc := run.NewService(sessions)
	release := make(chan struct{})
	var released bool
	svc.Lookup = func(id string) (loop.Provider, error) {
		return &blockingProvider{release: release, released: &released}, nil
	}
	header := helpers.CreateRunSession(t, sessions)
	hub, err := svc.StartRun(header.ID, run.Prompt{Text: "go", Provider: "mock", ModelID: "m"})
	if err != nil {
		t.Fatal(err)
	}
	if !svc.IsActive(header.ID) {
		t.Fatal("run must be active")
	}
	if _, err := svc.StartRun(header.ID, run.Prompt{Text: "again", Provider: "mock", ModelID: "m"}); err == nil {
		t.Fatal("second start must fail while active")
	}
	if !svc.Abort(header.ID) {
		t.Fatal("abort must succeed")
	}
	close(release)
	select {
	case <-hub.Done():
	case <-time.After(10 * time.Second):
		t.Fatal("aborted run did not settle")
	}
	if hub.Outcome != run.OutcomeAborted {
		t.Fatalf("outcome: %s", hub.Outcome)
	}
	if svc.IsActive(header.ID) {
		t.Fatal("run must be inactive after abort")
	}
	if svc.Abort(header.ID) {
		t.Fatal("abort with no active run must fail")
	}
}

func TestRunUnknownSessionAndProvider(t *testing.T) {
	sessions := helpers.NewRunSessions(t)
	svc := run.NewService(sessions)
	if _, err := svc.StartRun("missing", run.Prompt{Text: "hi"}); err == nil {
		t.Fatal("missing session must fail")
	}
	header := helpers.CreateRunSession(t, sessions)
	if _, err := svc.StartRun(header.ID, run.Prompt{Text: "hi", Provider: "devin", ModelID: "m"}); err == nil {
		t.Fatal("unported provider must fail")
	}
	if svc.IsActive(header.ID) {
		t.Fatal("failed start must not leave an active run")
	}
}

// blockingProvider stalls RunTurn until release closes or ctx ends.
type blockingProvider struct {
	release     chan struct{}
	released    *bool
	entered     chan struct{}
	enteredOnce *bool
}

func (b *blockingProvider) RunTurn(ctx context.Context, req loop.TurnRequest, s *stream.Stream[loop.Event]) error {
	if b.entered != nil && (b.enteredOnce == nil || !*b.enteredOnce) {
		if b.enteredOnce != nil {
			*b.enteredOnce = true
		}
		close(b.entered)
	}
	select {
	case <-b.release:
		*(b.released) = true
		s.Push(loop.Event{Kind: loop.EventText, Text: "late"})
		s.Complete()
		return nil
	case <-ctx.Done():
		s.Fail(ctx.Err())
		return ctx.Err()
	}
}
