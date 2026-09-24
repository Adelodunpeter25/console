// Task 2.2: setup is sent as a leading <setup> user message, after the
// stable system prompt and before the conversation, and stays identical
// across runs of the same session.
package tests

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/loop"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/stream"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/run"
	"github.com/Adelodunpeter25/console/apps/server-go/tests/helpers"
)

type setupRecorder struct {
	mu   sync.Mutex
	reqs []loop.TurnRequest
}

func (p *setupRecorder) RunTurn(ctx context.Context, req loop.TurnRequest, s *stream.Stream[loop.Event]) error {
	// Title generation shares the provider; only record agent turns.
	if len(req.Tools) > 0 {
		p.mu.Lock()
		p.reqs = append(p.reqs, req)
		p.mu.Unlock()
	}
	s.Push(loop.Event{Kind: loop.EventText, Text: "ok"})
	s.Complete()
	return nil
}

func TestWithSetupPrependsMessage(t *testing.T) {
	history := []any{loop.UserMessage{Role: loop.RoleUser, Content: "hi"}}
	if got := loop.WithSetup("", history); len(got) != 1 {
		t.Fatalf("empty setup must not add a message: %#v", got)
	}
	got := loop.WithSetup("env", history)
	first, ok := got[0].(loop.UserMessage)
	if len(got) != 2 || !ok || first.Content != "<setup>\nenv\n</setup>" {
		t.Fatalf("setup order: %#v", got)
	}
	if len(history) != 1 {
		t.Fatal("history must not be mutated")
	}
}

func TestRunSendsStableSetupAcrossTurns(t *testing.T) {
	sessions := helpers.NewRunSessions(t)
	svc := run.NewService(sessions)
	provider := &setupRecorder{}
	svc.Lookup = func(id string) (loop.Provider, error) { return provider, nil }
	header := helpers.CreateRunSession(t, sessions)

	for _, text := range []string{"first", "second"} {
		hub, err := svc.StartRun(header.ID, run.Prompt{Text: text, Provider: "mock", ModelID: "m"})
		if err != nil {
			t.Fatal(err)
		}
		select {
		case <-hub.Done():
		case <-time.After(10 * time.Second):
			t.Fatal("run did not settle")
		}
	}

	provider.mu.Lock()
	defer provider.mu.Unlock()
	if len(provider.reqs) != 2 {
		t.Fatalf("provider calls: %d", len(provider.reqs))
	}
	first, second := provider.reqs[0], provider.reqs[1]
	if first.Setup == "" || first.Setup != second.Setup || first.SystemPrompt != second.SystemPrompt {
		t.Fatal("system prompt and setup must stay identical across turns")
	}
	if strings.Contains(first.SystemPrompt, "Today is") || strings.Contains(first.SystemPrompt, "<workstation>") {
		t.Fatalf("system prompt must not carry setup:\n%s", first.SystemPrompt)
	}
	// Order: setup message, then the conversation.
	lead, ok := second.Messages[0].(loop.UserMessage)
	if !ok || !strings.HasPrefix(lead.Content, "<setup>") || !strings.Contains(lead.Content, "<workstation>") {
		t.Fatalf("first message must be setup: %#v", second.Messages[0])
	}
	if len(second.Messages) != 4 {
		t.Fatalf("setup + first + reply + second, got %d messages", len(second.Messages))
	}
	// Setup is never persisted into the session.
	loaded, err := sessions.Load(header.ID, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range loaded.Messages {
		if strings.Contains(string(m), "\u003csetup\u003e") || strings.Contains(string(m), "<setup>") {
			t.Fatalf("setup persisted: %s", m)
		}
	}
}
