// All server tests live in this folder. DB + service coverage.
package tests

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/loop"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/db"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/run"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/services"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/services/session"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/types"
	"github.com/Adelodunpeter25/console/apps/server-go/tests/helpers"
)

func newTestManager(t *testing.T) (*db.DB, *services.SessionService, *services.ProjectService, *services.FavoriteService) {
	t.Helper()
	manager, err := db.Open(db.OpenOptions{Path: ":memory:"})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(manager.Close)
	return manager,
		services.NewSessionService(manager),
		services.NewProjectService(manager),
		services.NewFavoriteService(manager)
}

func TestProjectsCRUD(t *testing.T) {
	_, _, projects, _ := newTestManager(t)

	created, err := projects.Create(services.CreateProjectOptions{Name: "Console", Dir: "/tmp/console"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if created.ID == "" || created.Path != "/tmp/console" {
		t.Fatalf("unexpected project: %+v", created)
	}

	// Upsert on same dir replaces the name.
	updated, err := projects.Create(services.CreateProjectOptions{Name: "Console v2", Dir: "/tmp/console"})
	if err != nil {
		t.Fatalf("upsert: %v", err)
	}
	if updated.ID != created.ID || updated.Name != "Console v2" {
		t.Fatalf("upsert mismatch: %+v vs %+v", updated, created)
	}

	byDir, err := projects.GetByDir("/tmp/console")
	if err != nil || byDir.ID != created.ID {
		t.Fatalf("byDir: %v %+v", err, byDir)
	}

	list, err := projects.List()
	if err != nil || len(list) != 1 {
		t.Fatalf("list: %v %d", err, len(list))
	}
}

func TestSessionLifecycle(t *testing.T) {
	_, sessions, _, _ := newTestManager(t)

	header, err := sessions.Create(types.CreateSessionOptions{
		Cwd: "/tmp/console", ModelID: "claude-sonnet-4", Provider: "anthropic",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if header.Title != "New Session" || header.Status != "idle" {
		t.Fatalf("unexpected header: %+v", header)
	}

	// Messages persist to the per-session DB as content JSON; load returns
	// the stored object with createdAt injected (desktop shape).
	msg := types.AgentMessage{ID: "m1", Role: "user"}
	msg.Data, _ = json.Marshal(map[string]string{"role": "user", "text": "hello"})
	if err := sessions.AppendMessage(header.ID, msg); err != nil {
		t.Fatalf("append: %v", err)
	}

	loaded, err := sessions.Load(header.ID, 0, 0)
	if err != nil || loaded == nil {
		t.Fatalf("load: %v", err)
	}
	if len(loaded.Messages) != 1 {
		t.Fatalf("unexpected messages: %+v", loaded.Messages)
	}
	var decoded map[string]any
	if err := json.Unmarshal(loaded.Messages[0], &decoded); err != nil {
		t.Fatalf("message not JSON: %v", err)
	}
	if decoded["role"] != "user" || decoded["text"] != "hello" {
		t.Fatalf("unexpected message content: %v", decoded)
	}
	if _, ok := decoded["createdAt"]; !ok {
		t.Fatalf("createdAt not injected: %v", decoded)
	}
	if loaded.Header.MessageCount != 1 {
		t.Fatalf("message count = %d, want 1", loaded.Header.MessageCount)
	}

	// Duplicate ids are ignored.
	if err := sessions.AppendMessage(header.ID, msg); err != nil {
		t.Fatalf("re-append: %v", err)
	}
	loaded, _ = sessions.Load(header.ID, 0, 0)
	if len(loaded.Messages) != 1 {
		t.Fatalf("duplicate not ignored: %d messages", len(loaded.Messages))
	}

	deleted, err := sessions.SoftDelete(header.ID)
	if err != nil || !deleted {
		t.Fatalf("delete: %v %v", err, deleted)
	}
	if got, _ := sessions.Load(header.ID, 0, 0); got != nil {
		t.Fatal("deleted session should not load")
	}
	if list, _ := sessions.ListFiltered(session.ListFilter{}); len(list) != 0 {
		t.Fatal("deleted session should not list")
	}
}

func TestModelFavorites(t *testing.T) {
	_, _, _, favorites := newTestManager(t)

	fav := types.ModelFavorite{Provider: "anthropic", ModelID: "claude-sonnet-4"}
	if err := favorites.Set(fav, true); err != nil {
		t.Fatalf("set: %v", err)
	}
	// Re-set is a no-op conflict insert.
	if err := favorites.Set(fav, true); err != nil {
		t.Fatalf("re-set: %v", err)
	}
	list, err := favorites.List()
	if err != nil || len(list) != 1 {
		t.Fatalf("list: %v %d", err, len(list))
	}
	if err := favorites.Set(fav, false); err != nil {
		t.Fatalf("unset: %v", err)
	}
	if list, _ := favorites.List(); len(list) != 0 {
		t.Fatal("favorite not removed")
	}
}

func TestDeleteProjectRemovesSessions(t *testing.T) {
	_, sessions, projects, _ := newTestManager(t)

	proj, err := projects.Create(services.CreateProjectOptions{Name: "P", Dir: "/tmp/p"})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	pid := proj.ID
	if _, err := sessions.Create(types.CreateSessionOptions{
		Cwd: "/tmp/p", ModelID: "m", Provider: "anthropic", ProjectID: &pid,
	}); err != nil {
		t.Fatalf("create session: %v", err)
	}

	deleted, err := projects.Delete(pid)
	if err != nil || !deleted {
		t.Fatalf("delete: %v %v", err, deleted)
	}
	if list, _ := sessions.ListFiltered(session.ListFilter{}); len(list) != 0 {
		t.Fatal("project session still listed")
	}
}

func TestPurgeExpiredDeletedSessions(t *testing.T) {
	manager, sessions, _, _ := newTestManager(t)
	svc := run.NewService(sessions)

	mkSession := func() string {
		h, err := sessions.Create(types.CreateSessionOptions{Cwd: t.TempDir(), ModelID: "m", Provider: "p"})
		if err != nil {
			t.Fatal(err)
		}
		return h.ID
	}
	backdateDeleted := func(id string, age time.Duration) {
		old := time.Now().UnixMilli() - age.Milliseconds()
		if _, err := manager.Global().Exec(
			`UPDATE sessions SET deleted_at = ? WHERE id = ?`, old, id); err != nil {
			t.Fatal(err)
		}
	}

	oldID := mkSession()
	recentID := mkSession()
	if _, err := sessions.SoftDelete(oldID); err != nil {
		t.Fatal(err)
	}
	if _, err := sessions.SoftDelete(recentID); err != nil {
		t.Fatal(err)
	}
	backdateDeleted(oldID, run.DeletedSessionRetention+time.Hour)

	// An expired soft-deleted session with an active run is deferred.
	liveID := mkSession()
	entered := make(chan struct{}, 1)
	release := make(chan struct{})
	svc.Lookup = func(id string) (loop.Provider, error) {
		return &helpers.MockProvider{Turns: []func() []loop.Event{
			func() []loop.Event {
				entered <- struct{}{}
				<-release
				return []loop.Event{{Kind: loop.EventText, Text: "done"}}
			},
		}}, nil
	}
	hub, err := svc.StartRun(liveID, run.Prompt{Text: "slow", Provider: "mock", ModelID: "m"})
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-entered:
	case <-time.After(10 * time.Second):
		t.Fatal("run never started")
	}
	if _, err := sessions.SoftDelete(liveID); err != nil {
		t.Fatal(err)
	}
	backdateDeleted(liveID, run.DeletedSessionRetention+time.Hour)

	purged := svc.PurgeExpiredDeletedSessions()
	if len(purged) != 1 || purged[0] != oldID {
		t.Fatalf("purged = %v, want [%s]", purged, oldID)
	}
	if got, _ := sessions.Load(oldID, 0, 0); got != nil {
		t.Fatal("expired session should be gone")
	}
	if got, _ := sessions.Load(recentID, 0, 0); got != nil {
		t.Fatal("recently deleted session must survive")
	}
	if got, _ := sessions.Load(liveID, 0, 0); got != nil {
		t.Fatal("active session must be deferred")
	}

	close(release)
	select {
	case <-hub.Done():
	case <-time.After(15 * time.Second):
		t.Fatal("run did not settle")
	}
	purged = svc.PurgeExpiredDeletedSessions()
	if len(purged) != 1 || purged[0] != liveID {
		t.Fatalf("second purge = %v, want [%s]", purged, liveID)
	}
}

// TestOpsOnMissingSessionNoOp mirrors the TS session-*.ts convention: an
// operation on a session id with no row in the global index silently
// no-ops (matching `if (projectId === undefined) return;`) instead of
// leaking a raw "sql: no rows in result set" as a turn-ending agent error.
func TestOpsOnMissingSessionNoOp(t *testing.T) {
	_, sessions, _, _ := newTestManager(t)
	missing := "does-not-exist"

	if err := sessions.AppendMessage(missing, types.AgentMessage{ID: "m1", Role: "user", Data: json.RawMessage(`{"role":"user","content":"hi"}`)}); err != nil {
		t.Fatalf("AppendMessage on missing session must no-op, got: %v", err)
	}
	if err := sessions.ReplaceMessages(missing, nil); err != nil {
		t.Fatalf("ReplaceMessages on missing session must no-op, got: %v", err)
	}
	if items, err := sessions.GetSessionTodos(missing); err != nil || len(items) != 0 {
		t.Fatalf("GetSessionTodos on missing session: %+v %v", items, err)
	}
	if err := sessions.SaveSessionTodos(missing, []types.TodoItem{{ID: 1, Content: "x", Status: "pending"}}); err != nil {
		t.Fatalf("SaveSessionTodos on missing session must no-op, got: %v", err)
	}
	if changes, err := sessions.GetSessionFileChanges(missing, -1); err != nil || len(changes) != 0 {
		t.Fatalf("GetSessionFileChanges on missing session: %+v %v", changes, err)
	}
	if err := sessions.RecordFileChange(missing, types.SessionFileChange{Path: "a.go", TurnIndex: 0, Status: "modified"}); err != nil {
		t.Fatalf("RecordFileChange on missing session must no-op, got: %v", err)
	}
	if err := sessions.UpdateTitle(missing, "New title"); err != nil {
		t.Fatalf("UpdateTitle on missing session must no-op, got: %v", err)
	}
	if qp, err := sessions.GetQueuedPrompt(missing); err != nil || qp != nil {
		t.Fatalf("GetQueuedPrompt on missing session: %+v %v", qp, err)
	}
	if subs, err := sessions.GetSubagents(missing); err != nil || len(subs) != 0 {
		t.Fatalf("GetSubagents on missing session: %+v %v", subs, err)
	}
}
