// Worktree session integration: create-with-worktree, permanent-delete
// cleanup, dirty blocking. Facade level with a :memory: DB; HOME is
// redirected to temp so DefaultRoot never touches the real home dir.
package tests

import (
	"database/sql"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/db"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/services"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/services/session"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/types"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/utils"
)

func openWorktreeManager(t *testing.T) *db.DB {
	t.Helper()
	manager, err := db.Open(db.OpenOptions{Path: ":memory:"})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(manager.Close)
	return manager
}

func initWorktreeRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	runGit(t, dir, "init", "-b", "main")
	runGit(t, dir, "-c", "user.name=t", "-c", "user.email=t@t", "commit", "--allow-empty", "-m", "init")
	return dir
}

func runGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_CONFIG_NOSYSTEM=1")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, out)
	}
	return string(out)
}

func isolateHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	return home
}

func createWorkedSession(t *testing.T, sessions *services.SessionService, repo, title string) types.SessionHeader {
	t.Helper()
	header, err := sessions.Create(types.CreateSessionOptions{
		Title:    title,
		Cwd:      repo,
		Worktree: &types.CreateWorktreeSpec{},
	})
	if err != nil {
		t.Fatalf("Create with worktree: %v", err)
	}
	return header
}

func TestCreateWithWorktree(t *testing.T) {
	home := isolateHome(t)
	manager := openWorktreeManager(t)
	sessions := services.NewSessionService(manager)
	repo := initWorktreeRepo(t)

	header := createWorkedSession(t, sessions, repo, "Fix login bug")
	if header.Worktree == nil {
		t.Fatalf("header missing worktree: %+v", header)
	}
	wantRoot := filepath.Join(home, "console", "worktrees")
	if filepath.Dir(header.Worktree.Path) != wantRoot {
		t.Fatalf("worktree path %q not under %q", header.Worktree.Path, wantRoot)
	}
	if header.Cwd != header.Worktree.Path {
		t.Fatalf("cwd %q != worktree path %q", header.Cwd, header.Worktree.Path)
	}
	if branch := header.Worktree.Branch; !strings.HasPrefix(branch, "fix-login-bug-") || len(branch) != len("fix-login-bug-")+6 {
		t.Fatalf("unexpected branch %q", branch)
	}
	if header.Worktree.Repo != repo {
		t.Fatalf("repo %q != %q", header.Worktree.Repo, repo)
	}
	// Branch exists on the repo; main checkout untouched.
	branches := services.NewGitService().ListBranches(repo)
	found := false
	for _, b := range branches.Branches {
		if b.Name == header.Worktree.Branch {
			found = true
		}
	}
	if !found {
		t.Fatalf("branch %q missing: %+v", header.Worktree.Branch, branches)
	}
	// Header round-trips through Load with the worktree attached.
	loaded, err := sessions.Load(header.ID, 0, 0)
	if err != nil || loaded == nil || loaded.Header.Worktree == nil {
		t.Fatalf("Load worktree = %+v, %v", loaded, err)
	}
}

