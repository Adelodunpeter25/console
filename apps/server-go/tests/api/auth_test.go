// Codex OAuth login flow coverage: status, login URL state, callback
// consume/expiry, and the exchange-and-save success path.
package tests

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/auth"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/providers/codex"
)

func TestAuthStatusLoggedOut(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CODEX_CREDENTIALS_PATH", filepath.Join(dir, "missing.json"))
	t.Setenv("OPENAI_CODEX_OAUTH_TOKEN", "")
	t.Setenv("CLAUDE_CREDENTIALS_PATH", filepath.Join(dir, "missing.json"))
	t.Setenv("CLAUDE_OAUTH_TOKEN", "")
	t.Setenv("ANTHROPIC_OAUTH_TOKEN", "")
	t.Setenv("ANTIGRAVITY_CREDENTIALS_PATH", filepath.Join(dir, "missing.json"))
	status := auth.NewAuthService().GetStatus()
	if status.Codex.LoggedIn || status.Claude.LoggedIn || status.Antigravity.LoggedIn {
		t.Fatal("must be logged out without credentials")
	}
	if status.Devin.LoggedIn {
		t.Fatalf("unported providers must report logged out: %+v", status)
	}
}

func TestAuthStatusLoggedIn(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CODEX_CREDENTIALS_PATH", filepath.Join(dir, "codex-creds.json"))
	t.Setenv("OPENAI_CODEX_OAUTH_TOKEN", "")
	t.Setenv("CLAUDE_CREDENTIALS_PATH", filepath.Join(dir, "missing-claude.json"))
	t.Setenv("ANTIGRAVITY_CREDENTIALS_PATH", filepath.Join(dir, "missing-antigravity.json"))
	svc := auth.NewAuthService()
	if err := codex.SaveCredential(codex.OAuthCredential{AccessToken: "a", RefreshToken: "r", ExpiresAt: 1, AccountID: "acc", Email: "u@example.com"}); err != nil {
		t.Fatal(err)
	}
	status := svc.GetStatus()
	if !status.Codex.LoggedIn || status.Codex.Email != "u@example.com" {
		t.Fatalf("status: %+v", status)
	}
}

func TestAuthLoginURLRoundTrip(t *testing.T) {
	svc := auth.NewAuthService()
	first, err := svc.GetLoginURL()
	if err != nil {
		t.Fatal(err)
	}
	if first.Provider != "codex" || first.State == "" || !strings.Contains(first.AuthURL, first.State) {
		t.Fatalf("login url: %+v", first)
	}
	if !strings.HasSuffix(first.RedirectURI, "/auth/callback") {
		t.Fatalf("redirect: %s", first.RedirectURI)
	}
	second, err := svc.GetLoginURL()
	if err != nil {
		t.Fatal(err)
	}
	if first.State == second.State {
		t.Fatal("state tokens must be unique")
	}
}

func TestAuthCallbackConsumeAndExpiry(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CODEX_CREDENTIALS_PATH", filepath.Join(dir, "codex-creds.json"))
	svc := auth.NewAuthService()
	svc.Exchange = func(code, verifier, redirectURI string) (codex.OAuthCredential, error) {
		if code != "auth-code" || verifier == "" || !strings.HasSuffix(redirectURI, "/auth/callback") {
			t.Errorf("exchange args: %q %q %q", code, verifier, redirectURI)
		}
		return codex.OAuthCredential{AccessToken: "a", RefreshToken: "r", ExpiresAt: time.Now().UnixMilli() + 3600_000, AccountID: "acc-1", Email: "user@example.com"}, nil
	}

	login, err := svc.GetLoginURL()
	if err != nil {
		t.Fatal(err)
	}
	result, err := svc.HandleCallback("auth-code", login.State)
	if err != nil {
		t.Fatalf("callback: %v", err)
	}
	if result.Provider != "codex" || result.UserEmail != "user@example.com" {
		t.Fatalf("result: %+v", result)
	}
	// State is single-use: replay must fail.
	if _, err := svc.HandleCallback("auth-code", login.State); err == nil {
		t.Fatal("replayed state must be rejected")
	}
	// Unknown state must fail.
	if _, err := svc.HandleCallback("auth-code", "nope"); err == nil {
		t.Fatal("unknown state must be rejected")
	}
	// Missing state must fail.
	if _, err := svc.HandleCallback("auth-code", ""); err == nil {
		t.Fatal("missing state must be rejected")
	}
	// Credential was persisted by the callback.
	cred, err := codex.LoadCredential()
	if err != nil || cred.AccountID != "acc-1" {
		t.Fatalf("saved credential: %+v %v", cred, err)
	}
	if !auth.NewAuthService().GetStatus().Codex.LoggedIn {
		t.Fatal("status must be logged in after callback")
	}
}
