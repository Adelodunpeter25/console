// The provider account store (docs/plan/provider-accounts.md Phase 1):
// CRUD, the one-active-per-provider invariant, deactivate-to-zero,
// delete-active promotion, register dedupe, and the legacy credential
// migration.
package tests

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/accounts"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/db"
)

// newAccounts builds an isolated account store. Every legacy credential
// override points at a file that does not exist, so migration falls through
// unless a test itself plants a fixture (mirrors the TestMain isolation:
// the developer's real ~/.console credentials must stay out of tests).
func newAccounts(t *testing.T) *db.DB {
	t.Helper()
	dir := t.TempDir()
	for env, name := range map[string]string{
		"CLAUDE_CREDENTIALS_PATH":      "claude-creds.json",
		"CODEX_CREDENTIALS_PATH":       "codex-creds.json",
		"ANTIGRAVITY_CREDENTIALS_PATH": "antigravity-creds.json",
	} {
		t.Setenv(env, filepath.Join(dir, "no-legacy", name))
	}
	database, err := db.Open(db.OpenOptions{Path: ":memory:", StorageDir: dir})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { database.Close() })
	if err := accounts.Configure(database); err != nil {
		t.Fatalf("configure accounts: %v", err)
	}
	return database
}

func TestAccountsRegisterActivatesFirstOnly(t *testing.T) {
	newAccounts(t)

	first, created, err := accounts.Register("claude", "work@x.io", "work@x.io", "")
	if err != nil || !created {
		t.Fatalf("first register: created=%v err=%v", created, err)
	}
	if !first.Active {
		t.Error("first account should be active with an empty store")
	}

	second, created, err := accounts.Register("claude", "me@x.io", "me@x.io", "")
	if err != nil || !created {
		t.Fatalf("second register: created=%v err=%v", created, err)
	}
	if second.Active {
		t.Error("adding must not steal active from the existing account")
	}
	selected, ok, err := accounts.Enabled("claude")
	if err != nil || !ok || selected.ID != first.ID {
		t.Fatalf("active account = %v (ok=%v, err=%v), want first", selected, ok, err)
	}
}

func TestAccountsRegisterDedupesByIdentity(t *testing.T) {
	newAccounts(t)

	stored, created, err := accounts.Register("codex", "acct_123", "", "")
	if err != nil || !created {
		t.Fatalf("register: created=%v err=%v", created, err)
	}
	// Same identity: update in place, not a new row.
	dup, created, err := accounts.Register("codex", "acct_123", "new_email@x.io", "Live")
	if err != nil {
		t.Fatalf("re-register: %v", err)
	}
	if created || dup.ID != stored.ID {
		t.Fatalf("identity re-register must dedupe: created=%v id=%s", created, dup.ID)
	}
	if dup.Email != "new_email@x.io" || dup.Label != "Live" {
		t.Fatalf("re-register did not update in place: %+v", dup)
	}
	list, err := accounts.List("codex")
	if err != nil || len(list) != 1 {
		t.Fatalf("after dedupe, list has %d rows (err=%v)", len(list), err)
	}
}

func TestAccountsRegisterEmailFallbackWhenNoIdentity(t *testing.T) {
	newAccounts(t)

	if _, created, err := accounts.Register("antigravity", "", "a@x.io", ""); err != nil || !created {
		t.Fatalf("register: created=%v err=%v", created, err)
	}
	// No identity: email is the dedupe key.
	if _, created, err := accounts.Register("antigravity", "", "a@x.io", ""); err != nil || created {
		t.Fatalf("email re-register should dedupe: created=%v err=%v", created, err)
	}
}

func TestAccountsUniqueActiveInvariant(t *testing.T) {
	database := newAccounts(t)

	// The store API cannot produce two actives; prove the DB enforces it
	// even against raw writes (the partial unique index is the guard).
	_, err := database.Global().Exec(
		`INSERT INTO provider_accounts (id, provider, active, created_at, updated_at)
		 VALUES ('a1', 'claude', 1, 1, 1), ('a2', 'claude', 1, 2, 2)`,
	)
	if err == nil {
		t.Fatal("two active accounts for one provider must violate the unique index")
	}
}

