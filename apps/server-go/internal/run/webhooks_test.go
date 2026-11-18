package run

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/loop"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/stream"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/db"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/services"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/services/webhooks"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/types"
)

type fakeWebhookProvider struct {
	prompts chan string
}

func (p *fakeWebhookProvider) RunTurn(_ context.Context, req loop.TurnRequest, s *stream.Stream[loop.Event]) error {
	if len(req.Messages) > 0 {
		if u, ok := req.Messages[len(req.Messages)-1].(loop.UserMessage); ok {
			p.prompts <- u.Content
		}
	}
	s.Push(loop.Event{Kind: loop.EventText, Text: "ok"})
	s.Push(loop.Event{Kind: loop.EventTurnDone, StopReason: "end_turn"})
	s.Complete()
	return nil
}

func TestDeliverWebhookWakesSessionWithPayload(t *testing.T) {
	dir := t.TempDir()
	database, err := db.Open(db.OpenOptions{Path: filepath.Join(dir, "global.db"), StorageDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	sessions := services.NewSessionService(database)
	sess, err := sessions.Create(types.CreateSessionOptions{
		Title: "CI watcher", Cwd: dir, ModelID: "m1", Provider: "fake",
	})
	if err != nil {
		t.Fatal(err)
	}

	prov := &fakeWebhookProvider{prompts: make(chan string, 1)}
	svc := NewService(sessions)
	svc.Lookup = func(string) (loop.Provider, error) { return prov, nil }
	store := webhooks.New(database.Global())
	svc.SetWebhooks(store)

	sub, err := store.Create(sess.ID, "github", "Fix any failed check.", "sec", map[string]any{"conclusion": "failure"}, true, 0, time.Now())
	if err != nil {
		t.Fatal(err)
	}

	matched, err := svc.DeliverWebhook("github", sub.ID, map[string]string{"X-GitHub-Event": "check_suite"}, "sec", []byte(`{"conclusion":"failure","name":"android"}`))
	if err != nil {
		t.Fatalf("DeliverWebhook error: %v", err)
	}
	if matched != 1 {
		t.Fatalf("matched = %d, want 1", matched)
	}

	select {
	case gotPrompt := <-prov.prompts:
		if !strings.Contains(gotPrompt, "Fix any failed check.") || !strings.Contains(gotPrompt, "check_suite") || !strings.Contains(gotPrompt, "android") {
			t.Fatalf("unexpected wake prompt:\n%s", gotPrompt)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for webhook wake run")
	}

	// Wait for active run to settle so temp dir cleanup doesn't race SQLite writes.
	deadline := time.Now().Add(3 * time.Second)
	for svc.IsActive(sess.ID) && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}

	if gotSub, _ := store.Get(sub.ID); gotSub != nil {
		t.Fatalf("once subscription should be deleted after delivery, got %+v", gotSub)
	}
}
