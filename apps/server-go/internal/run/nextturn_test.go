// nextTurn coverage: a canceled settle drains the staged prompt only for a
// steer cancel. A Stop (or watchdog, or plain context.Canceled) must leave
// the queue untouched — the aborted run ends and each staged prompt starts
// as its own run via startStagedIfIdle. Draining here would hold this run's
// active slot and reject the user's next send with ErrActive.
package run

import (
	"context"
	"testing"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/db"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/services"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/types"
)

func nextTurnSessions(t *testing.T) *services.SessionService {
	t.Helper()
	manager, err := db.Open(db.OpenOptions{Path: ":memory:"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(manager.Close)
	return services.NewSessionService(manager)
}

func nextTurnSession(t *testing.T, sessions *services.SessionService) types.SessionHeader {
	t.Helper()
	header, err := sessions.Create(types.CreateSessionOptions{Cwd: t.TempDir(), ModelID: "m", Provider: "mock"})
	if err != nil {
		t.Fatal(err)
	}
	return header
}

func TestNextTurnLeavesQueueOnAbort(t *testing.T) {
	sessions := nextTurnSessions(t)
	svc := NewService(sessions)
	header := nextTurnSession(t, sessions)
	for _, tc := range []struct {
		name  string
		cause error
	}{
		{"abort", errRunAborted},
		{"plain cancel", context.Canceled},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := svc.QueuePrompt(header.ID, Prompt{Text: "queued"}); err != nil {
				t.Fatalf("queue: %v", err)
			}
			ctx, cancel := context.WithCancelCause(context.Background())
			cancel(tc.cause)
			if _, ok := svc.nextTurn(ctx, context.Canceled, header.ID, nil); ok {
				t.Fatal("aborted run drained a staged prompt")
			}
			if qp, _ := svc.QueuedPrompt(header.ID); qp == nil {
				t.Fatal("abort-cancel settle dropped the staged prompt")
			}
			// Clean up for the next subtest.
			if _, err := svc.ClearQueuedPrompt(header.ID); err != nil {
				t.Fatalf("clear: %v", err)
			}
		})
	}
}

func TestNextTurnDrainsStagedPromptOnSteer(t *testing.T) {
	sessions := nextTurnSessions(t)
	svc := NewService(sessions)
	header := nextTurnSession(t, sessions)
	hub := NewHub()
	defer hub.Close(OutcomeDone)
	if _, err := svc.QueuePrompt(header.ID, Prompt{Text: "steer me"}); err != nil {
		t.Fatalf("queue: %v", err)
	}

	ctx, cancel := context.WithCancelCause(context.Background())
	cancel(errRunSteered)
	prompt, ok := svc.nextTurn(ctx, context.Canceled, header.ID, hub)
	if !ok || prompt.Text != "steer me" {
		t.Fatalf("steer-cancel settle did not drain the staged prompt: ok=%v prompt=%+v", ok, prompt)
	}
}
