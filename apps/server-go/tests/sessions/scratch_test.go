// Scratchpad sessions: the sandboxed working dir must exist whenever a
// session points at one — on create (explicit-null projectId) and on the
// update path a client uses to clear the project ("No Folder"). A missing
// dir is what makes PtyManager.Spawn reject the terminal with
// "working directory does not exist".
//
// File-backed store under a temp CONSOLE_STORAGE_DIR so Create's scratch
// root and the manager's storage dir are the same tree.
package tests

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/db"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/services"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/types"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/utils"
)

func newScratchStore(t *testing.T) (*services.SessionService, *services.ProjectService, string) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("CONSOLE_STORAGE_DIR", dir)
	manager, err := db.Open(db.OpenOptions{Path: filepath.Join(dir, "console-global.db")})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(manager.Close)
	if manager.StorageDir() != dir {
		t.Fatalf("storage dir = %q; want %q", manager.StorageDir(), dir)
	}
	return services.NewSessionService(manager), services.NewProjectService(manager), dir
}

// projectSession creates a session linked to a registered project dir.
func projectSession(t *testing.T, sessions *services.SessionService, projects *services.ProjectService) types.SessionHeader {
	t.Helper()
	dir := t.TempDir()
	project, err := projects.Create(services.CreateProjectOptions{Name: "P", Dir: dir})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	header, err := sessions.Create(types.CreateSessionOptions{Cwd: dir, ProjectID: &project.ID})
	if err != nil {
		t.Fatalf("create project session: %v", err)
	}
	if header.ProjectID == nil {
		t.Fatal("project session should carry a project link")
	}
	return header
}

func scratchDirFor(storageDir, sessionID string) string {
	return filepath.Join(storageDir, "scratch", sessionID)
}

func assertDir(t *testing.T, path string, want bool) {
	t.Helper()
	st, err := os.Stat(path)
	if err != nil {
		if want && os.IsNotExist(err) {
			t.Fatalf("scratch dir missing: %s", path)
		}
		if !want && !os.IsNotExist(err) {
			t.Fatalf("stat %s: %v", path, err)
		}
		return
	}
	if !st.IsDir() {
		t.Fatalf("scratch path is not a directory: %s", path)
	}
	if !want {
		t.Fatalf("scratch dir should not exist: %s", path)
	}
}

// The create path mints the id, derives <scratch>/<id> and mkdirs it.
func TestScratchCreateMakesSandboxDir(t *testing.T) {
	sessions, _, storageDir := newScratchStore(t)

	header, err := sessions.Create(types.CreateSessionOptions{ProjectNull: true})
	if err != nil {
		t.Fatalf("create scratchpad: %v", err)
	}
	if header.ProjectID != nil {
		t.Fatalf("projectId = %q; want nil", *header.ProjectID)
	}
	want := scratchDirFor(storageDir, header.ID)
	if header.Cwd != want {
		t.Fatalf("cwd = %q; want %q", header.Cwd, want)
	}
	assertDir(t, header.Cwd, true)
}

// Clearing the project on an existing session (desktop sends an explicit
// null) must drop the project link and make the client-supplied scratch
// dir — the regression that left terminals unable to spawn.
func TestScratchUpdateClearsProjectAndMakesDir(t *testing.T) {
	sessions, projects, storageDir := newScratchStore(t)
	header := projectSession(t, sessions, projects)

	// The client pre-computes the path it wants; the server owns making it.
	target := scratchDirFor(storageDir, header.ID)
	assertDir(t, target, false)

	if err := sessions.UpdateCwd(header.ID, target, nil, true); err != nil {
		t.Fatalf("update cwd to scratch: %v", err)
	}
	assertDir(t, target, true)

	after, err := sessions.Header(header.ID)
	if err != nil {
		t.Fatalf("header: %v", err)
	}
	if after.Cwd != target {
		t.Fatalf("cwd = %q; want %q", after.Cwd, target)
	}
	if after.ProjectID != nil {
		t.Fatalf("projectId = %q; want nil after explicit null", *after.ProjectID)
	}
}