func TestAccountsActivateAndDeselect(t *testing.T) {
	newAccounts(t)

	first, _, _ := accounts.Register("claude", "a@x.io", "a@x.io", "")
	second, _, _ := accounts.Register("claude", "b@x.io", "b@x.io", "")

	if err := accounts.Activate("claude", second.ID); err != nil {
		t.Fatalf("activate second: %v", err)
	}
	selected, _, _ := accounts.Enabled("claude")
	if selected.ID != second.ID {
		t.Fatalf("active = %s, want second", selected.ID)
	}

	// Self-activate is an idempotent no-op.
	if err := accounts.Activate("claude", second.ID); err != nil {
		t.Fatalf("re-activate same account: %v", err)
	}

	// Unknown id.
	if err := accounts.Activate("claude", "acc_none"); !errors.Is(err, accounts.ErrNotFound) {
		t.Fatalf("activate unknown = %v, want ErrNotFound", err)
	}

	// Deselect-to-zero is legal: the provider is disabled until a pick.
	if err := accounts.Select("claude", second.ID, false); err != nil {
		t.Fatalf("deselect: %v", err)
	}
	if _, ok, err := accounts.Enabled("claude"); err != nil || ok {
		t.Fatalf("after deselect, ok=%v err=%v, want zero active", ok, err)
	}
	if id, err := accounts.EnabledID("claude"); err != nil || id != "" {
		t.Fatalf("EnabledID after deselect = %q (err=%v), want empty", id, err)
	}
	// Deselecting again is a no-op, and re-selecting works.
	if err := accounts.Select("claude", second.ID, false); err != nil {
		t.Fatalf("double deselect: %v", err)
	}
	if err := accounts.Select("claude", first.ID, true); err != nil {
		t.Fatalf("re-select after zero: %v", err)
	}
	selected, _, _ = accounts.Enabled("claude")
	if selected.ID != first.ID {
		t.Fatalf("active = %s, want first", selected.ID)
	}
}

func TestAccountsDeletePromotesNext(t *testing.T) {
	newAccounts(t)

	first, _, _ := accounts.Register("claude", "a@x.io", "a@x.io", "")
	// Ms-resolution created_at means registrations inside one millisecond
	// tie; sleep so "oldest first" promotion is unambiguous.
	time.Sleep(2 * time.Millisecond)
	if _, created, _ := accounts.Register("claude", "b@x.io", "b@x.io", ""); !created {
		t.Fatal("expected second account")
	}
	time.Sleep(2 * time.Millisecond)
	if _, created, _ := accounts.Register("claude", "c@x.io", "c@x.io", ""); !created {
		t.Fatal("expected third account")
	}

	// Deleting the active account promotes the oldest remaining.
	if err := accounts.Delete("claude", first.ID); err != nil {
		t.Fatalf("delete active: %v", err)
	}
	selected, ok, err := accounts.Enabled("claude")
	if err != nil || !ok || selected.Email != "b@x.io" {
		t.Fatalf("after delete, active = %+v (ok=%v err=%v), want the second account", selected, ok, err)
	}

	// Deleting again is idempotent.
	if err := accounts.Delete("claude", first.ID); err != nil {
		t.Fatalf("delete twice must succeed: %v", err)
	}
}

func TestAccountsDeleteLastLeadsLoggedOut(t *testing.T) {
	newAccounts(t)

	only, _, _ := accounts.Register("codex", "", "x@x.io", "")
	if err := accounts.Delete("codex", only.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if id, err := accounts.EnabledID("codex"); err != nil || id != "" {
		t.Fatalf("EnabledID = %q (err=%v), want logged-out", id, err)
	}
	list, err := accounts.List("codex")
	if err != nil || len(list) != 0 {
		t.Fatalf("list after deleting all = %v (err=%v), want empty", list, err)
	}
}

func TestAccountsDeleteRemovesCredentialFile(t *testing.T) {
	newAccounts(t)

	stored, _, _ := accounts.Register("antigravity", "a@x.io", "a@x.io", "")
	path := accounts.CredentialPath("antigravity", stored.ID)
	// Register is metadata-only; the directory appears when credential
	// code first writes a file (phase 2). Create it here to seed one.
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(`{"token":"t"}`), 0o600); err != nil {
		t.Fatalf("seed credential file: %v", err)
	}
	if err := accounts.Delete("antigravity", stored.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("credential file should be gone (stat err=%v)", err)
	}
}

func TestAccountsAllGroupsByProvider(t *testing.T) {
	newAccounts(t)

	if _, _, err := accounts.Register("claude", "a@x.io", "a@x.io", ""); err != nil {
		t.Fatalf("register claude: %v", err)
	}
	if _, _, err := accounts.Register("codex", "", "c@x.io", ""); err != nil {
		t.Fatalf("register codex: %v", err)
	}
	all, err := accounts.All()
	if err != nil {
		t.Fatalf("all: %v", err)
	}
	if len(all["claude"]) != 1 || len(all["codex"]) != 1 {
		t.Fatalf("grouped = %+v", all)
	}
	if _, ok := all["antigravity"]; ok {
		t.Error("provider with zero accounts must be absent")
	}
}

