// Files-to-copy coverage: .worktreeinclude patterns, the `link:` opt-in,
// the default .env*, negation, build-output refusal, and bad-pattern
// silence. All through WorktreeAdd end to end; assertions are on the new
// worktree's filesystem. Helpers git/initRepo/branchExists live in
// worktree_service_test.go (same package).
package tests

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/services"
)

func writeRepoFile(t *testing.T, dir, name, content string, perm os.FileMode) {
	t.Helper()
	p := filepath.Join(dir, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), perm); err != nil {
		t.Fatal(err)
	}
}

func commitAll(t *testing.T, dir, msg string) {
	t.Helper()
	git(t, dir, "add", "-A")
	git(t, dir, "-c", "user.name=t", "-c", "user.email=t@t", "commit", "-m", msg)
}

func worktreeTarget(t *testing.T) string {
	t.Helper()
	tmp, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return filepath.Join(tmp, "wt")
}

func assertFileContent(t *testing.T, path, want string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	if string(data) != want {
		t.Fatalf("%s = %q, want %q", path, data, want)
	}
}

func assertAbsent(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Lstat(path); !os.IsNotExist(err) {
		t.Fatalf("%s present, want absent (err=%v)", path, err)
	}
}

func TestWorktreeAddCarriesGitignoredMatches(t *testing.T) {
	svc := services.NewWorktreeService()
	repo := initRepo(t, true)
	writeRepoFile(t, repo, ".gitignore", ".env*\nconfig/*.json\n*.keystore\nkeys/\n", 0o644)
	writeRepoFile(t, repo, ".worktreeinclude", "# local files carried into every worktree\n.env*\nconfig/*.json\n!config/*.example.json\nlink: keys/dev.keystore\nlink: keys/missing.keystore\n", 0o644)
	writeRepoFile(t, repo, "app.txt", "tracked", 0o644)
	commitAll(t, repo, "base")
	writeRepoFile(t, repo, ".env.local", "ENV=1", 0o600)
	writeRepoFile(t, repo, "config/local.json", "{}", 0o644)
	writeRepoFile(t, repo, "config/local.example.json", "{}", 0o644)
	writeRepoFile(t, repo, "scratch.txt", "scratch", 0o644)
	writeRepoFile(t, repo, "keys/dev.keystore", "keydata", 0o644)

	wt := worktreeTarget(t)
	if err := svc.WorktreeAdd(repo, wt, "carry", ""); err != nil {
		t.Fatalf("WorktreeAdd: %v", err)
	}

	// Matched gitignored files arrive; mode bits survive the copy.
	assertFileContent(t, filepath.Join(wt, ".env.local"), "ENV=1")
	if fi, err := os.Stat(filepath.Join(wt, ".env.local")); err != nil || fi.Mode().Perm() != 0o600 {
		t.Fatalf(".env.local perm = %v, err = %v; want 0600", fiMode(fi), err)
	}
	assertFileContent(t, filepath.Join(wt, "config", "local.json"), "{}")
	// Negated, non-ignored untracked, and missing link sources stay out.
	assertAbsent(t, filepath.Join(wt, "config", "local.example.json"))
	assertAbsent(t, filepath.Join(wt, "scratch.txt"))
	assertAbsent(t, filepath.Join(wt, "keys", "missing.keystore"))
	// Tracked files still come from git itself.
	assertFileContent(t, filepath.Join(wt, "app.txt"), "tracked")

	// `link:` entries are live symlinks into the source checkout.
	link := filepath.Join(wt, "keys", "dev.keystore")
	fi, err := os.Lstat(link)
	if err != nil || fi.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("%s not a symlink: fi=%v err=%v", link, fi, err)
	}
	assertFileContent(t, link, "keydata")
	got, err := filepath.EvalSymlinks(link)
	if err != nil {
		t.Fatalf("EvalSymlinks: %v", err)
	}
	want, err := filepath.EvalSymlinks(filepath.Join(repo, "keys", "dev.keystore"))
	if err != nil {
		t.Fatalf("EvalSymlinks source: %v", err)
	}
	if got != want {
		t.Fatalf("symlink resolves to %s, want %s", got, want)
	}
}

