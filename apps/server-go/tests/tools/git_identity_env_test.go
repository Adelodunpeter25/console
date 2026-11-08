// Git identity injection: spawned commands get the connected account as a
// fallback author, delivered under the machine's own git config.
package tests

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/providers/github"
)

func gitConfigGlobalOf(env []string) string {
	for _, kv := range env {
		if v, ok := strings.CutPrefix(kv, "GIT_CONFIG_GLOBAL="); ok {
			return v
		}
	}
	return ""
}

func TestGitIdentityUsesNoReplyWithoutPublicEmail(t *testing.T) {
	clearGHEnv(t)
	t.Setenv("GITHUB_CREDENTIALS_PATH", filepath.Join(t.TempDir(), "github-creds.json"))
	t.Setenv("HOME", t.TempDir())
	if err := github.SaveCredential(github.Credential{Token: testGHPAT, Username: "octocat", ID: 583231}); err != nil {
		t.Fatal(err)
	}
	path := gitConfigGlobalOf(github.AppendToEnv([]string{"PATH=" + os.Getenv("PATH")}))
	if path == "" {
		t.Fatal("GIT_CONFIG_GLOBAL should be set when GitHub is connected")
	}
	out, err := exec.Command("git", "config", "--file", path, "user.email").Output()
	if err != nil || strings.TrimSpace(string(out)) != "583231+octocat@users.noreply.github.com" {
		t.Fatalf("email = %q (%v)", out, err)
	}
}

func TestGitIdentityPrefersPublicEmailAndYieldsToTheUsersConfig(t *testing.T) {
	clearGHEnv(t)
	t.Setenv("GITHUB_CREDENTIALS_PATH", filepath.Join(t.TempDir(), "github-creds.json"))
	home := t.TempDir()
	t.Setenv("HOME", home)
	if err := os.WriteFile(filepath.Join(home, ".gitconfig"), []byte("[user]\n\temail = mine@example.com\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := github.SaveCredential(github.Credential{Token: testGHPAT, Username: "octocat", Name: "The Octocat", Email: "octo@example.com"}); err != nil {
		t.Fatal(err)
	}
	path := gitConfigGlobalOf(github.AppendToEnv(nil))
	get := func(key string) string {
		cmd := exec.Command("git", "config", "--get", key)
		cmd.Env = []string{"HOME=" + home, "GIT_CONFIG_GLOBAL=" + path, "PATH=" + os.Getenv("PATH")}
		out, _ := cmd.Output()
		return strings.TrimSpace(string(out))
	}
	if got := get("user.name"); got != "The Octocat" {
		t.Fatalf("name = %q, want the GitHub name as fallback", got)
	}
	if got := get("user.email"); got != "mine@example.com" {
		t.Fatalf("email = %q, the user's own git config must win", got)
	}
}

func TestGitIdentityAbsentWhenNotConnectedOrAlreadyOverridden(t *testing.T) {
	withNoCreds(t)
	if got := gitConfigGlobalOf(github.AppendToEnv(nil)); got != "" {
		t.Fatalf("no credential, yet GIT_CONFIG_GLOBAL=%q", got)
	}
	withTempCreds(t)
	if got := gitConfigGlobalOf(github.AppendToEnv([]string{"GIT_CONFIG_GLOBAL=/custom"})); got != "/custom" {
		t.Fatalf("an explicit GIT_CONFIG_GLOBAL must be left alone, got %q", got)
	}
}
