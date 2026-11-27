// Sessions must refuse to point at a working directory that is no longer on
// disk — typically a project whose folder was deleted after registration.
// A session created on a ghost cwd used to survive the create and then fail
// every tool call and terminal spawn with low-signal errors.
package tests

import (
	"errors"
	"os"
	"testing"

	projectssvc "github.com/Adelodunpeter25/console/apps/server-go/internal/services/projects"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/services/session"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/types"
)

func TestCreateRejectsMissingCwd(t *testing.T) {
	sessions, _, _ := newScratchStore(t)

	_, err := sessions.Create(types.CreateSessionOptions{
		Cwd: "/nonexistent/console-ghost-dir",
	})
	if !errors.Is(err, session.ErrCwdMissing) {
		t.Fatalf("create on missing cwd: err = %v; want ErrCwdMissing", err)
	}
}

func TestCreateStillAllowsScratchOnMissingCwd(t *testing.T) {
	sessions, _, _ := newScratchStore(t)

	header, err := sessions.Create(types.CreateSessionOptions{ProjectNull: true})
	if err != nil {
		t.Fatalf("scratch create: %v", err)
	}
	if header.Cwd == "" {
		t.Fatal("scratch session has no sandboxed cwd")
	}
}

func TestUpdateCwdRejectsMissingDir(t *testing.T) {
	sessions, projects, _ := newScratchStore(t)

	header := projectSession(t, sessions, projects)

	err := sessions.UpdateCwd(header.ID, "/nonexistent/console-ghost-dir", header.ProjectID, true)
	if !errors.Is(err, session.ErrCwdMissing) {
		t.Fatalf("update to missing cwd: err = %v; want ErrCwdMissing", err)
	}
}

func TestUpdateCwdStillAllowsClearingToScratch(t *testing.T) {
	sessions, projects, _ := newScratchStore(t)

	header := projectSession(t, sessions, projects)

	// "No Folder" on the clients: explicit null project, no cwd — the
	// session re-roots into its sandboxed scratch dir.
	if err := sessions.UpdateCwd(header.ID, "", nil, true); err != nil {
		t.Fatalf("clear to scratch: %v", err)
	}
}

func TestProjectSessionAfterFolderDeleteCannotRecreate(t *testing.T) {
	sessions, projects, _ := newScratchStore(t)

	header := projectSession(t, sessions, projects)

	// Simulate the folder being deleted after registration, then a second
	// session opened on the same stale project row.
	if err := os.RemoveAll(header.Cwd); err != nil {
		t.Fatalf("remove dir: %v", err)
	}
	_, staleErr := sessions.Create(types.CreateSessionOptions{Cwd: header.Cwd})
	if !errors.Is(staleErr, session.ErrCwdMissing) {
		t.Fatalf("create on deleted project dir: err = %v; want ErrCwdMissing", staleErr)
	}
	if _, err := projects.Get(*header.ProjectID); err != nil {
		t.Fatalf("project row should survive the failed create: %v", err)
	}
}

func TestListPrunesProjectWithMissingDir(t *testing.T) {
	sessions, projects, _ := newScratchStore(t)

	dir := t.TempDir()
	project, err := projects.Create(projectssvc.CreateProjectOptions{Name: "P", Dir: dir})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	header, err := sessions.Create(types.CreateSessionOptions{Cwd: dir, ProjectID: &project.ID})
	if err != nil {
		t.Fatalf("create session: %v", err)
	}

	// The folder disappears after registration; the next list serves a
	// clean set, and the ghost project and its sessions are gone.
	if err := os.RemoveAll(dir); err != nil {
		t.Fatalf("remove dir: %v", err)
	}

	list, err := projects.List()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 0 {
		t.Fatalf("stale project still listed: %+v", list)
	}
	if _, err := projects.Get(project.ID); err == nil {
		t.Fatal("stale project row survived the prune")
	}
	if h, err := sessions.Header(header.ID); err != nil || h != nil {
		t.Fatalf("stale session survived the prune: %+v err=%v", h, err)
	}
}