func fiMode(fi os.FileInfo) os.FileMode {
	if fi == nil {
		return 0
	}
	return fi.Mode().Perm()
}

func TestWorktreeAddLinksWholeDir(t *testing.T) {
	svc := services.NewWorktreeService()
	repo := initRepo(t, true)
	writeRepoFile(t, repo, ".gitignore", "*.keystore\nkeystore\n", 0o644)
	writeRepoFile(t, repo, ".worktreeinclude", "link: keystore/\n", 0o644)
	commitAll(t, repo, "base")
	writeRepoFile(t, repo, "keystore/ks.keystore", "v1", 0o644)

	wt := worktreeTarget(t)
	if err := svc.WorktreeAdd(repo, wt, "dirlink", ""); err != nil {
		t.Fatalf("WorktreeAdd: %v", err)
	}
	dirLink := filepath.Join(wt, "keystore")
	if fi, err := os.Lstat(dirLink); err != nil || fi.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("%s not a symlink: fi=%v err=%v", dirLink, fi, err)
	}
	// Files added at the source after creation (a rotated key) show up
	// through the link.
	writeRepoFile(t, repo, "keystore/rotated.keystore", "v2", 0o644)
	assertFileContent(t, filepath.Join(dirLink, "rotated.keystore"), "v2")
}

func TestWorktreeAddDefaultsToEnv(t *testing.T) {
	svc := services.NewWorktreeService()
	repo := initRepo(t, true)
	writeRepoFile(t, repo, ".gitignore", ".env*\nlocal.txt\n", 0o644)
	commitAll(t, repo, "base")
	writeRepoFile(t, repo, ".env", "A", 0o644)
	writeRepoFile(t, repo, ".env.local", "B", 0o644)
	writeRepoFile(t, repo, "local.txt", "C", 0o644)

	wt := worktreeTarget(t)
	if err := svc.WorktreeAdd(repo, wt, "envdefault", ""); err != nil {
		t.Fatalf("WorktreeAdd: %v", err)
	}
	assertFileContent(t, filepath.Join(wt, ".env"), "A")
	assertFileContent(t, filepath.Join(wt, ".env.local"), "B")
	assertAbsent(t, filepath.Join(wt, "local.txt"))
}

func TestWorktreeAddSkipsBuildOutput(t *testing.T) {
	svc := services.NewWorktreeService()
	repo := initRepo(t, true)
	writeRepoFile(t, repo, ".gitignore", "dist/\n", 0o644)
	writeRepoFile(t, repo, ".worktreeinclude", "dist/**\n", 0o644)
	commitAll(t, repo, "base")
	writeRepoFile(t, repo, "dist/bundle.js", "js", 0o644)

	wt := worktreeTarget(t)
	if err := svc.WorktreeAdd(repo, wt, "nobuild", ""); err != nil {
		t.Fatalf("WorktreeAdd: %v", err)
	}
	assertAbsent(t, filepath.Join(wt, "dist", "bundle.js"))
}

func TestWorktreeAddBadPatternsStillSucceed(t *testing.T) {
	svc := services.NewWorktreeService()
	repo := initRepo(t, true)
	writeRepoFile(t, repo, ".worktreeinclude", "[[[bad\n# just a comment\n\n!\nlink:\n", 0o644)
	commitAll(t, repo, "base")

	wt := worktreeTarget(t)
	if err := svc.WorktreeAdd(repo, wt, "badpatterns", ""); err != nil {
		t.Fatalf("WorktreeAdd with bad patterns: %v", err)
	}
	found := false
	list, err := svc.WorktreeList(repo)
	if err != nil {
		t.Fatalf("WorktreeList: %v", err)
	}
	for _, w := range list {
		if w.Path == wt && w.Branch == "badpatterns" {
			found = true
		}
	}
	if !found {
		t.Fatalf("worktree missing from list after bad patterns: %+v", list)
	}
}
