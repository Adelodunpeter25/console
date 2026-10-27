// Package accounts stores the provider account registry behind
// multi-account support: one row per stored OAuth credential per provider,
// plus the single `active` pointer that decides which credential runs use.
//
// Explicit switching only. There is no rotation, no cooldown, no failover —
// the store's entire selection algorithm is the `active` flag. At most one
// account per provider is active; zero active is legal and means the
// provider is disabled until one is selected. The invariant is enforced by
// a partial unique index in the schema, not by hope.
//
// Secrets never live here. Credential payloads stay in 0600 files under
// accounts/<provider>/<id>.json; this store holds display metadata and the
// active flag only, so activation is an atomic DB change rather than file
// surgery. See docs/plan/provider-accounts.md for the design record.
package accounts

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/db"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/utils"
)

// Account is one stored login. CreatedAt/UpdatedAt are unix millis.
type Account struct {
	ID       string
	Provider string
	// Identity is the provider-specific dedupe key (Claude: email,
	// Codex: chatgpt_account_id falling back to email, Antigravity: email).
	// Empty only for legacy-migrated accounts whose credential shape was
	// not parsed at migration time.
	Identity string
	Email    string
	// Label is an optional operator label. Empty means the caller derives
	// a display name from Email.
	Label     string
	Active    bool
	CreatedAt int64
	UpdatedAt int64
}

var (
	// mu serializes store mutations. All exported operations take it, so
	// activate/deselect transactions cannot interleave with each other.
	mu sync.RWMutex
	// gdb is the global-database connection set by Configure. The DDL
	// itself lives in db/schema.go (InitGlobalDB).
	gdb        *sql.DB
	storageDir string
)

// checkDB validates the configured handle is live. A closed one (e.g. a
// test DB that was closed while later tests still run) degrades to
// ErrNotConfigured, so callers fall back to legacy behavior exactly as
// they would with an unconfigured store.
func checkDB() error {
	if gdb == nil {
		return ErrNotConfigured
	}
	if err := gdb.Ping(); err != nil {
		return ErrNotConfigured
	}
	return nil
}

// Configure wires the store to the global database and runs the legacy
// credential migration. Idempotent; safe to call once at boot from
// routes.New. The storage dir comes from the db so dev installs and
// CONSOLE_STORAGE_DIR overrides keep their accounts separate.
func Configure(database *db.DB) error {
	g := database.Global()
	mu.Lock()
	defer mu.Unlock()
	gdb = g
	storageDir = database.StorageDir()
	return migrateLocked()
}

// CredentialPath is the file location for one account's credential payload:
// <storage>/accounts/<provider>/<id>.json. Unconfigured access (tests,
// tools) resolves through ConsoleStorageDir().
func CredentialPath(provider, id string) string {
	mu.RLock()
	defer mu.RUnlock()
	return credentialPathLocked(provider, id)
}

func credentialPathLocked(provider, id string) string {
	dir := storageDir
	if dir == "" {
		dir = utils.ConsoleStorageDir()
	}
	return filepath.Join(dir, "accounts", provider, id+".json")
}

// PinnedError renders the load failure of an account pinned to a run.
// Deleting an account removes its credential file, so a missing file means
// the pin was removed mid-run: the failure must say so and forbid the
// silent fallback to another account that decision 7 rules out. Any other
// error (unreadable, malformed) is returned unchanged.
func PinnedError(provider, id string, err error) error {
	if _, statErr := os.Stat(CredentialPath(provider, id)); os.IsNotExist(statErr) {
		return fmt.Errorf("account %s was removed; select another account for provider %s", id, provider)
	}
	return err
}

// newID mints "acc_" + 16 hex chars — stable, stored, never derived.
func newID() string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		// rand failure is pathological; timestamp keeps ids unique in-process.
		return fmt.Sprintf("acc_%x", time.Now().UnixNano())
	}
	return "acc_" + hex.EncodeToString(b)
}

func nowMS() int64 { return time.Now().UnixMilli() }

// removeCredentialFile drops one account's credential file. A missing file
// is the normal idempotent case, not an error.
func removeCredentialFile(provider, id string) error {
	if err := os.Remove(credentialPathLocked(provider, id)); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}
