// ProjectBootstrapService coverage: blank init creates a real repo, clone
// works against a local origin, and both refuse to touch a non-empty target.
// Also pins the gitconfig integration the plan depends on — an SSH-style URL
// must arrive at git as HTTPS without any rewriting here.
package tests

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/gitconfig"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/providers/github"
	projectssvc "github.com/Adelodunpeter25/console/apps/server-go/internal/services/projects"
)

// bootstrapRepo creates a local git repo with one commit, usable as a clone
// origin (local paths need no credentials, unlike github.com).
func bootstrapRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if out := bootstrapGit(t, dir, "init", "-b", "main"); out == "" {
		t.Fatal("git init produced no output")
	}
	bootstrapGit(t, dir, "-c", "user.name=t", "-c", "user.email=t@t", "commit", "--allow-empty", "-m", "init")
	return dir
}

// blankService returns a bootstrap service plus an empty parent dir to create
// projects in.
func blankService(t *testing.T) (*projectssvc.ProjectBootstrapService, string) {
	t.Helper()
	return projectssvc.NewProjectBootstrapService(), t.TempDir()
}

func TestInitBlankCreatesRepo(t *testing.T) {
	svc, parent := blankService(t)

	dir, err := svc.InitBlank(parent, "my-project")
	if err != nil {
		t.Fatalf("InitBlank: %v", err)
	}
	if dir != filepath.Join(parent, "my-project") {
		t.Fatalf("dir = %q", dir)
	}
	if info, err := os.Stat(filepath.Join(dir, ".git")); err != nil || !info.IsDir() {
		t.Fatalf(".git must exist in %s: %v", dir, err)
	}
	// A blank project is an empty repo — no seed commit. Verified by exit status:
	// `rev-parse --verify HEAD` fails on an unborn HEAD and prints to stderr, so
	// inspecting its output would read a correct repo as broken.
	if bootstrapGitOK(t, dir, "rev-parse", "--verify", "HEAD") {
		t.Fatal("blank project must have no commits")
	}
}

