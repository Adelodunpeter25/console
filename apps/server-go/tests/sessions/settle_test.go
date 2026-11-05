// Settled chats: the kept-but-out-of-the-way state between done and deleted
// (docs/plan/settled-chats.md). Covers the service surface and the one
// invariant the whole design rests on: no purge ever eats a settled chat.
package tests

import (
	"errors"
	"testing"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/services/session"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/types"
)

func TestSettleAndUnsettle(t *testing.T) {
	_, sessions, _, _ := newTestManager(t)
	header, err := sessions.Create(types.CreateSessionOptions{Title: "done work", Cwd: "/tmp"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	id := header.ID

	if settled, err := sessions.Settle(id); err != nil || !settled {
		t.Fatalf("settle = %v, %v", settled, err)
	}
	loaded, err := sessions.Header(id)
	if err != nil || loaded == nil || loaded.SettledAt == nil {
		t.Fatalf("settled_at not set: %+v, %v", loaded, err)
	}
	firstSettledAt := *loaded.SettledAt

	// Idempotent: the original timestamp survives a second settle, so the
	// chat's position in the Settled section does not jump.
	if settled, err := sessions.Settle(id); err != nil || !settled {
		t.Fatalf("second settle = %v, %v", settled, err)
	}
	again, _ := sessions.Header(id)
	if again.SettledAt == nil || *again.SettledAt != firstSettledAt {
		t.Fatalf("settled_at moved on re-settle: %v -> %v", firstSettledAt, again.SettledAt)
	}

	// Settling is not activity: updated_at is untouched.
	if again.UpdatedAt != header.UpdatedAt {
		t.Fatalf("settle bumped updated_at: %d -> %d", header.UpdatedAt, again.UpdatedAt)
	}

	// A settled chat stays in the normal list (decision 2).
	list, err := sessions.ListFiltered(session.ListFilter{})
	if err != nil || len(list) != 1 || list[0].SettledAt == nil {
		t.Fatalf("settled chat missing from list: %#v, %v", list, err)
	}

	// Unsettle clears the mark and is idempotent.
	if unsettled, err := sessions.Unsettle(id); err != nil || !unsettled {
		t.Fatalf("unsettle = %v, %v", unsettled, err)
	}
	if unsettled, err := sessions.Unsettle(id); err != nil || !unsettled {
		t.Fatalf("second unsettle = %v, %v", unsettled, err)
	}
	final, _ := sessions.Header(id)
	if final.SettledAt != nil {
		t.Fatalf("settled_at survived unsettle: %v", final.SettledAt)
	}
}

func TestSettleRefusesBusySessions(t *testing.T) {
	_, sessions, _, _ := newTestManager(t)
	header, err := sessions.Create(types.CreateSessionOptions{Title: "working", Cwd: "/tmp"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	id := header.ID

	// The run-in-flight probe arrives as a callback (the run service's
	// IsActive); fake it busy here.
	busy := map[string]bool{}
	sessions.SetBusyCheck(func(id string) bool { return busy[id] })

	busy[id] = true
	if _, err := sessions.Settle(id); !errors.Is(err, session.ErrSessionBusy) {
		t.Fatalf("busy settle error = %v, want ErrSessionBusy", err)
	}

	// Unsettling has no busy limit: a settled chat is idle by definition,
	// and the refusal only ever guards the settle direction.
	busy[id] = false
	if settled, err := sessions.Settle(id); err != nil || !settled {
		t.Fatalf("settle = %v, %v", settled, err)
	}
	busy[id] = true
	if unsettled, err := sessions.Unsettle(id); err != nil || !unsettled {
		t.Fatalf("unsettle while busy = %v, %v", unsettled, err)
	}

	// A queued prompt is busy too, even with no run in flight.
	busy[id] = false
	if err := sessions.SaveQueuedPrompt(id, types.QueuedPrompt{ID: "q1", SessionID: id, Prompt: "next"}); err != nil {
		t.Fatalf("queue: %v", err)
	}
	if _, err := sessions.Settle(id); !errors.Is(err, session.ErrSessionBusy) {
		t.Fatalf("queued settle error = %v, want ErrSessionBusy", err)
	}
}

func TestSettleUnknownSession(t *testing.T) {
	_, sessions, _, _ := newTestManager(t)
	if settled, err := sessions.Settle("ghost"); err != nil || settled {
		t.Fatalf("settle ghost = %v, %v; want false, nil", settled, err)
	}
}

func TestPurgeNeverTouchesSettledChats(t *testing.T) {
	_, sessions, _, _ := newTestManager(t)
	header, err := sessions.Create(types.CreateSessionOptions{Title: "kept", Cwd: "/tmp"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if settled, err := sessions.Settle(header.ID); err != nil || !settled {
		t.Fatalf("settle = %v, %v", settled, err)
	}
	// The sweep lists rows by deleted_at only; a settled, non-deleted chat
	// must never appear, whatever the cutoff.
	expired, err := sessions.ExpiredDeletedSessions(1 << 62)
	if err != nil {
		t.Fatalf("expired: %v", err)
	}
	for _, id := range expired {
		if id == header.ID {
			t.Fatal("the purge swept a settled, non-deleted chat")
		}
	}
}
