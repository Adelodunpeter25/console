// WorktreeService coverage: add/list/remove/prune, dirty refusal,
// unborn-HEAD refusal. Self-contained — no session or route wiring.
package tests

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/services"
)

func git(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"GIT_CONFIG_NOSYSTEM=1",
		"HOME="+t.TempDir(),
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, out)
	}
	return string(out)
}

func initRepo(t *testing.T, committed bool) string {
	t.Helper()
	dir := t.TempDir()
	git(t, dir, "init", "-b", "main")
	git(t, dir, "-c", "user.name=t", "-c", "user.email=t@t", "commit", "--allow-empty", "-m", "init")
	if !committed {
		// fresh repo with no commits: reset back to unborn HEAD
		git(t, dir, "update-ref", "-d", "HEAD")
	}
	return dir
}

func TestWorktreeAddListRemove(t *testing.T) {
	svc := services.NewWorktreeService()
	repo := initRepo(t, true)
	// git reports real paths; TempDir on macOS goes through /var -> /private/var.
	tmp, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	wt := filepath.Join(tmp, "wt")

	if err := svc.WorktreeAdd(repo, wt, "feature-x"); err != nil {
		t.Fatalf("WorktreeAdd: %v", err)
	}
	list, err := svc.WorktreeList(repo)
	if err != nil {
		t.Fatalf("WorktreeList: %v", err)
	}
	found := false
	for _, w := range list {
		if w.Path == wt && w.Branch == "feature-x" {
			found = true
		}
	}
	if !found {
		t.Fatalf("added worktree missing from list: %+v", list)
	}
	if err := svc.WorktreeRemove(repo, wt, false); err != nil {
		t.Fatalf("WorktreeRemove clean: %v", err)
	}
	if _, err := os.Stat(wt); !os.IsNotExist(err) {
		t.Fatalf("worktree dir still exists after remove")
	}
}

func TestWorktreeRemoveRefusesDirty(t *testing.T) {
	svc := services.NewWorktreeService()
	repo := initRepo(t, true)
	wt := filepath.Join(t.TempDir(), "wt")
	if err := svc.WorktreeAdd(repo, wt, "dirty-branch"); err != nil {
		t.Fatalf("WorktreeAdd: %v", err)
	}
	// untracked file counts as dirty
	if err := os.WriteFile(filepath.Join(wt, "half-written.txt"), []byte("wip"), 0o644); err != nil {
		t.Fatal(err)
	}
	dirty, err := svc.IsDirty(wt)
	if err != nil || !dirty {
		t.Fatalf("IsDirty = %v, %v; want true, nil", dirty, err)
	}
	if err := svc.WorktreeRemove(repo, wt, false); !errors.Is(err, services.ErrWorktreeDirty) {
		t.Fatalf("remove dirty = %v; want ErrWorktreeDirty", err)
	}
	if err := svc.WorktreeRemove(repo, wt, true); err != nil {
		t.Fatalf("force remove dirty: %v", err)
	}
}

func TestWorktreeAddRefusesUnbornHEAD(t *testing.T) {
	svc := services.NewWorktreeService()
	repo := initRepo(t, false)
	wt := filepath.Join(t.TempDir(), "wt")
	if err := svc.WorktreeAdd(repo, wt, "nope"); !errors.Is(err, services.ErrUnbornHEAD) {
		t.Fatalf("add on unborn HEAD = %v; want ErrUnbornHEAD", err)
	}
}

func TestWorktreePrune(t *testing.T) {
	svc := services.NewWorktreeService()
	repo := initRepo(t, true)
	if err := svc.WorktreePrune(repo); err != nil {
		t.Fatalf("WorktreePrune: %v", err)
	}
}
