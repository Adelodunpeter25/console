// Phase 2 (account-aware provider credentials): active-account resolution,
// per-account loads, env precedence, zero-active disabled state, atomic
// credential writes, and refresh single-flight. The provider flows run
// against the real account store from the newAccounts helper.
package tests

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/accounts"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/providers/antigravity"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/providers/claude"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/providers/codex"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/providers/shared"
)

// registerActive plants one active account row in the store (metadata
// only). Its credential file is created by the caller when needed.
func registerActive(t *testing.T, provider, identity, email string) accounts.Account {
	t.Helper()
	acct, _, err := accounts.Register(provider, identity, email, "")
	if err != nil {
		t.Fatalf("register %s: %v", provider, err)
	}
	if !acct.Active {
		t.Fatalf("registered account should be active: %+v", acct)
	}
	return acct
}

// mustInactive asserts that adding an account never steals active from an
// existing one (decision 3), returning it.
func mustInactive(t *testing.T, provider, identity, email string) accounts.Account {
	t.Helper()
	acct, _, err := accounts.Register(provider, identity, email, "")
	if err != nil {
		t.Fatalf("register %s: %v", provider, err)
	}
	if acct.Active {
		t.Fatalf("new account must not steal active: %+v", acct)
	}
	return acct
}

func TestClaudeAccountsActiveResolution(t *testing.T) {
	newAccounts(t)
	first := registerActive(t, "claude", "a@x.io", "a@x.io")
	if err := claude.SaveCredentialFor(first.ID, claude.OAuthCredential{
		AccessToken: "tok-a", RefreshToken: "ra", ExpiresAt: 1 << 40, Email: "a@x.io",
	}); err != nil {
		t.Fatalf("save for account: %v", err)
	}

	got, err := claude.LoadCredential()
	if err != nil || got.AccessToken != "tok-a" || got.Email != "a@x.io" {
		t.Fatalf("active load = %+v (err=%v)", got, err)
	}

	// The legacy single file is ignored while an account is active.
	if err := claude.SaveCredential(claude.OAuthCredential{
		AccessToken: "legacy-tok", RefreshToken: "r", ExpiresAt: 1 << 41,
	}); err != nil {
		t.Fatalf("save legacy: %v", err)
	}
	if got, _ = claude.LoadCredential(); got.AccessToken != "tok-a" {
		t.Fatalf("active account must outrank the legacy file, got %q", got.AccessToken)
	}
}

func TestClaudeAccountsPinnedLoad(t *testing.T) {
	newAccounts(t)
	first := registerActive(t, "claude", "a@x.io", "a@x.io")
	second := mustInactive(t, "claude", "b@x.io", "b@x.io")
	for _, a := range []accounts.Account{first, second} {
		if err := claude.SaveCredentialFor(a.ID, claude.OAuthCredential{
			AccessToken: "tok-" + a.Email, RefreshToken: "r", ExpiresAt: 1 << 40,
		}); err != nil {
			t.Fatalf("save %s: %v", a.ID, err)
		}
	}

	got, err := claude.LoadCredentialFor(first.ID)
	if err != nil || got.AccessToken != "tok-a@x.io" {
		t.Fatalf("pinned load first = %+v (err=%v)", got, err)
	}
	// Pinned load reads an inactive account's file without switching active.
	if got, err = claude.LoadCredentialFor(second.ID); err != nil || got.AccessToken != "tok-b@x.io" {
		t.Fatalf("pinned load second = %+v (err=%v)", got, err)
	}
	if id, _ := accounts.EnabledID("claude"); id != first.ID {
		t.Fatalf("active account changed to %q by pinned loads", id)
	}

	// Unknown id: file not found, no fallback.
	if _, err := claude.LoadCredentialFor("acc_unknown"); err == nil {
		t.Fatal("unknown account load must error")
	}
}

func TestClaudeAccountsZeroActiveDisabled(t *testing.T) {
	newAccounts(t)
	acct := registerActive(t, "claude", "a@x.io", "a@x.io")
	if err := accounts.Select("claude", acct.ID, false); err != nil {
		t.Fatalf("deselect: %v", err)
	}

	_, err := claude.LoadCredential()
	if err == nil || !strings.Contains(err.Error(), "No active Claude account") {
		t.Fatalf("zero-active load err = %v, want a select-one message", err)
	}
	// The provider is still "configured" (credentials exist); this is the
	// needs-a-selection state, not a logged-out one.
	if !claude.CredentialExists() {
		t.Fatal("CredentialExists should stay true while accounts exist")
	}
}

func TestClaudeAccountsEnvPrecedence(t *testing.T) {
	newAccounts(t)
	acct := registerActive(t, "claude", "a@x.io", "a@x.io")
	if err := claude.SaveCredentialFor(acct.ID, claude.OAuthCredential{
		AccessToken: "file-tok", RefreshToken: "r", ExpiresAt: 1 << 40,
	}); err != nil {
		t.Fatal(err)
	}

	t.Setenv("CLAUDE_OAUTH_TOKEN", "env-tok")
	got, err := claude.LoadCredential()
	if err != nil || got.AccessToken != "env-tok" {
		t.Fatalf("env load = %+v (err=%v), want env token", got, err)
	}
	// Env does not shadow a named account.
	if got, err := claude.LoadCredentialFor(acct.ID); err != nil || got.AccessToken != "file-tok" {
		t.Fatalf("pinned load under env = %+v (err=%v), want the account file", got, err)
	}
}

