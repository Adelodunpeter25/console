// FS and Git service coverage.
package tests

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/services"
)

func TestBrowseDirectory(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "a.txt"), []byte("hi"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".hidden"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	fs := services.NewFsService()

	result, err := fs.BrowseDirectory(root, false)
	if err != nil {
		t.Fatalf("browse: %v", err)
	}
	if result.CurrentPath != root || result.ParentPath == nil {
		t.Fatalf("unexpected browse: %+v", result)
	}
	if len(result.Entries) != 2 {
		t.Fatalf("hidden file leaked: %+v", result.Entries)
	}
	if result.Entries[0].Name != "sub" || !result.Entries[0].IsDir {
		t.Fatalf("dirs should sort first: %+v", result.Entries)
	}
}

func TestFilePreviewGates(t *testing.T) {
	root := t.TempDir()
	fs := services.NewFsService()

	if err := os.WriteFile(filepath.Join(root, "big.txt"), make([]byte, 600*1024), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := fs.ReadFileContentWithMeta(filepath.Join(root, "big.txt"), 0, 0); err == nil {
		t.Fatal("oversized file should be blocked")
	} else if blocked, ok := err.(*services.PreviewBlocked); !ok || blocked.Code != "FILE_TOO_LARGE" || blocked.Status != 413 {
		t.Fatalf("wrong error: %v", err)
	}

	if err := os.WriteFile(filepath.Join(root, "pnpm-lock.yaml"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := fs.ReadFileContentWithMeta(filepath.Join(root, "pnpm-lock.yaml"), 0, 0); err == nil {
		t.Fatal("lockfile should be blocked")
	}

	if err := os.WriteFile(filepath.Join(root, "ok.txt"), []byte("l1\nl2\nl3"), 0o644); err != nil {
		t.Fatal(err)
	}
	meta, err := fs.ReadFileContentWithMeta(filepath.Join(root, "ok.txt"), 2, 3)
	if err != nil || meta.Content != "l2\nl3" {
		t.Fatalf("slice: %v %q", err, meta.Content)
	}
}

func TestWriteReadTree(t *testing.T) {
	root := t.TempDir()
	fs := services.NewFsService()

	if _, err := fs.WriteFileContent(filepath.Join(root, "nested", "f.txt"), "hello"); err != nil {
		t.Fatalf("write: %v", err)
	}
	tree, err := fs.GetDirectoryTree(root, 3, true)
	if err != nil {
		t.Fatalf("tree: %v", err)
	}
	if !contains(tree, "f.txt") || !contains(tree, "nested/") {
		t.Fatalf("tree missing entries:\n%s", tree)
	}
}

func contains(haystack, needle string) bool {
	return len(haystack) >= len(needle) && (haystack == needle || indexOf(haystack, needle) >= 0)
}

func indexOf(h, n string) int {
	for i := 0; i+len(n) <= len(h); i++ {
		if h[i:i+len(n)] == n {
			return i
		}
	}
	return -1
}

func gitRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	run := func(args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %s", args, out)
		}
	}
	run("init", "-b", "main")
	run("config", "user.email", "t@t")
	run("config", "user.name", "t")
	if err := os.WriteFile(filepath.Join(dir, "tracked.txt"), []byte("one\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run("add", ".")
	run("commit", "-m", "init")
	return dir
}

func TestGitStatusAndBranches(t *testing.T) {
	repo := gitRepo(t)
	if err := os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte("one\ntwo\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "new.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	git := services.NewGitService()

	status := git.GetGitStatus(repo)
	if status.Branch != "main" || status.Clean {
		t.Fatalf("unexpected status: %+v", status)
	}
	var sawNew, sawTracked bool
	for _, f := range status.Files {
		switch filepath.Base(f.Path) {
		case "new.txt":
			sawNew = true
		case "tracked.txt":
			sawTracked = true
			if f.Additions != 1 {
				t.Fatalf("numstat: %+v", f)
			}
		}
	}
	if !sawNew || !sawTracked {
		t.Fatalf("missing files in status: %+v", status.Files)
	}

	branches := git.ListBranches(repo)
	if !branches.IsGitRepository || len(branches.Branches) != 1 || !branches.Branches[0].IsCurrent {
		t.Fatalf("branches: %+v", branches)
	}

	if _, err := git.GetDiff(repo, ""); err != nil {
		t.Fatalf("diff: %v", err)
	}
}

func TestGitCheckout(t *testing.T) {
	repo := gitRepo(t)
	git := services.NewGitService()
	run := func(args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = repo
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %s", args, out)
		}
	}
	run("branch", "feature")

	if err := git.CheckoutBranch(repo, "feature"); err != nil {
		t.Fatalf("checkout: %v", err)
	}
	branches := git.ListBranches(repo)
	var current string
	for _, b := range branches.Branches {
		if b.IsCurrent {
			current = b.Name
		}
	}
	if current != "feature" {
		t.Fatalf("current branch = %s, want feature", current)
	}
}
