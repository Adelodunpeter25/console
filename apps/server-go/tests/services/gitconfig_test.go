// gitconfig coverage: the GIT_CONFIG_* triples land in the process env with
// the right count, and real git actually honors them. The integration checks
// run a real `git config --list` (and `git remote get-url`) under the
// injected env, since the whole point of the mechanism is that git — not us —
// reads these variables.
package tests

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/gitconfig"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/providers/github"
)

// clearGitConfigEnv removes any inherited GIT_CONFIG_* so a developer's own
// shell config cannot influence these assertions.
func clearGitConfigEnv(t *testing.T) {
	t.Helper()
	for _, kv := range os.Environ() {
		if key, _, ok := strings.Cut(kv, "="); ok &&
			(key == "GIT_CONFIG_COUNT" || strings.HasPrefix(key, "GIT_CONFIG_")) {
			t.Setenv(key, "")
			_ = os.Unsetenv(key)
		}
	}
}

func TestGitConfigConfigureSetsTriples(t *testing.T) {
	clearGitConfigEnv(t)
	if err := gitconfig.Configure(); err != nil {
		t.Fatal(err)
	}

	count := os.Getenv("GIT_CONFIG_COUNT")
	if count != "3" {
		t.Fatalf("expected 3 config entries (helper + two insteadOf), got %q", count)
	}
	if got := os.Getenv("GIT_CONFIG_KEY_0"); got != "credential.helper" {
		t.Fatalf("key 0: %q", got)
	}
	value0 := os.Getenv("GIT_CONFIG_VALUE_0")
	if !strings.HasPrefix(value0, "!") {
		t.Fatalf("helper value must be a shell command (leading !), got %q", value0)
	}
	// The env prefix is what makes the invocation unambiguous with the
	// daemon's own CONSOLE_SERVE=1 mode, which this process inherits.
	if !strings.Contains(value0, gitconfig.HelperEnvVar+"=1") {
		t.Fatalf("helper value must set %s=1, got %q", gitconfig.HelperEnvVar, value0)
	}
	if !strings.Contains(value0, "'") {
		t.Fatalf("helper path should be shell-quoted, got %q", value0)
	}

	wantInsteadOf := []string{"git@github.com:", "ssh://git@github.com/"}
	for i, want := range wantInsteadOf {
		key := os.Getenv(fmt.Sprintf("GIT_CONFIG_KEY_%d", i+1))
		if key != "url.https://github.com/.insteadOf" {
			t.Fatalf("key %d: %q", i+1, key)
		}
		if got := os.Getenv(fmt.Sprintf("GIT_CONFIG_VALUE_%d", i+1)); got != want {
			t.Fatalf("value %d: got %q want %q", i+1, got, want)
		}
	}
}

// Real git must resolve the helper entry. Uses --get-urlmatch rather than
// --list so the assertion is about git's interpretation, not our formatting.
func TestGitConfigRealGitSeesHelperAndRewrite(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	clearGitConfigEnv(t)
	if err := gitconfig.Configure(); err != nil {
		t.Fatal(err)
	}

	// credential.helper is a multi-valued key; --get-all lists every value.
	out := runGitInherit(t, "", "config", "--get-all", "credential.helper")
	if !strings.Contains(out, gitconfig.HelperEnvVar+"=1") {
		t.Fatalf("git did not resolve the credential helper, got %q", out)
	}

	// insteadOf must actually rewrite an SSH remote to HTTPS.
	repo := t.TempDir()
	runGitInherit(t, repo, "init", "-q")
	runGitInherit(t, repo, "remote", "add", "origin", "git@github.com:octocat/hello.git")
	got := strings.TrimSpace(runGitInherit(t, repo, "remote", "get-url", "origin"))
	if got != "https://github.com/octocat/hello.git" {
		t.Fatalf("SSH remote must rewrite to HTTPS, got %q", got)
	}
}