func TestCodexAccountsActiveResolution(t *testing.T) {
	newAccounts(t)
	acct := registerActive(t, "codex", "acct_123", "c@x.io")
	if err := codex.SaveCredentialFor(acct.ID, codex.OAuthCredential{
		AccessToken: "tok", RefreshToken: "r", ExpiresAt: 1 << 40, AccountID: "acct_123", Email: "c@x.io",
	}); err != nil {
		t.Fatalf("save: %v", err)
	}

	got, err := codex.LoadCredential()
	if err != nil || got.AccountID != "acct_123" {
		t.Fatalf("active load = %+v (err=%v)", got, err)
	}

	if err := accounts.Select("codex", acct.ID, false); err != nil {
		t.Fatal(err)
	}
	if _, err := codex.LoadCredential(); err == nil || !strings.Contains(err.Error(), "No active Codex account") {
		t.Fatalf("zero-active load err = %v, want a select-one message", err)
	}
}

func TestAntigravityAccountsActiveResolution(t *testing.T) {
	newAccounts(t)
	acct := registerActive(t, "antigravity", "ag@x.io", "ag@x.io")
	if err := antigravity.SaveCredentialFor(acct.ID, antigravity.OAuthCredential{
		Token: "ag-token", ProjectID: "p1", Email: "ag@x.io", ExpiresAt: 1 << 40,
	}); err != nil {
		t.Fatalf("save: %v", err)
	}

	got, err := antigravity.LoadCredential()
	if err != nil || got.AccessToken != "ag-token" || got.ProjectID != "p1" {
		t.Fatalf("active load = %+v (err=%v)", got, err)
	}

	if err := accounts.Select("antigravity", acct.ID, false); err != nil {
		t.Fatal(err)
	}
	if _, err := antigravity.LoadCredential(); err == nil || !strings.Contains(err.Error(), "No active Antigravity account") {
		t.Fatalf("zero-active load err = %v, want a select-one message", err)
	}
}

func TestSharedSaveCredentialFilePermissions(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sub", "creds.json")
	if err := shared.SaveCredentialFile(path, map[string]string{"token": "t"}); err != nil {
		t.Fatalf("save: %v", err)
	}
	if fi, err := os.Stat(path); err != nil || fi.Mode().Perm() != 0o600 {
		t.Fatalf("file mode = %v (err=%v), want 0600", fi.Mode().Perm(), err)
	}
	if fi, err := os.Stat(filepath.Dir(path)); err != nil || fi.Mode().Perm() != 0o700 {
		t.Fatalf("dir mode = %v (err=%v), want 0700", fi.Mode().Perm(), err)
	}
	var back map[string]string
	raw, _ := os.ReadFile(path)
	if string(raw) == "" || string(raw[len(raw)-1]) != "\n" {
		t.Fatalf("saved content = %q, want newline-terminated", raw)
	}
	if err := json.Unmarshal(raw, &back); err != nil || back["token"] != "t" {
		t.Fatalf("reparse = %q (%v)", raw, err)
	}
	// The atomic write leaves exactly one file behind — no temp residue.
	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil || len(entries) != 1 {
		t.Fatalf("dir entries = %v (err=%v), want exactly the credential file", entries, err)
	}
}

func TestSharedSingleFlight(t *testing.T) {
	const n = 5
	var runs atomic.Int64
	start := time.Now()
	results := make([]string, n)
	errs := make([]error, n)

	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			results[i], errs[i] = shared.SingleFlight("same-key", func() (string, error) {
				runs.Add(1)
				time.Sleep(50 * time.Millisecond)
				return "winner", nil
			})
		}(i)
	}
	wg.Wait()
	if runs.Load() != 1 {
		t.Fatalf("fn ran %d times, want 1", runs.Load())
	}
	if time.Since(start) < 50*time.Millisecond {
		t.Fatal("waiters must not return before the first run completes")
	}
	for i := range results {
		if errs[i] != nil || results[i] != "winner" {
			t.Fatalf("waiter %d = %q (err=%v)", i, results[i], errs[i])
		}
	}
}

func TestSharedSingleFlightDistinctKeys(t *testing.T) {
	// Concurrency is what single-flight collapses; sequential callers with
	// the same key each run fn again.
	var runs atomic.Int64
	var wg sync.WaitGroup
	for _, key := range []string{"a", "a", "b"} {
		wg.Add(1)
		go func(key string) {
			defer wg.Done()
			if _, err := shared.SingleFlight(key, func() (int, error) {
				runs.Add(1)
				time.Sleep(20 * time.Millisecond)
				return 1, nil
			}); err != nil {
				t.Error(err)
			}
		}(key)
	}
	wg.Wait()
	if runs.Load() != 2 {
		t.Fatalf("fn ran %d times, want 2 (one per distinct key, concurrent calls deduped)", runs.Load())
	}
}
