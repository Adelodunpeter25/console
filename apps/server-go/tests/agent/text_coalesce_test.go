// Text-part coalescing: consecutive text/thinking deltas must persist as
// one part (TS streamOneTurn parity). Without this each streamed word is
// its own TextPart and desktop renders every part as a separate vertical
// markdown block — one word per line.
package tests

import (
	"context"
	"testing"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/loop"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/permissions"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/tools"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/db"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/services"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/types"
	"github.com/Adelodunpeter25/console/apps/server-go/tests/helpers"
)

func TestTurnCoalescesConsecutiveTextDeltas(t *testing.T) {
	manager, err := db.Open(db.OpenOptions{Path: ":memory:"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(manager.Close)
	sessions := services.NewSessionService(manager)
	header, err := sessions.Create(types.CreateSessionOptions{
		Cwd: "/tmp/coalesce-test", ModelID: "mock", Provider: "mock",
	})
	if err != nil {
		t.Fatal(err)
	}

	provider := &helpers.MockProvider{Turns: []func() []loop.Event{
		func() []loop.Event {
			return []loop.Event{
				{Kind: loop.EventText, Text: "Hi"},
				{Kind: loop.EventText, Text: "!"},
				{Kind: loop.EventText, Text: " What"},
				{Kind: loop.EventText, Text: " would"},
				{Kind: loop.EventThinking, Text: "hmm"},
				{Kind: loop.EventThinking, Text: " hmm2"},
				{Kind: loop.EventText, Text: " done"},
			}
		},
	}}
	registry := tools.NewRegistry(tools.DefaultTools()...)
	executor := loop.NewExecutor(registry, permissions.FullAccess, helpers.AutoApprover{})
	agent := loop.New(provider, executor, sessions)

	events, err := agent.Run(context.Background(), header.ID, "hi", registry.Definitions())
	if err != nil {
		t.Fatal(err)
	}
	var turn *loop.AssistantMessage
	for {
		event, err, ok := events.Next()
		if !ok {
			if err != nil {
				t.Fatalf("stream error: %v", err)
			}
			break
		}
		if event.Kind == loop.EventTurnDone {
			if m, ok := event.Message.(loop.AssistantMessage); ok {
				turn = &m
			}
		}
	}
	if turn == nil {
		t.Fatal("no turnDone event")
	}
	// Expect: one TextPart("Hi! What would"), one ThinkingPart, one TextPart(" done").
	if len(turn.Content) != 3 {
		t.Fatalf("content parts: %d (%#v)", len(turn.Content), turn.Content)
	}
	first, ok := turn.Content[0].(loop.TextPart)
	if !ok || first.Text != "Hi! What would" {
		t.Fatalf("first part: %#v", turn.Content[0])
	}
	thinking, ok := turn.Content[1].(loop.ThinkingPart)
	if !ok || thinking.Text != "hmm hmm2" {
		t.Fatalf("thinking part: %#v", turn.Content[1])
	}
	last, ok := turn.Content[2].(loop.TextPart)
	if !ok || last.Text != " done" {
		t.Fatalf("last part: %#v", turn.Content[2])
	}
}
