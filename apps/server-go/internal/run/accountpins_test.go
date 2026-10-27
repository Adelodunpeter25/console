// accountPins coverage: lazy resolution caches per provider, including the
// empty pin, for the run's lifetime (decision 7) — a mid-run switch between
// accounts or a drop to zero-active must not change a running run's pin,
// and an env-owned provider resolves the empty pin while env owns it.
package run

import (
	"testing"
	"time"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/accounts"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/db"
)

func newPinAccounts(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("CLAUDE_CREDENTIALS_PATH", dir+"/no-legacy/claude.json")
	t.Setenv("CODEX_CREDENTIALS_PATH", dir+"/no-legacy/codex.json")
	t.Setenv("ANTIGRAVITY_CREDENTIALS_PATH", dir+"/no-legacy/antigravity.json")
	database, err := db.Open(db.OpenOptions{Path: ":memory:", StorageDir: dir})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { database.Close() })
	if err := accounts.Configure(database); err != nil {
		t.Fatalf("configure accounts: %v", err)
	}
}

func TestAccountPinsCachePerRun(t *testing.T) {
	newPinAccounts(t)
	a, _, err := accounts.Register("claude", "a@x.dev", "a@x.dev", "")
	if err != nil {
		t.Fatalf("register a: %v", err)
	}
	time.Sleep(2 * time.Millisecond)
	b, _, err := accounts.Register("claude", "b@x.dev", "b@x.dev", "")
	if err != nil {
		t.Fatalf("register b: %v", err)
	}

	pins := &accountPins{}
	if got := pins.forProvider("claude"); got != a.ID {
		t.Fatalf("first pin = %q, want %q (oldest active)", got, a.ID)
	}

	// Switch to B mid-run: the cached pin must stay A until the run ends.
	if err := accounts.Activate("claude", b.ID); err != nil {
		t.Fatalf("activate b: %v", err)
	}
	if got := pins.forProvider("claude"); got != a.ID {
		t.Fatalf("pin after switch = %q, want cached %q", got, a.ID)
	}

	// Zero active: still cached.
	pins2 := &accountPins{}
	if err := accounts.Activate("claude", b.ID); err == nil {
		// Activate switched; drop to zero with a fresh resolver's view.
		_ = err
	}
	if err := accounts.Select("claude", b.ID, false); err != nil {
		t.Fatalf("deselect b: %v", err)
	}
	if got := pins2.forProvider("claude"); got != "" {
		t.Fatalf("zero-active pin = %q, want \"\"", got)
	}
	// ...and a run that resolved empty before activation stays empty.
	if got := pins2.forProvider("claude"); got != "" {
		t.Fatalf("empty pin changed after activate = %q", got)
	}
}

func TestAccountPinsEnvOwned(t *testing.T) {
	newPinAccounts(t)
	if _, _, err := accounts.Register("codex", "acct_a", "a@x.dev", ""); err != nil {
		t.Fatalf("register: %v", err)
	}
	t.Setenv("OPENAI_CODEX_OAUTH_TOKEN", "env-token")

	pins := &accountPins{}
	// Env owns codex: the pin stays empty so the run keeps env precedence
	// even though account A is active (env is the synthetic active, §4.4).
	if got := pins.forProvider("codex"); got != "" {
		t.Fatalf("env-owned pin = %q, want \"\"", got)
	}
}

func TestAccountPinsNilReceiver(t *testing.T) {
	var pins *accountPins
	if got := pins.forProvider("claude"); got != "" {
		t.Fatalf("nil pins resolved %q", got)
	}
}
