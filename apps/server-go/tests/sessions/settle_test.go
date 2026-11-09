// Settled chats: the kept-but-out-of-the-way state between done and deleted
// (docs/plan/settled-chats.md). Covers the service surface, the pin/settle
// exclusivity, and retention: the permanent purge never eats a settled chat
// directly; the settled sweep soft-deletes it first, after 7 days.
package tests

import (
	"errors"
	"testing"
	"time"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/run"

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

func TestPinAndSettleAreMutuallyExclusive(t *testing.T) {
	_, sessions, _, _ := newTestManager(t)
	header, err := sessions.Create(types.CreateSessionOptions{Title: "either", Cwd: "/tmp"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	id := header.ID

	if err := sessions.SetPinned(id, true); err != nil {
		t.Fatalf("pin: %v", err)
	}
	if settled, err := sessions.Settle(id); err != nil || !settled {
		t.Fatalf("settle = %v, %v", settled, err)
	}
	got, _ := sessions.Header(id)
	if got.SettledAt == nil || got.PinnedAt != nil {
		t.Fatalf("settling must unpin: settled=%v pinned=%v", got.SettledAt, got.PinnedAt)
	}

	if err := sessions.SetPinned(id, true); err != nil {
		t.Fatalf("re-pin: %v", err)
	}
	got, _ = sessions.Header(id)
	if got.PinnedAt == nil || got.SettledAt != nil {
		t.Fatalf("pinning must unsettle: settled=%v pinned=%v", got.SettledAt, got.PinnedAt)
	}

	// Unpinning leaves the settled mark alone.
	if _, err := sessions.Settle(id); err != nil {
		t.Fatalf("settle: %v", err)
	}
	if err := sessions.SetPinned(id, false); err != nil {
		t.Fatalf("unpin: %v", err)
	}
	got, _ = sessions.Header(id)
	if got.SettledAt == nil {
		t.Fatal("unpinning cleared the settled mark")
	}
}

func TestSettledSweepSoftDeletesAfterRetention(t *testing.T) {
	manager, sessions, _, _ := newTestManager(t)
	svc := run.NewService(sessions)
	old := time.Now().UnixMilli() - (run.SettledSessionRetention + time.Hour).Milliseconds()

	mk := func(title string) string {
		h, err := sessions.Create(types.CreateSessionOptions{Title: title, Cwd: "/tmp"})
		if err != nil {
			t.Fatalf("create: %v", err)
		}
		return h.ID
	}
	set := func(id, column string) {
		if _, err := manager.Global().Exec(
			`UPDATE sessions SET `+column+` = ? WHERE id = ?`, old, id); err != nil {
			t.Fatal(err)
		}
	}
	settle := func(id string) {
		if settled, err := sessions.Settle(id); err != nil || !settled {
			t.Fatalf("settle = %v, %v", settled, err)
		}
	}

	expired := mk("expired")
	settle(expired)
	set(expired, "settled_at")
	set(expired, "updated_at")

	recent := mk("recently settled")
	settle(recent)
	set(recent, "updated_at")

	touched := mk("settled long ago, touched since")
	settle(touched)
	set(touched, "settled_at")

	active := mk("never settled")
	set(active, "updated_at")

	legacyPinned := mk("pinned and settled before exclusivity")
	settle(legacyPinned)
	set(legacyPinned, "settled_at")
	set(legacyPinned, "updated_at")
	set(legacyPinned, "pinned_at")

	deleted := svc.DeleteExpiredSettledSessions()
	if len(deleted) != 1 || deleted[0] != expired {
		t.Fatalf("deleted = %v, want [%s]", deleted, expired)
	}
	if h, _ := sessions.Header(expired); h != nil {
		t.Fatal("expired settled chat still live")
	}
	// Soft-deleted, not purged: it stays restorable.
	if h, err := sessions.HeaderIncludingDeleted(expired); err != nil || h == nil || h.DeletedAt == nil {
		t.Fatalf("expired settled chat not soft-deleted: %+v, %v", h, err)
	}
	if purged := svc.PurgeExpiredDeletedSessions(); len(purged) != 0 {
		t.Fatalf("freshly swept chat purged immediately: %v", purged)
	}
	for _, id := range []string{recent, touched, active, legacyPinned} {
		if h, _ := sessions.Header(id); h == nil {
			t.Fatalf("sweep deleted %s, which must survive", id)
		}
	}
	if again := svc.DeleteExpiredSettledSessions(); len(again) != 0 {
		t.Fatalf("second sweep = %v, want none", again)
	}
}
