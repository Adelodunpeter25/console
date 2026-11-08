// Automations: the console.automation.* tools and the schedule store they
// write through, including the one-active-per-session rule and the
// scanner's fire/advance cycle.
package tests

import (
	"fmt"
	"testing"
	"time"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/tools"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/db"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/services/automations"
)

func newAutomationStore(t *testing.T) *automations.Service {
	t.Helper()
	manager, err := db.Open(db.OpenOptions{Path: ":memory:"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(manager.Close)
	return automations.New(manager.Global())
}

func callTool(t *testing.T, tool tools.Tool, args string) any {
	t.Helper()
	out, err := tool.Execute(t.Context(), []byte(args))
	if err != nil {
		t.Fatalf("execute %s: %v", tool.Name(), err)
	}
	return out
}

func callToolErr(t *testing.T, tool tools.Tool, args string) {
	t.Helper()
	if _, err := tool.Execute(t.Context(), []byte(args)); err == nil {
		t.Fatalf("execute %s: expected error", tool.Name())
	}
}

func TestAutomationCreateOneActivePerSession(t *testing.T) {
	store := newAutomationStore(t)
	create := tools.NewAutomationCreateTool("s1", store)
	callTool(t, create, `{"prompt":"check the backfill","kind":"interval","intervalSeconds":300}`)

	runAt := fmt.Sprintf("%d", time.Now().Add(time.Hour).Unix())
	// The second active automation on the session is refused.
	callToolErr(t, tools.NewAutomationCreateTool("s1", store),
		`{"prompt":"another","kind":"once","runAt":`+runAt+`}`)

	// A different session is fine.
	callTool(t, tools.NewAutomationCreateTool("s2", store),
		`{"prompt":"other thread","kind":"once","runAt":`+runAt+`}`)
}

func TestAutomationDeleteAndSessionScope(t *testing.T) {
	store := newAutomationStore(t)
	runAt := fmt.Sprintf("%d", time.Now().Add(time.Hour).Unix())
	callTool(t, tools.NewAutomationCreateTool("s1", store),
		`{"prompt":"remind me","kind":"once","runAt":`+runAt+`}`)
	list := callTool(t, tools.NewAutomationGetTool("s1", store), `{}`).(string)
	if list == "No automations on this session." {
		t.Fatal("expected one automation, got none")
	}
	id := automationID(t, store, "s1")

	// Another session cannot see or delete it.
	callToolErr(t, tools.NewAutomationDeleteTool("s2", store), `{"id":"`+id+`"}`)
	callToolErr(t, tools.NewAutomationUpdateTool("s2", store), `{"id":"`+id+`","prompt":"x"}`)
	callTool(t, tools.NewAutomationDeleteTool("s1", store), `{"id":"`+id+`"}`)
	if left := callTool(t, tools.NewAutomationGetTool("s1", store), `{}`).(string); left != "No automations on this session." {
		t.Fatalf("expected deleted automation to be gone, got: %s", left)
	}
}

func automationID(t *testing.T, store *automations.Service, sessionID string) string {
	t.Helper()
	list, err := store.ListForSession(sessionID)
	if err != nil || len(list) != 1 {
		t.Fatalf("expected one automation on %s: %v", sessionID, err)
	}
	return list[0].ID
}

func TestAutomationStoreDueAndAfterFire(t *testing.T) {
	store := newAutomationStore(t)
	now := time.Now()

	if _, err := store.Create("s1", "tick", automations.KindInterval, 300, 0, now); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Create("s2", "remind", automations.KindOnce, 0, now.Unix()-10, now); err != nil {
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

	intervalID := automationID(t, store, "s1")
	onceID := automationID(t, store, "s2")
	if err := store.AfterFire(onceID, nil, now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	done, _ := store.Get(onceID)
	if done.Status != automations.StatusDone {
		t.Fatalf("one-shot should be done after firing, got %s", done.Status)
	}
	next := now.Add(time.Minute)
	if err := store.AfterFire(intervalID, nil, next); err != nil {
		t.Fatal(err)
	}
	recurring, _ := store.Get(intervalID)
	if recurring.Status != automations.StatusActive || recurring.NextRunAt != next.Unix()+300 {
		t.Fatalf("interval should re-arm +300s, got status=%s next=%d", recurring.Status, recurring.NextRunAt)
	}
	// A failed fire marks the row error instead of looping.
	if err := store.AfterFire(intervalID, errSessionGone, next); err != nil {
		t.Fatal(err)
	}
	broken, _ := store.Get(intervalID)
	if broken.Status != automations.StatusError || broken.LastError != errSessionGone.Error() {
		t.Fatalf("failed fire should mark error, got %s (%s)", broken.Status, broken.LastError)
	}
}

var errSessionGone = fmt.Errorf("session gone")
