package db

import (
	"encoding/json"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/types"
	"testing"
)

func newTestStorage(t *testing.T) *Storage {
	t.Helper()
	s, err := OpenStorage(OpenOptions{DBPath: ":memory:"})
	if err != nil {
		t.Fatalf("open storage: %v", err)
	}
	t.Cleanup(s.Close)
	return s
}

func TestProjectsCRUD(t *testing.T) {
	s := newTestStorage(t)

	created, err := createProject(s.globalDB, createProjectOptions{Name: "Console", Dir: "/tmp/console"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if created.ID == "" || created.Path != "/tmp/console" {
		t.Fatalf("unexpected project: %+v", created)
	}

	// Upsert on same dir replaces the name.
	updated, err := createProject(s.globalDB, createProjectOptions{Name: "Console v2", Dir: "/tmp/console"})
	if err != nil {
		t.Fatalf("upsert: %v", err)
	}
	if updated.ID != created.ID || updated.Name != "Console v2" {
		t.Fatalf("upsert mismatch: %+v vs %+v", updated, created)
	}

	byDir, err := projectByDir(s.globalDB, "/tmp/console")
	if err != nil || byDir.ID != created.ID {
		t.Fatalf("byDir: %v %+v", err, byDir)
	}

	list, err := listProjects(s.globalDB)
	if err != nil || len(list) != 1 {
		t.Fatalf("list: %v %d", err, len(list))
	}
}

func TestSessionLifecycle(t *testing.T) {
	s := newTestStorage(t)

	header, err := s.CreateSession(types.CreateSessionOptions{
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
	if err := s.AppendMessage(header.ID, msg); err != nil {
		t.Fatalf("append: %v", err)
	}

	loaded, err := s.LoadSession(header.ID, 0, 0)
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
	if err := s.AppendMessage(header.ID, msg); err != nil {
		t.Fatalf("re-append: %v", err)
	}
	loaded, _ = s.LoadSession(header.ID, 0, 0)
	if len(loaded.Messages) != 1 {
		t.Fatalf("duplicate not ignored: %d messages", len(loaded.Messages))
	}

	deleted, err := s.SoftDeleteSession(header.ID)
	if err != nil || !deleted {
		t.Fatalf("delete: %v %v", err, deleted)
	}
	if got, _ := s.LoadSession(header.ID, 0, 0); got != nil {
		t.Fatal("deleted session should not load")
	}
	if list, _ := s.ListSessions(0); len(list) != 0 {
		t.Fatal("deleted session should not list")
	}
}

func TestModelFavorites(t *testing.T) {
	s := newTestStorage(t)

	fav := types.ModelFavorite{Provider: "anthropic", ModelID: "claude-sonnet-4"}
	if err := s.SetModelFavorite(fav, true); err != nil {
		t.Fatalf("set: %v", err)
	}
	// Re-set is a no-op conflict insert.
	if err := s.SetModelFavorite(fav, true); err != nil {
		t.Fatalf("re-set: %v", err)
	}
	list, err := s.ListModelFavorites()
	if err != nil || len(list) != 1 {
		t.Fatalf("list: %v %d", err, len(list))
	}
	if err := s.SetModelFavorite(fav, false); err != nil {
		t.Fatalf("unset: %v", err)
	}
	if list, _ := s.ListModelFavorites(); len(list) != 0 {
		t.Fatal("favorite not removed")
	}
}

func TestDeleteProjectRemovesSessions(t *testing.T) {
	s := newTestStorage(t)

	proj, err := createProject(s.globalDB, createProjectOptions{Name: "P", Dir: "/tmp/p"})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	pid := proj.ID
	if _, err := s.CreateSession(types.CreateSessionOptions{
		Cwd: "/tmp/p", ModelID: "m", Provider: "anthropic", ProjectID: &pid,
	}); err != nil {
		t.Fatalf("create session: %v", err)
	}

	deleted, err := s.deleteProject(pid)
	if err != nil || !deleted {
		t.Fatalf("deleteProject: %v %v", err, deleted)
	}
	if list, _ := s.ListSessions(0); len(list) != 0 {
		t.Fatal("project session still listed")
	}
}