func TestCreateWithExplicitBranch(t *testing.T) {
	isolateHome(t)
	manager := openWorktreeManager(t)
	sessions := services.NewSessionService(manager)
	repo := initWorktreeRepo(t)

	header, err := sessions.Create(types.CreateSessionOptions{
		Title:    "whatever",
		Cwd:      repo,
		Worktree: &types.CreateWorktreeSpec{Branch: "my-feature"},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if header.Worktree == nil || header.Worktree.Branch != "my-feature" {
		t.Fatalf("branch not honored: %+v", header.Worktree)
	}
}

func TestCreateWorktreeNonRepo(t *testing.T) {
	isolateHome(t)
	manager := openWorktreeManager(t)
	sessions := services.NewSessionService(manager)

	_, err := sessions.Create(types.CreateSessionOptions{
		Title:    "nope",
		Cwd:      t.TempDir(),
		Worktree: &types.CreateWorktreeSpec{},
	})
	if !errors.Is(err, services.ErrNotGitRepo) {
		t.Fatalf("non-repo err = %v; want ErrNotGitRepo", err)
	}
	list, err := sessions.ListFiltered(session.ListFilter{})
	if err != nil || len(list) != 0 {
		t.Fatalf("session row leaked: %v %+v", err, list)
	}
}

func TestCreateWorktreeScratchpad(t *testing.T) {
	isolateHome(t)
	manager := openWorktreeManager(t)
	sessions := services.NewSessionService(manager)

	_, err := sessions.Create(types.CreateSessionOptions{
		Title:       "scratch",
		ProjectNull: true,
		Worktree:    &types.CreateWorktreeSpec{},
	})
	if !errors.Is(err, services.ErrWorktreeScratchpad) {
		t.Fatalf("scratchpad err = %v; want ErrWorktreeScratchpad", err)
	}
}

func TestPermanentDeleteCleansWorktree(t *testing.T) {
	isolateHome(t)
	manager := openWorktreeManager(t)
	sessions := services.NewSessionService(manager)
	repo := initWorktreeRepo(t)

	header := createWorkedSession(t, sessions, repo, "temp work")
	branch := header.Worktree.Branch
	path := header.Worktree.Path

	if ok, err := sessions.SoftDelete(header.ID); err != nil || !ok {
		t.Fatalf("SoftDelete = %v, %v", ok, err)
	}
	deleted, err := sessions.PermanentDelete(header.ID)
	if err != nil || !deleted {
		t.Fatalf("PermanentDelete = %v, %v", deleted, err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("worktree dir still exists: %s", path)
	}
	// Branch stays (leave-always v1 policy).
	branches := services.NewGitService().ListBranches(repo)
	stillThere := false
	for _, b := range branches.Branches {
		if b.Name == branch {
			stillThere = true
		}
	}
	if !stillThere {
		t.Fatalf("branch %q was deleted, want leave-always", branch)
	}
	if h, _ := sessions.Header(header.ID); h != nil {
		t.Fatalf("session row still present")
	}
}

func TestPermanentDeleteBlockedWhenDirty(t *testing.T) {
	isolateHome(t)
	manager := openWorktreeManager(t)
	sessions := services.NewSessionService(manager)
	repo := initWorktreeRepo(t)

	header := createWorkedSession(t, sessions, repo, "dirty work")
	if err := os.WriteFile(filepath.Join(header.Worktree.Path, "wip.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if ok, err := sessions.SoftDelete(header.ID); err != nil || !ok {
		t.Fatalf("SoftDelete = %v, %v", ok, err)
	}
	_, err := sessions.PermanentDelete(header.ID)
	if !errors.Is(err, services.ErrWorktreeDirty) {
		t.Fatalf("dirty delete err = %v; want ErrWorktreeDirty", err)
	}
	// Session row kept (still soft-deleted, still owning its worktree).
	owned, err := sessions.OwnedWorktreePaths()
	if err != nil {
		t.Fatal(err)
	}
	kept := false
	for _, p := range owned {
		if p == header.Worktree.Path {
			kept = true
		}
	}
	if !kept {
		t.Fatalf("session row removed despite blocked delete")
	}
	if _, err := os.Stat(filepath.Join(header.Worktree.Path, "wip.txt")); err != nil {
		t.Fatalf("work lost: %v", err)
	}
}

func TestPermanentDeletePlainSession(t *testing.T) {
	isolateHome(t)
	manager := openWorktreeManager(t)
	sessions := services.NewSessionService(manager)
	repo := initWorktreeRepo(t)

	header, err := sessions.Create(types.CreateSessionOptions{Title: "plain", Cwd: repo})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if header.Worktree != nil {
		t.Fatalf("plain session has worktree: %+v", header.Worktree)
	}
	if ok, _ := sessions.SoftDelete(header.ID); !ok {
		t.Fatalf("SoftDelete failed")
	}
	deleted, err := sessions.PermanentDelete(header.ID)
	if err != nil || !deleted {
		t.Fatalf("PermanentDelete = %v, %v", deleted, err)
	}
}

func TestAttachWorktreeToEmptySession(t *testing.T) {
	isolateHome(t)
	manager := openWorktreeManager(t)
	sessions := services.NewSessionService(manager)
	repo := initWorktreeRepo(t)

	header, err := sessions.Create(types.CreateSessionOptions{Title: "Fix login bug", Cwd: repo})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if header.Worktree != nil {
		t.Fatalf("plain session already has worktree: %+v", header.Worktree)
	}

	updated, err := sessions.AttachWorktree(header.ID, &types.CreateWorktreeSpec{})
	if err != nil {
		t.Fatalf("AttachWorktree: %v", err)
	}
	if updated.ID != header.ID {
		t.Fatalf("AttachWorktree changed session id: %q != %q", updated.ID, header.ID)
	}
	if updated.Worktree == nil {
		t.Fatalf("updated header missing worktree: %+v", updated)
	}
	if updated.Cwd != updated.Worktree.Path {
		t.Fatalf("cwd %q != worktree path %q", updated.Cwd, updated.Worktree.Path)
	}
	if updated.Worktree.Repo != repo {
		t.Fatalf("repo %q != %q", updated.Worktree.Repo, repo)
	}

	// No second session row was created.
	list, err := sessions.ListFiltered(session.ListFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 {
		t.Fatalf("session count = %d; want 1: %+v", len(list), list)
	}
}

func TestAttachWorktreeRejectsSessionWithMessages(t *testing.T) {
	isolateHome(t)
	manager := openWorktreeManager(t)
	sessions := services.NewSessionService(manager)
	repo := initWorktreeRepo(t)

	header, err := sessions.Create(types.CreateSessionOptions{Title: "has messages", Cwd: repo})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := sessions.AppendMessage(header.ID, types.AgentMessage{}); err != nil {
		t.Fatalf("AppendMessage: %v", err)
	}

	_, err = sessions.AttachWorktree(header.ID, &types.CreateWorktreeSpec{})
	if !errors.Is(err, services.ErrWorktreeSessionHasMessages) {
		t.Fatalf("err = %v; want ErrWorktreeSessionHasMessages", err)
	}

	reloaded, err := sessions.Header(header.ID)
	if err != nil || reloaded == nil {
		t.Fatalf("Header: %v, %v", reloaded, err)
	}
	if reloaded.Worktree != nil || reloaded.Cwd != repo {
		t.Fatalf("session mutated despite rejection: %+v", reloaded)
	}
}

func TestAttachWorktreeRejectsAlreadyOwned(t *testing.T) {
	isolateHome(t)
	manager := openWorktreeManager(t)
	sessions := services.NewSessionService(manager)
	repo := initWorktreeRepo(t)

	header := createWorkedSession(t, sessions, repo, "already worktree")

	_, err := sessions.AttachWorktree(header.ID, &types.CreateWorktreeSpec{})
	if !errors.Is(err, services.ErrWorktreeAlreadyOwned) {
		t.Fatalf("err = %v; want ErrWorktreeAlreadyOwned", err)
	}
}

// TestWorktreesDirCentralized verifies the worktree root goes through the
// central app paths file: prod default, dev variant, explicit override.
func TestWorktreesDirCentralized(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("CONSOLE_ENV", "")
	t.Setenv("CONSOLE_WORKTREES_DIR", "")

	if got := utils.WorktreesDir(); got != filepath.Join(home, "console", "worktrees") {
		t.Fatalf("prod root = %q", got)
	}
	t.Setenv("CONSOLE_ENV", "dev")
	if got := utils.WorktreesDir(); got != filepath.Join(home, "console-dev", "worktrees") {
		t.Fatalf("dev root = %q", got)
	}
	t.Setenv("CONSOLE_WORKTREES_DIR", filepath.Join(home, "custom-wt"))
	if got := utils.WorktreesDir(); got != filepath.Join(home, "custom-wt") {
		t.Fatalf("override root = %q", got)
	}
	root, err := services.DefaultRoot()
	if err != nil || root != utils.WorktreesDir() {
		t.Fatalf("DefaultRoot = %q, %v; want %q", root, err, utils.WorktreesDir())
	}
}

// TestWorktreeColumnMigration opens a pre-worktree global DB file (old
// sessions schema) and verifies the additive migration adds the ownership
// columns with old rows intact and NULL worktrees.
func TestWorktreeColumnMigration(t *testing.T) {
	isolateHome(t)
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "global.db")

	raw, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	_, err = raw.Exec(`CREATE TABLE sessions (
		id TEXT PRIMARY KEY, title TEXT NOT NULL, cwd TEXT NOT NULL,
		project_id TEXT, model_id TEXT NOT NULL, provider TEXT NOT NULL,
		message_count INTEGER NOT NULL DEFAULT 0, status TEXT NOT NULL DEFAULT 'idle',
		approval_mode TEXT NOT NULL DEFAULT 'always-ask',
		created_at INTEGER NOT NULL, updated_at INTEGER NOT NULL, deleted_at INTEGER
	)`)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := raw.Exec(`INSERT INTO sessions
		(id, title, cwd, model_id, provider, created_at, updated_at)
		VALUES ('old-1', 'old', '/tmp', 'm', 'p', 1, 1)`); err != nil {
		t.Fatal(err)
	}
	if err := raw.Close(); err != nil {
		t.Fatal(err)
	}

	manager, err := db.Open(db.OpenOptions{Path: dbPath})
	if err != nil {
		t.Fatalf("open old db: %v", err)
	}
	t.Cleanup(manager.Close)

	sessions := services.NewSessionService(manager)
	list, err := sessions.ListFiltered(session.ListFilter{})
	if err != nil || len(list) != 1 || list[0].Worktree != nil {
		t.Fatalf("old row after migration: %+v, %v", list, err)
	}
	// New writes use the migrated columns.
	repo := initWorktreeRepo(t)
	header := createWorkedSession(t, sessions, repo, "after migration")
	if header.Worktree == nil {
		t.Fatalf("create after migration lost worktree")
	}
}
