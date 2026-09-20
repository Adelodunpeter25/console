// All server tests live in this folder. DB + service coverage.
package tests

import (
	"encoding/json"
	"testing"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/db"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/services"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/types"
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

	// Messages persist to the per-session DB with role and raw JSON.
	msg := types.AgentMessage{ID: "m1", Role: "user"}
	msg.Data, _ = json.Marshal(map[string]string{"text": "hello"})
	if err := sessions.AppendMessage(header.ID, msg); err != nil {
		t.Fatalf("append: %v", err)
	}

	loaded, err := sessions.Load(header.ID, 0, 0)
	if err != nil || loaded == nil {
		t.Fatalf("load: %v", err)
	}
	if len(loaded.Messages) != 1 || loaded.Messages[0].Role != "user" {
		t.Fatalf("unexpected messages: %+v", loaded.Messages)
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
	if list, _ := sessions.List(0); len(list) != 0 {
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
	if list, _ := sessions.List(0); len(list) != 0 {
		t.Fatal("project session still listed")
	}
}