func TestAccountsMigrateLegacyFile(t *testing.T) {
	dir := t.TempDir()
	legacy := filepath.Join(dir, "claude-creds.json")
	body := `{"accessToken":"tok","refreshToken":"ref"}`
	if err := os.WriteFile(legacy, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CLAUDE_CREDENTIALS_PATH", legacy)
	t.Setenv("CODEX_CREDENTIALS_PATH", filepath.Join(dir, "no-codex.json"))
	t.Setenv("ANTIGRAVITY_CREDENTIALS_PATH", filepath.Join(dir, "no-antigravity.json"))

	database, err := db.Open(db.OpenOptions{Path: ":memory:", StorageDir: dir})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { database.Close() })
	if err := accounts.Configure(database); err != nil {
		t.Fatalf("configure: %v", err)
	}

	list, err := accounts.List("claude")
	if err != nil || len(list) != 1 {
		t.Fatalf("migration should create one claude account, got %v (err=%v)", list, err)
	}
	acct := list[0]
	if acct.ID != "acc_legacy_claude" || !acct.Active {
		t.Fatalf("legacy account = %+v, want active acc_legacy_claude", acct)
	}

	// The credential file was copied byte-for-byte; the legacy file stays.
	copied, err := os.ReadFile(accounts.CredentialPath("claude", acct.ID))
	if err != nil {
		t.Fatalf("copied credential file: %v", err)
	}
	if string(copied) != body {
		t.Fatalf("copied credential = %q, want %q", copied, body)
	}
	if kept, err := os.ReadFile(legacy); err != nil || string(kept) != body {
		t.Fatalf("legacy file must be untouched: %q (err=%v)", kept, err)
	}
	// Re-running Configure is a no-op: idempotent migration.
	if err := accounts.Configure(database); err != nil {
		t.Fatalf("re-configure: %v", err)
	}
	if list2, _ := accounts.List("claude"); len(list2) != 1 {
		t.Fatalf("re-migration duplicated rows: %d", len(list2))
	}
}

func TestAccountsMigrateSkipsProvidersWithAccounts(t *testing.T) {
	dir := t.TempDir()
	legacy := filepath.Join(dir, "claude-creds.json")
	if err := os.WriteFile(legacy, []byte(`{"accessToken":"tok"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CLAUDE_CREDENTIALS_PATH", legacy)
	t.Setenv("CODEX_CREDENTIALS_PATH", filepath.Join(dir, "no-codex.json"))
	t.Setenv("ANTIGRAVITY_CREDENTIALS_PATH", filepath.Join(dir, "no-antigravity.json"))

	// First boot: the user adds a fresh account before any migration ran
	// (the store was configured against a different legacy state). The DB
	// is file-backed so the reopen below is a true second boot.
	dbPath := filepath.Join(dir, "global.db")
	database, err := db.Open(db.OpenOptions{Path: dbPath, StorageDir: dir})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := accounts.Configure(database); err != nil {
		t.Fatalf("configure: %v", err)
	}
	if len(mustList(t, "claude")) != 1 {
		t.Fatalf("first configure should migrate exactly one account")
	}
	database.Close()

	// Second boot on the same store: the row set already exists, so the
	// legacy file (still present) must not be imported again.
	database2, err := db.Open(db.OpenOptions{Path: dbPath, StorageDir: dir})
	if err != nil {
		t.Fatalf("reopen db: %v", err)
	}
	defer database2.Close()
	if err := accounts.Configure(database2); err != nil {
		t.Fatalf("re-configure: %v", err)
	}
	if list := mustList(t, "claude"); len(list) != 1 {
		t.Fatalf("second boot duplicated rows: %d", len(list))
	}
}

func mustList(t *testing.T, provider string) []accounts.Account {
	t.Helper()
	list, err := accounts.List(provider)
	if err != nil {
		t.Fatalf("list %s: %v", provider, err)
	}
	return list
}

// The unconfigured CredentialPath fallback (EnvConsoleStorageDir) can't be
// unit-tested without resetting the package global: once any test calls
// Configure, storageDir wins by design. The fallback is two lines delegating
// to utils.ConsoleStorageDir, which carries its own tests.
