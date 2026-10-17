// GH_TOKEN injection: agent bash runs (sync and background) inherit the
// stored GitHub PAT so `gh` works on a headless box with no browser login,
// and the secret is scrubbed from anything echoed back to the model.
package tests

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/tools"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/providers/github"
)

const testGHPAT = "ghp_testtoken1234567890"

// clearGHEnv removes any operator-exported token for the test. AppendToEnv
// lets an explicit GH_TOKEN/GITHUB_TOKEN win over the stored credential, so a
// developer machine (or CI) that exports one would otherwise suppress the
// injection these tests exercise. t.Setenv registers the restore; the Unsetenv
// then makes the variable genuinely absent rather than empty.
func clearGHEnv(t *testing.T) {
	t.Helper()
	for _, key := range []string{"GH_TOKEN", "GITHUB_TOKEN"} {
		t.Setenv(key, "")
		os.Unsetenv(key)
	}
}

// withTempCreds points the credential file at a temp dir and stores a fake
// PAT. SaveCredential only writes the file — no network.
func withTempCreds(t *testing.T) {
	t.Helper()
	clearGHEnv(t)
	t.Setenv("GITHUB_CREDENTIALS_PATH", filepath.Join(t.TempDir(), "github-creds.json"))
	if err := github.SaveCredential(github.Credential{Token: testGHPAT, Username: "octocat"}); err != nil {
		t.Fatalf("save credential: %v", err)
	}
}

func withNoCreds(t *testing.T) {
	t.Helper()
	clearGHEnv(t)
	t.Setenv("GITHUB_CREDENTIALS_PATH", filepath.Join(t.TempDir(), "missing.json"))
}

func TestGHEnvInjectedWhenConnected(t *testing.T) {
	withTempCreds(t)
	env := github.AppendToEnv([]string{"PATH=/usr/bin"})
	found := false
	for _, kv := range env {
		if kv == "GH_TOKEN="+testGHPAT {
			found = true
		}
		if strings.HasPrefix(kv, "GITHUB_TOKEN=") {
			t.Fatalf("GITHUB_TOKEN must be left alone: %q", kv)
		}
	}
	if !found {
		t.Fatalf("GH_TOKEN not injected: %v", env)
	}
}

func TestGHEnvAbsentWhenDisconnected(t *testing.T) {
	withNoCreds(t)
	env := github.AppendToEnv([]string{"PATH=/usr/bin"})
	if len(env) != 1 || env[0] != "PATH=/usr/bin" {
		t.Fatalf("env must be untouched without a credential: %v", env)
	}
}

func TestGHEnvExplicitValueWins(t *testing.T) {
	withTempCreds(t)
	for _, base := range []string{"GH_TOKEN=operator-set", "GITHUB_TOKEN=operator-set"} {
		env := github.AppendToEnv([]string{"PATH=/usr/bin", base})
		if len(env) != 2 {
			t.Fatalf("explicit value must suppress injection: %v", env)
		}
		for _, kv := range env {
			if strings.HasPrefix(kv, "GH_TOKEN="+testGHPAT) {
				t.Fatalf("stored token must not override explicit value: %v", env)
			}
		}
	}
}

func runBash(t *testing.T, command string) string {
	t.Helper()
	bash := tools.NewBashTool(nil, "")
	args, _ := json.Marshal(map[string]any{"command": command})
	out, err := bash.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	return resultText(t, out)
}

// The child really sees the token (printenv prints something), but the model
// only ever sees the scrubbed form.
func TestBashGHInjectedAndRedacted(t *testing.T) {
	withTempCreds(t)
	s := runBash(t, "printenv GH_TOKEN")
	if !strings.Contains(s, "[redacted]") {
		t.Fatalf("expected scrubbed token in output: %q", s)
	}
	if strings.Contains(s, testGHPAT) {
		t.Fatalf("raw token leaked into tool output: %q", s)
	}
}

func TestBashEnvDumpLeaksNothing(t *testing.T) {
	withTempCreds(t)
	s := runBash(t, "env")
	if strings.Contains(s, testGHPAT) {
		t.Fatalf("raw token leaked via env dump")
	}
}

func TestBashGHEmptyWhenDisconnected(t *testing.T) {
	withNoCreds(t)
	// printenv exits 1 on a missing var; the tool reports but must not error.
	bash := tools.NewBashTool(nil, "")
	args, _ := json.Marshal(map[string]any{"command": "printenv GH_TOKEN"})
	out, err := bash.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if strings.Contains(resultText(t, out), testGHPAT) {
		t.Fatalf("no token should exist without a credential")
	}
}

func TestRedactSecretsNoCredential(t *testing.T) {
	withNoCreds(t)
	if got := github.RedactSecrets("nothing secret here"); got != "nothing secret here" {
		t.Fatalf("redaction without a credential must be identity: %q", got)
	}
}