func TestInitBlankRefusesNonEmptyTarget(t *testing.T) {
	svc, parent := blankService(t)

	existing := filepath.Join(parent, "taken")
	if err := os.MkdirAll(existing, 0o755); err != nil {
		t.Fatal(err)
	}
	keep := filepath.Join(existing, "important.txt")
	if err := os.WriteFile(keep, []byte("do not delete"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := svc.InitBlank(parent, "taken")
	if !errors.Is(err, projectssvc.ErrTargetNotEmpty) {
		t.Fatalf("want ErrTargetNotEmpty, got %v", err)
	}
	// The refusal must not have touched the existing content.
	if _, err := os.Stat(keep); err != nil {
		t.Fatalf("existing file was disturbed: %v", err)
	}
}

// An existing but EMPTY dir is allowed: macOS users routinely pre-create the
// folder before pointing a clone or init at it.
func TestInitBlankAllowsExistingEmptyTarget(t *testing.T) {
	svc, parent := blankService(t)

	pre := filepath.Join(parent, "premade")
	if err := os.MkdirAll(pre, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.InitBlank(parent, "premade"); err != nil {
		t.Fatalf("empty existing dir must be accepted: %v", err)
	}
}

func TestInitBlankRejectsTraversalAndMissingParent(t *testing.T) {
	svc, parent := blankService(t)

	for _, name := range []string{"..", ".", "a/b", `a\b`, "  "} {
		if _, err := svc.InitBlank(parent, name); err == nil {
			t.Fatalf("name %q must be rejected", name)
		}
	}
	if _, err := svc.InitBlank(filepath.Join(parent, "nope"), "child"); err == nil {
		t.Fatal("missing parent dir must be rejected")
	}
}

func TestCloneFromLocalOrigin(t *testing.T) {
	svc, parent := blankService(t)
	origin := bootstrapRepo(t)

	dir, err := svc.Clone(origin, parent, "")
	if err != nil {
		t.Fatalf("Clone: %v", err)
	}
	// Folder name defaults to the source directory's name.
	if filepath.Base(dir) != filepath.Base(origin) {
		t.Fatalf("default folder name = %q, want %q", filepath.Base(dir), filepath.Base(origin))
	}
	if _, err := os.Stat(filepath.Join(dir, ".git")); err != nil {
		t.Fatalf("cloned repo has no .git: %v", err)
	}
	// A real clone carries history, not just a working tree.
	if got := bootstrapGit(t, dir, "rev-list", "--count", "HEAD"); strings.TrimSpace(got) == "0" {
		t.Fatalf("clone carried no commits")
	}
}

func TestCloneRefusesNonEmptyTarget(t *testing.T) {
	svc, parent := blankService(t)
	origin := bootstrapRepo(t)

	existing := filepath.Join(parent, "occupied")
	if err := os.MkdirAll(existing, 0o755); err != nil {
		t.Fatal(err)
	}
	keep := filepath.Join(existing, "keep.txt")
	if err := os.WriteFile(keep, []byte("mine"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := svc.Clone(origin, parent, "occupied")
	if !errors.Is(err, projectssvc.ErrTargetNotEmpty) {
		t.Fatalf("want ErrTargetNotEmpty, got %v", err)
	}
	if _, err := os.Stat(keep); err != nil {
		t.Fatalf("clone refusal disturbed existing content: %v", err)
	}
}

func TestCloneRejectsBadURL(t *testing.T) {
	svc, parent := blankService(t)

	for _, url := range []string{"", "   ", "https://github.com/", "not a url"} {
		if _, err := svc.Clone(url, parent, ""); err == nil {
			t.Fatalf("url %q must be rejected", url)
		}
	}
	// Nothing may be left behind by a rejected URL.
	entries, err := os.ReadDir(parent)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("rejected clone left %d entries behind", len(entries))
	}
}

// A missing token is only fatal for a GitHub URL. Any other host is left to
// whatever credentials git already has.
func TestCloneWithoutTokenOnlyBlocksGitHub(t *testing.T) {
	svc, parent := blankService(t)
	isolateGitHubCredentials(t)

	_, err := svc.Clone("https://github.com/octocat/private-thing.git", parent, "")
	if !errors.Is(err, projectssvc.ErrGitHubNotConnected) {
		t.Fatalf("want ErrGitHubNotConnected for a GitHub URL, got %v", err)
	}

	origin := bootstrapRepo(t)
	if _, err := svc.Clone(origin, parent, ""); err != nil {
		t.Fatalf("non-GitHub clone must not require a GitHub token: %v", err)
	}
}

func TestCloneWithStoredTokenIsNotBlocked(t *testing.T) {
	svc, parent := blankService(t)
	setGitHubCredential(t, "ghp_test_token")

	// The guard must not fire. The clone itself may still fail (github.com is
	// unreachable from a sandboxed test run), but it must fail as a git error,
	// not as ErrGitHubNotConnected.
	_, err := svc.Clone("https://github.com/octocat/hello.git", parent, "")
	if errors.Is(err, projectssvc.ErrGitHubNotConnected) {
		t.Fatalf("a stored token must satisfy the guard, got %v", err)
	}
}

// The plan lists "SSH-style URL pastes clone over HTTPS" as a requirement.
// It is satisfied by the injected insteadOf config, not by code here — this
// is the regression guard proving the two halves still meet.
func TestCloneSSHStyleURLUsesHTTPSRewrite(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	isolateGitHubCredentials(t)
	clearGitConfigEnv(t)
	if err := gitconfig.Configure(); err != nil {
		t.Fatal(err)
	}

	repo := t.TempDir()
	bootstrapGit(t, repo, "init", "-q")
	bootstrapGit(t, repo, "remote", "add", "origin", "git@github.com:octocat/hello.git")

	got := strings.TrimSpace(bootstrapGit(t, repo, "remote", "get-url", "origin"))
	if got != "https://github.com/octocat/hello.git" {
		t.Fatalf("SSH-style URL must resolve to HTTPS, got %q", got)
	}
}

// isolateGitHubCredentials points credential lookups at an empty temp file so
// a developer's real ~/.console credentials cannot influence the result.
func isolateGitHubCredentials(t *testing.T) {
	t.Helper()
	t.Setenv("GITHUB_CREDENTIALS_PATH", filepath.Join(t.TempDir(), "github-creds.json"))
}

func setGitHubCredential(t *testing.T, token string) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("GITHUB_CREDENTIALS_PATH", filepath.Join(dir, "github-creds.json"))
	if err := github.SaveCredential(github.Credential{Token: token}); err != nil {
		t.Fatal(err)
	}
}

// bootstrapGit runs git with an isolated HOME and no system config, and
// returns combined output. It tolerates failure, because several assertions
// here are "this command is expected to fail".
func bootstrapGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	out, _ := bootstrapGitRun(t, dir, args...)
	return out
}

// bootstrapGitOK reports whether git exited zero. Needed where the assertion
// is about exit status rather than output — e.g. an unborn HEAD, where git
// fails *and* writes to stderr, so an output check reads a correct repo as
// broken.
func bootstrapGitOK(t *testing.T, dir string, args ...string) bool {
	t.Helper()
	_, err := bootstrapGitRun(t, dir, args...)
	return err == nil
}

func bootstrapGitRun(t *testing.T, dir string, args ...string) (string, error) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_CONFIG_NOSYSTEM=1", "HOME="+t.TempDir())
	out, err := cmd.CombinedOutput()
	return string(out), err
}
