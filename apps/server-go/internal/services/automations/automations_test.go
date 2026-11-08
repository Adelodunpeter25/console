// The schedule store behind console.automation.*: one active automation per
// session, due scanning, and the post-fire advance.
package automations

import (
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/db"
)

func newTestDB(t *testing.T) *sql.DB {
	t.Helper()
	manager, err := db.Open(db.OpenOptions{Path: ":memory:"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(manager.Close)
	return manager.Global()
}

func newTestStore(t *testing.T) *Service {
	t.Helper()
	return New(newTestDB(t))
}

func TestCreateOneActivePerSession(t *testing.T) {
	store := newTestStore(t)
	now := time.Now()
	if _, err := store.Create("s1", "check the backfill", KindInterval, 300, 0, now); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Create("s1", "another", KindOnce, 0, now.Add(time.Hour).Unix(), now); !errors.Is(err, ErrActiveExists) {
		t.Fatalf("second create on one session = %v, want ErrActiveExists", err)
	}
	if _, err := store.Create("s2", "other thread", KindOnce, 0, now.Add(time.Hour).Unix(), now); err != nil {
		t.Fatal(err)
	}
}

func TestValidation(t *testing.T) {
	store := newTestStore(t)
	now := time.Now()
	cases := []struct {
		prompt, kind    string
		interval, runAt int64
	}{
		{"", KindInterval, 300, 0},
		{"x", "cron", 300, 0},
		{"x", KindInterval, 0, 0},
		{"x", KindOnce, 0, 0},
	}
	for _, c := range cases {
		if _, err := store.Create("s1", c.prompt, c.kind, c.interval, c.runAt, now); err == nil {
			t.Fatalf("create(%q, %q, %d, %d) should fail", c.prompt, c.kind, c.interval, c.runAt)
		}
	}
}

func TestDueAndAfterFire(t *testing.T) {
	store := newTestStore(t)
	now := time.Now()

	if _, err := store.Create("s1", "tick", KindInterval, 300, 0, now); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Create("s2", "remind", KindOnce, 0, now.Unix()-10, now); err != nil {
		t.Fatal(err)
	}
	due, err := store.Due(now.Add(time.Minute))
	if err != nil || len(due) != 1 || due[0].SessionID != "s2" {
		t.Fatalf("expected only the overdue one-shot due: %v %v", due, err)
	}
	// The interval automation is due once its interval has passed.
	later, err := store.Due(now.Add(301 * time.Second))
	if err != nil || len(later) != 2 {
		t.Fatalf("expected both due after the interval: %v %v", later, err)
	}
	if later[0].SessionID != "s2" || later[1].SessionID != "s1" {
		t.Fatalf("wrong fire order: %s then %s", later[0].SessionID, later[1].SessionID)
	}
	// At creation time the overdue one-shot is already due; the interval is not.
	if soon, _ := store.Due(now); len(soon) != 1 || soon[0].SessionID != "s2" {
		t.Fatalf("expected only the one-shot due at creation, got %d", len(soon))
	}

	var intervalID, onceID string
	list, _ := store.ListForSession("s1")
	intervalID = list[0].ID
	list, _ = store.ListForSession("s2")
	onceID = list[0].ID

	if err := store.AfterFire(onceID, nil, now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	done, _ := store.Get(onceID)
	if done.Status != StatusDone {
		t.Fatalf("one-shot should be done after firing, got %s", done.Status)
	}
	next := now.Add(time.Minute)
	if err := store.AfterFire(intervalID, nil, next); err != nil {
		t.Fatal(err)
	}
	recurring, _ := store.Get(intervalID)
	if recurring.Status != StatusActive || recurring.NextRunAt != next.Unix()+300 {
		t.Fatalf("interval should re-arm +300s, got status=%s next=%d", recurring.Status, recurring.NextRunAt)
	}
	// A failed fire marks the row error instead of looping every tick.
	if err := store.AfterFire(intervalID, errors.New("session gone"), next); err != nil {
		t.Fatal(err)
	}
	broken, _ := store.Get(intervalID)
	if broken.Status != StatusError || broken.LastError != "session gone" {
		t.Fatalf("failed fire should mark error, got %s (%s)", broken.Status, broken.LastError)
	}
}

func TestUpdatePausedAndDelete(t *testing.T) {
	store := newTestStore(t)
	now := time.Now()
	a, err := store.Create("s1", "first", KindInterval, 600, 0, now)
	if err != nil {
		t.Fatal(err)
	}
	if ok, _ := store.SetStatus(a.ID, StatusPaused); !ok {
		t.Fatal("pause failed")
	}
	// A paused automation can still be updated (adjust then resume), but does
	// not fire while paused.
	if got, _ := store.Update(a.ID, "second", KindInterval, 600, 0, now); got == nil {
		t.Fatal("paused automation should update")
	}
	if due, _ := store.Due(now.Add(time.Hour)); len(due) != 0 {
		t.Fatalf("paused automation is due: %v", due)
	}
	if ok, _ := store.Delete(a.ID); !ok {
		t.Fatal("delete failed")
	}
	if got, _ := store.Get(a.ID); got != nil {
		t.Fatal("deleted automation still exists")
	}
}
