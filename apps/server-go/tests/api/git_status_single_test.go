// GetGitStatus uses one status call plus one combined numstat; these cover the
// branch-header parsing and the staged+unstaged numstat merge it relies on.
package tests

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/services"
)

func gitIn(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %s", args, out)
	}
}

func TestGitStatusStagedAndUnstagedNumstat(t *testing.T) {
	repo := gitRepo(t)
	write := func(name, body string) {
		if err := os.WriteFile(filepath.Join(repo, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	// Committed file that is then renamed (staged rename).
	write("old.txt", "x\n")
	gitIn(t, repo, "add", "old.txt")
	gitIn(t, repo, "commit", "-m", "old")
	gitIn(t, repo, "mv", "old.txt", "renamed.txt")
	// Staged edit, then a further unstaged edit on top of it.
	write("tracked.txt", "one\ntwo\n")
	gitIn(t, repo, "add", "tracked.txt")
	write("tracked.txt", "one\ntwo\nthree\n")

	status := services.NewGitService().GetGitStatus(repo)
	if status.Branch != "main" {
		t.Fatalf("branch: %q", status.Branch)
	}
	got := map[string]int64{}
	for _, f := range status.Files {
		got[filepath.Base(f.Path)] = f.Additions
	}
	if got["tracked.txt"] != 2 {
		t.Fatalf("tracked.txt additions should merge staged+unstaged to 2: %+v", status.Files)
	}
	if _, ok := got["renamed.txt"]; !ok {
		t.Fatalf("rename missing: %+v", status.Files)
	}
}

func TestGitStatusBranchHeaderForms(t *testing.T) {
	git := services.NewGitService()

	fresh := t.TempDir()
	gitIn(t, fresh, "init", "-b", "trunk")
	if b := git.GetGitStatus(fresh).Branch; b != "trunk" {
		t.Fatalf("no-commits branch: %q", b)
	}

	repo := gitRepo(t)
	gitIn(t, repo, "checkout", "-b", "feature/x")
	if b := git.GetGitStatus(repo).Branch; b != "feature/x" {
		t.Fatalf("slash branch: %q", b)
	}
	gitIn(t, repo, "checkout", "--detach")
	if b := git.GetGitStatus(repo).Branch; b != "HEAD" {
		t.Fatalf("detached branch: %q", b)
	}
}