// The rewrite must be additive: an already-HTTPS URL is untouched, and a
// non-GitHub SSH remote must not be rewritten to github.com.
func TestGitConfigRewriteIsScopedToGitHub(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	clearGitConfigEnv(t)
	if err := gitconfig.Configure(); err != nil {
		t.Fatal(err)
	}

	repo := t.TempDir()
	runGitInherit(t, repo, "init", "-q")

	runGitInherit(t, repo, "remote", "add", "plain", "https://github.com/octocat/hello.git")
	if got := strings.TrimSpace(runGitInherit(t, repo, "remote", "get-url", "plain")); got != "https://github.com/octocat/hello.git" {
		t.Fatalf("HTTPS URL must be unchanged, got %q", got)
	}

	runGitInherit(t, repo, "remote", "add", "sshurl", "ssh://git@github.com/octocat/hello.git")
	if got := strings.TrimSpace(runGitInherit(t, repo, "remote", "get-url", "sshurl")); got != "https://github.com/octocat/hello.git" {
		t.Fatalf("ssh:// form must rewrite, got %q", got)
	}

	runGitInherit(t, repo, "remote", "add", "other", "git@gitlab.com:org/repo.git")
	if got := strings.TrimSpace(runGitInherit(t, repo, "remote", "get-url", "other")); got != "git@gitlab.com:org/repo.git" {
		t.Fatalf("non-GitHub SSH remote must be untouched, got %q", got)
	}
}

// End-to-end through the real git binary: with no credential stored, git
// must find nothing and fall through to its normal behavior.
//
// Uses a purpose-built console binary rather than Configure(), because this
// test actually invokes the helper: pointing it at the test binary would make
// git re-enter the test suite as a subprocess.
func TestGitConfigNoCredentialIsSilentNotAnError(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	console := buildConsoleBinary(t)

	dir := t.TempDir()
	t.Setenv("GITHUB_CREDENTIALS_PATH", filepath.Join(dir, "github-creds.json"))
	clearGitConfigEnv(t)
	if err := gitconfig.ConfigureWithExe(console); err != nil {
		t.Fatal(err)
	}
	out, code := credentialFill(t)
	if code == 0 && strings.Contains(out, "x-access-token") {
		t.Fatalf("helper must not invent credentials: %q", out)
	}
}

// End-to-end through the real git binary: with a credential stored, git must
// receive it from the helper. This exercises the whole chain — Configure ->
// git -> a real console binary running the helper — rather than each piece in
// isolation.
//
// The helper is driven by a purpose-built console binary rather than the test
// binary, because git executes the helper path as a subprocess and the test
// binary would re-enter the test suite.
func TestGitConfigEndToEndWithCredential(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	console := buildConsoleBinary(t)

	dir := t.TempDir()
	t.Setenv("GITHUB_CREDENTIALS_PATH", filepath.Join(dir, "github-creds.json"))
	if err := github.SaveCredential(github.Credential{Token: "ghp_e2e_secret"}); err != nil {
		t.Fatal(err)
	}
	clearGitConfigEnv(t)
	if err := gitconfig.ConfigureWithExe(console); err != nil {
		t.Fatal(err)
	}
	out, code := credentialFill(t)
	if code != 0 {
		t.Fatalf("git credential fill failed (%d): %q", code, out)
	}
	if !strings.Contains(out, "username=x-access-token") || !strings.Contains(out, "password=ghp_e2e_secret") {
		t.Fatalf("git did not receive the credential: %q", out)
	}
}

// The helper must answer only for github.com even when a real binary is
// driving it, so an unrelated host never receives the token.
func TestGitConfigEndToEndOtherHostGetsNothing(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	console := buildConsoleBinary(t)

	dir := t.TempDir()
	t.Setenv("GITHUB_CREDENTIALS_PATH", filepath.Join(dir, "github-creds.json"))
	if err := github.SaveCredential(github.Credential{Token: "ghp_e2e_secret"}); err != nil {
		t.Fatal(err)
	}
	clearGitConfigEnv(t)
	if err := gitconfig.ConfigureWithExe(console); err != nil {
		t.Fatal(err)
	}
	out, _ := credentialFillHost(t, "gitlab.com")
	if strings.Contains(out, "ghp_e2e_secret") {
		t.Fatalf("token leaked to a non-GitHub host: %q", out)
	}
}

