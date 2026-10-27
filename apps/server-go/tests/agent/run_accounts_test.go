// Run-account pinning end-to-end: a run records the account it authenticates
// as (decision 7 of docs/plan/provider-accounts.md). The provider sees the
// pin resolved once per run; switching or deactivating accounts between runs
// changes new runs only, an env token keeps precedence even with a stored
// active account, and a deleted pin fails loudly instead of falling back.
package tests

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/accounts"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/loop"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/stream"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/db"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/providers/claude"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/run"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/services"
	"github.com/Adelodunpeter25/console/apps/server-go/tests/helpers"
)

// accountCapture records the AccountID of every turn it serves.
type accountCapture struct {
	mu  sync.Mutex
	ids []string
}

func (c *accountCapture) RunTurn(ctx context.Context, req loop.TurnRequest, s *stream.Stream[loop.Event]) error {
	c.mu.Lock()
	c.ids = append(c.ids, req.AccountID)
	c.mu.Unlock()
	s.Push(loop.Event{Kind: loop.EventText, Text: "ok"})
	s.Complete()
	return nil
}

func (c *accountCapture) count() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.ids)
}

func (c *accountCapture) last() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.ids) == 0 {
		return "<none>"
	}
	return c.ids[len(c.ids)-1]
}

// pinAccounts isolates the account store: legacy credential paths point at
// missing temp files (tests/agent's TestMain does not isolate them, so the
// migration would read dev's real files) and the DB lives in this test's dir.
// The DB is closed on cleanup; checkDB then degrades the store to
// ErrNotConfigured so later tests in the package keep legacy behavior.
func pinAccounts(t *testing.T) {
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

// pinRun drives one turn on a fresh session and returns after the run
// settles. want is the number of turn pins the run should have recorded.
func pinRun(t *testing.T, sessions *services.SessionService, svc *run.Service, capture *accountCapture, want int) {
	t.Helper()
	header := helpers.CreateRunSession(t, sessions)
	// A non-generic title suppresses the background title generation, which
	// would make a second, asynchronous provider call pin the same way but
	// keep the assertion nondeterministic.
	if err := sessions.UpdateTitle(header.ID, "Pinned pin"); err != nil {
		t.Fatal(err)
	}
	hub, err := svc.StartRun(header.ID, run.Prompt{Text: "go", Provider: "codex", ModelID: "m"})
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-hub.Done():
	case <-time.After(10 * time.Second):
		t.Fatalf("run did not settle; recorded %d turns, last account %q", capture.count(), capture.last())
	}
	if capture.count() != want {
		t.Fatalf("recorded %d turn pins, want %d", capture.count(), want)
	}
}

func TestRunPinsAccounts(t *testing.T) {
	pinAccounts(t)
	sessions := helpers.NewRunSessions(t)
	svc := run.NewService(sessions)
	capture := &accountCapture{}
	svc.Lookup = func(id string) (loop.Provider, error) {
		return capture, nil
	}

	// First registered account becomes active (Phase 1 semantics).
	a, _, err := accounts.Register("codex", "acct_a", "a@x.dev", "")
	if err != nil {
		t.Fatalf("register a: %v", err)
	}
	time.Sleep(2 * time.Millisecond)
	b, _, err := accounts.Register("codex", "acct_b", "b@x.dev", "")
	if err != nil {
		t.Fatalf("register b: %v", err)
	}

	// Run 1 resolves the active account A and pins it for the run.
	pinRun(t, sessions, svc, capture, 1)
	if got := capture.last(); got != a.ID {
		t.Fatalf("run 1 pin = %q, want %q", got, a.ID)
	}

	// Switch to B: the finished run kept A; the next run resolves B.
	if err := accounts.Activate("codex", b.ID); err != nil {
		t.Fatalf("activate b: %v", err)
	}
	pinRun(t, sessions, svc, capture, 2)
	if got := capture.last(); got != b.ID {
		t.Fatalf("run 2 pin = %q, want %q", got, b.ID)
	}

	// Zero active: the provider is disabled — runs resolve the unpinned
	// (empty) credential, not a leftover.
	if err := accounts.Select("codex", b.ID, false); err != nil {
		t.Fatalf("deselect b: %v", err)
	}
	pinRun(t, sessions, svc, capture, 3)
	if got := capture.last(); got != "" {
		t.Fatalf("run 3 pin = %q, want \"\"", got)
	}

	// While an env token owns the provider, even an active stored account
	// loses: runs keep the ordinary loader (env precedence, §4.4).
	if err := accounts.Activate("codex", b.ID); err != nil {
		t.Fatalf("re-activate b: %v", err)
	}
	t.Setenv("OPENAI_CODEX_OAUTH_TOKEN", "env-token")
	pinRun(t, sessions, svc, capture, 4)
	if got := capture.last(); got != "" {
		t.Fatalf("env-owned run pin = %q, want \"\"", got)
	}
}

// A run pinned to an account that no longer exists must fail loudly saying
// so, not silently fall back to another credential (decision 7). The load
// fails before any HTTP happens, so a stub-less provider is safe to use.
func TestRunDeletedPinFailsLoudly(t *testing.T) {
	pinAccounts(t)
	p := &claude.Provider{}
	events := stream.New[loop.Event]()
	err := p.RunTurn(context.Background(), loop.TurnRequest{Model: "m", AccountID: "acc_unknown"}, events)
	for {
		_, evErr, ok := events.Next()
		if !ok {
			break
		}
		if err == nil {
			err = evErr
		}
	}
	if err == nil {
		t.Fatal("pinned load of unknown account must fail")
	}
	if !strings.Contains(err.Error(), "was removed") || !strings.Contains(err.Error(), "acc_unknown") {
		t.Fatalf("error must name the removed pin, got: %v", err)
	}
}