// An omitted projectId keeps the current link.
func TestScratchUpdateOmittedProjectKeepsLink(t *testing.T) {
	sessions, projects, _ := newScratchStore(t)
	header := projectSession(t, sessions, projects)

	moved := t.TempDir()
	if err := sessions.UpdateCwd(header.ID, moved, nil, false); err != nil {
		t.Fatalf("update cwd: %v", err)
	}
	after, err := sessions.Header(header.ID)
	if err != nil {
		t.Fatalf("header: %v", err)
	}
	if after.ProjectID == nil || *after.ProjectID != *header.ProjectID {
		t.Fatalf("projectId = %v; want %q kept", after.ProjectID, *header.ProjectID)
	}
	if after.Cwd != moved {
		t.Fatalf("cwd = %q; want %q", after.Cwd, moved)
	}
}

// "scratch" is the wire alias for no project and must clear the link too.
func TestScratchUpdateViaAliasClearsProject(t *testing.T) {
	sessions, projects, storageDir := newScratchStore(t)
	header := projectSession(t, sessions, projects)

	target := scratchDirFor(storageDir, header.ID)
	alias := "scratch"
	if err := sessions.UpdateCwd(header.ID, target, &alias, true); err != nil {
		t.Fatalf("update cwd to scratch alias: %v", err)
	}
	assertDir(t, target, true)
	after, err := sessions.Header(header.ID)
	if err != nil {
		t.Fatalf("header: %v", err)
	}
	if after.ProjectID != nil {
		t.Fatalf("projectId = %q; want nil", *after.ProjectID)
	}
}

// A scratchpad cwd outside the store's scratch root is never created: an
// update must not mkdir arbitrary client paths.
func TestScratchUpdateSkipsForeignPaths(t *testing.T) {
	sessions, _, _ := newScratchStore(t)

	header, err := sessions.Create(types.CreateSessionOptions{ProjectNull: true})
	if err != nil {
		t.Fatalf("create scratchpad: %v", err)
	}

	// A cwd that only looks nested via ".." is not scratch-owned either.
	outside := filepath.Join(t.TempDir(), "not-scratch")
	escape := filepath.Join(utils.ConsoleStorageDir(), "scratch", "..", "..", "escaped")
	for _, path := range []string{outside, escape} {
		if err := sessions.UpdateCwd(header.ID, path, nil, true); err != nil {
			t.Fatalf("update cwd %s: %v", path, err)
		}
		assertDir(t, path, false)
	}
}

// The shared scratch root is not one session's sandbox: pointing a session
// at it must not create it.
func TestScratchUpdateSkipsScratchRoot(t *testing.T) {
	sessions, projects, storageDir := newScratchStore(t)
	header := projectSession(t, sessions, projects)

	root := filepath.Join(storageDir, "scratch")
	assertDir(t, root, false)

	if err := sessions.UpdateCwd(header.ID, root, nil, true); err != nil {
		t.Fatalf("update cwd to scratch root: %v", err)
	}
	assertDir(t, root, false)
}

// Permanent delete still removes the sandbox dir it owns.
func TestScratchPermanentDeleteRemovesDir(t *testing.T) {
	sessions, _, _ := newScratchStore(t)

	header, err := sessions.Create(types.CreateSessionOptions{ProjectNull: true})
	if err != nil {
		t.Fatalf("create scratchpad: %v", err)
	}
	assertDir(t, header.Cwd, true)

	if _, err := sessions.SoftDelete(header.ID); err != nil {
		t.Fatalf("soft delete: %v", err)
	}
	assertDir(t, header.Cwd, true)

	if _, err := sessions.PermanentDelete(header.ID); err != nil {
		t.Fatalf("permanent delete: %v", err)
	}
	assertDir(t, header.Cwd, false)
}