// The daemon runs under CONSOLE_SERVE=1 and git inherits that env. The
// helper must still answer rather than booting a second server — this is the
// ordering constraint documented in cmd/console/main.go, asserted end to end.
func TestGitConfigHelperBeatsServeMode(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	console := buildConsoleBinary(t)

	dir := t.TempDir()
	t.Setenv("GITHUB_CREDENTIALS_PATH", filepath.Join(dir, "github-creds.json"))
	if err := github.SaveCredential(github.Credential{Token: "ghp_serve_test"}); err != nil {
		t.Fatal(err)
	}
	clearGitConfigEnv(t)
	if err := gitconfig.ConfigureWithExe(console); err != nil {
		t.Fatal(err)
	}
	// Simulate the daemon's environment reaching git.
	t.Setenv("CONSOLE_SERVE", "1")

	cmd := exec.Command("git", "credential", "fill")
	cmd.Stdin = strings.NewReader("protocol=https\nhost=github.com\n\n")
	cmd.Env = append(os.Environ(),
		"HOME="+t.TempDir(),
		"GIT_CONFIG_NOSYSTEM=1",
		"GIT_TERMINAL_PROMPT=0",
		"CONSOLE_SERVE=1",
		"PORT=0",
	)
	done := make(chan struct{})
	var out []byte
	var runErr error
	go func() {
		out, runErr = cmd.CombinedOutput()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(30 * time.Second):
		t.Fatal("helper hung: CONSOLE_SERVE=1 was treated as serve mode")
	}
	if runErr != nil {
		t.Fatalf("git credential fill failed under CONSOLE_SERVE=1: %v\n%s", runErr, out)
	}
	if !strings.Contains(string(out), "password=ghp_serve_test") {
		t.Fatalf("helper did not answer under CONSOLE_SERVE=1: %q", out)
	}
}

// buildConsoleBinary compiles cmd/console once per test run and returns its
// path. Skips the test if the toolchain cannot build it.
func buildConsoleBinary(t *testing.T) string {
	t.Helper()
	consoleOnce.Do(func() {
		dir, err := os.MkdirTemp("", "console-helper-bin")
		if err != nil {
			return
		}
		out := filepath.Join(dir, "console")
		cmd := exec.Command("go", "build", "-o", out, "./cmd/console")
		cmd.Dir = repoRoot(t)
		if combined, err := cmd.CombinedOutput(); err != nil {
			consoleBuildErr = fmt.Errorf("build console: %v\n%s", err, combined)
			return
		}
		consoleBin = out
	})
	if consoleBin == "" {
		if consoleBuildErr != nil {
			t.Skipf("cannot build console binary: %v", consoleBuildErr)
		}
		t.Skip("console binary unavailable")
	}
	return consoleBin
}

var (
	consoleOnce     sync.Once
	consoleBin      string
	consoleBuildErr error
)

// repoRoot walks up from the test's working directory to the Go module root.
func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("could not locate go.mod")
		}
		dir = parent
	}
}

// credentialFill asks git for credentials non-interactively. Isolation from
// the developer's own git config matters here: a global/system
// credential.helper (macOS keychain, GCM, ~/.git-credentials) answers first
// and would mask the behavior under test. HOME plus GIT_CONFIG_NOSYSTEM is
// used rather than GIT_CONFIG_SYSTEM, which some git builds (notably Apple's)
// ignore.
func credentialFill(t *testing.T) (string, int) {
	t.Helper()
	return credentialFillHost(t, "github.com")
}

// credentialFillHost is credentialFill for an arbitrary host.
func credentialFillHost(t *testing.T, host string) (string, int) {
	t.Helper()
	cmd := exec.Command("git", "credential", "fill")
	cmd.Stdin = strings.NewReader("protocol=https\nhost=" + host + "\n\n")
	cmd.Env = append(os.Environ(),
		"HOME="+t.TempDir(),
		"GIT_CONFIG_NOSYSTEM=1",
		"GIT_TERMINAL_PROMPT=0",
	)
	out, err := cmd.CombinedOutput()
	code := 0
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			code = exitErr.ExitCode()
		} else {
			t.Fatalf("run git credential fill: %v", err)
		}
	}
	return string(out), code
}

// runGitInherit runs git with the current process env (which includes the
// GIT_CONFIG_* triples set by Configure) and returns stdout.
func runGitInherit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	if dir != "" {
		cmd.Dir = dir
	}
	cmd.Env = os.Environ()
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return string(out)
}
