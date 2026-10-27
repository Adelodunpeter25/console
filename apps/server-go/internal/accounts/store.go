// Store operations: list, read the active pointer, activate/deselect,
// delete (with next-account promotion), and register (the login-callback
// entry point). Every mutation runs in a transaction; the partial unique
// index idx_provider_accounts_one_active makes a double-active state
// impossible even under a bug or a concurrent write.
package accounts

import (
	"database/sql"
	"errors"
	"fmt"
)

// ErrNotFound is returned when an operation names an account id the store
// does not hold. Route handlers map it to 404.
var ErrNotFound = errors.New("account not found")

// ErrNotConfigured is returned when the store is used before Configure.
// With Configure failing only at boot, this is a wiring bug, not a
// user-facing state.
var ErrNotConfigured = errors.New("account store not configured")

const accountColumns = "id, provider, identity, email, label, active, created_at, updated_at"

// List reports every stored account for one provider, oldest first. An
// unknown provider is an empty list, not an error.
func List(provider string) ([]Account, error) {
	mu.RLock()
	defer mu.RUnlock()
	if gdb == nil {
		return nil, ErrNotConfigured
	}
	rows, err := gdb.Query(
		`SELECT `+accountColumns+` FROM provider_accounts WHERE provider = ? ORDER BY created_at, id`,
		provider,
	)
	if err != nil {
		return nil, err
	}
	return scanAccounts(rows)
}

// All reports every stored account grouped by provider. Providers with no
// accounts are absent from the map.
func All() (map[string][]Account, error) {
	mu.RLock()
	defer mu.RUnlock()
	if gdb == nil {
		return nil, ErrNotConfigured
	}
	rows, err := gdb.Query(`SELECT ` + accountColumns + ` FROM provider_accounts ORDER BY created_at, id`)
	if err != nil {
		return nil, err
	}
	all, err := scanAccounts(rows)
	if err != nil {
		return nil, err
	}
	grouped := make(map[string][]Account)
	for _, a := range all {
		grouped[a.Provider] = append(grouped[a.Provider], a)
	}
	return grouped, nil
}

// Enabled returns the provider's active account, if any. Missing (zero
// active) is a false ok, not an error — the expected state right after a
// logout or an explicit deselect.
func Enabled(provider string) (Account, bool, error) {
	mu.RLock()
	defer mu.RUnlock()
	if gdb == nil {
		return Account{}, false, ErrNotConfigured
	}
	var a Account
	var active int
	err := gdb.QueryRow(
		`SELECT `+accountColumns+` FROM provider_accounts WHERE provider = ? AND active = 1`,
		provider,
	).Scan(&a.ID, &a.Provider, &a.Identity, &a.Email, &a.Label, &active, &a.CreatedAt, &a.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Account{}, false, nil
	}
	if err != nil {
		return Account{}, false, err
	}
	a.Active = active == 1
	return a, true, nil
}

// EnabledID is Enabled's narrow form for load paths that only need the id.
// Empty string means no active account.
func EnabledID(provider string) (string, error) {
	a, ok, err := Enabled(provider)
	if err != nil || !ok {
		return "", err
	}
	return a.ID, nil
}

// Activate selects one account and deselects every other account of the
// provider, in a single transaction. Selecting the already-active account
// is a no-op upgrade, not an error. Unknown ids return ErrNotFound.
func Activate(provider, id string) error {
	mu.Lock()
	defer mu.Unlock()
	if gdb == nil {
		return ErrNotConfigured
	}
	now := nowMS()
	return runTx(func(tx *sql.Tx) error {
		var n int
		if err := tx.QueryRow(
			`SELECT COUNT(*) FROM provider_accounts WHERE provider = ? AND id = ?`, provider, id,
		).Scan(&n); err != nil {
			return err
		}
		if n == 0 {
			return ErrNotFound
		}
		if _, err := tx.Exec(
			`UPDATE provider_accounts SET active = 0, updated_at = ? WHERE provider = ? AND active = 1`,
			now, provider,
		); err != nil {
			return err
		}
		_, err := tx.Exec(
			`UPDATE provider_accounts SET active = 1, updated_at = ? WHERE provider = ? AND id = ?`,
			now, provider, id,
		)
		return err
	})
}

// Select stores an explicit active state for one account. active=true is a
// full switch (Activate); active=false deselects only this account.
// Deactivating the only active account is allowed and leaves the provider
// with zero active accounts — the documented "needs a selection" state.
func Select(provider, id string, active bool) error {
	if active {
		return Activate(provider, id)
	}
	mu.Lock()
	defer mu.Unlock()
	if gdb == nil {
		return ErrNotConfigured
	}
	now := nowMS()
	return runTx(func(tx *sql.Tx) error {
		var n int
		if err := tx.QueryRow(
			`SELECT COUNT(*) FROM provider_accounts WHERE provider = ? AND id = ?`, provider, id,
		).Scan(&n); err != nil {
			return err
		}
		if n == 0 {
			return ErrNotFound
		}
		// The WHERE keeps already-inactive rows untouched: deselect is a
		// no-op for them, not an updated_at churn.
		_, err := tx.Exec(
			`UPDATE provider_accounts SET active = 0, updated_at = ? WHERE provider = ? AND id = ? AND active = 1`,
			now, provider, id,
		)
		return err
	})
}

// Delete removes one account. Idempotent: deleting an id the store does
// not hold succeeds without effect, mirroring the GitHub logout route.
// Deleting the active account promotes the next one (oldest first); with
// none remaining the provider returns to logged-out. The account's
// credential file is removed as well.
func Delete(provider, id string) error {
	mu.Lock()
	defer mu.Unlock()
	if gdb == nil {
		return ErrNotConfigured
	}
	var wasActive bool
	err := runTx(func(tx *sql.Tx) error {
		var active int
		err := tx.QueryRow(
			`SELECT active FROM provider_accounts WHERE provider = ? AND id = ?`, provider, id,
		).Scan(&active)
		if errors.Is(err, sql.ErrNoRows) {
			return nil // idempotent: nothing to do
		}
		if err != nil {
			return err
		}
		wasActive = active == 1
		if _, err := tx.Exec(
			`DELETE FROM provider_accounts WHERE provider = ? AND id = ?`, provider, id,
		); err != nil {
			return err
		}
		if !wasActive {
			return nil
		}
		// Promote the oldest remaining account, deterministically. With no
		// rows left this is a no-op and the provider shows logged-out.
		_, err = tx.Exec(
			`UPDATE provider_accounts SET active = 1, updated_at = ?
			 WHERE id = (SELECT id FROM provider_accounts WHERE provider = ? ORDER BY created_at, id LIMIT 1)`,
			nowMS(), provider,
		)
		return err
	})
	if err != nil {
		return err
	}
	return removeCredentialFile(provider, id)
}

// Register stores a credential's account, deduped by the provider identity
// key (falling back to email when the caller has no identity). A match
// updates display fields in place — active is untouched, so a re-login can
// never switch runs out from under the user (decision 3). A non-match
// creates a new account that becomes active only when the provider had no
// active account at all. Returns the stored account and whether it was
// newly created.
func Register(provider, identity, email, label string) (Account, bool, error) {
	if provider == "" {
		return Account{}, false, fmt.Errorf("accounts: provider is required")
	}
	mu.Lock()
	defer mu.Unlock()
	if gdb == nil {
		return Account{}, false, ErrNotConfigured
	}
	now := nowMS()
	var stored Account
	created := false
	err := runTx(func(tx *sql.Tx) error {
		existing, err := lookupForRegister(tx, provider, identity, email)
		if err != nil {
			return err
		}
		if existing.ID != "" {
			updated := existing
			if email != "" {
				updated.Email = email
			}
			if identity != "" && existing.Identity == "" {
				updated.Identity = identity
			}
			if label != "" {
				updated.Label = label
			}
			updated.UpdatedAt = now
			_, err = tx.Exec(
				`UPDATE provider_accounts SET email = ?, identity = ?, label = ?, updated_at = ?
				 WHERE provider = ? AND id = ?`,
				updated.Email, updated.Identity, updated.Label, now, provider, updated.ID,
			)
			if err != nil {
				return err
			}
			stored = updated
			return nil
		}

		var nActive int
		if err := tx.QueryRow(
			`SELECT COUNT(*) FROM provider_accounts WHERE provider = ? AND active = 1`, provider,
		).Scan(&nActive); err != nil {
			return err
		}
		id := newID()
		active := 0
		if nActive == 0 {
			active = 1
		}
		if _, err := tx.Exec(
			`INSERT INTO provider_accounts (id, provider, identity, email, label, active, created_at, updated_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			id, provider, identity, email, label, active, now, now,
		); err != nil {
			return err
		}
		stored = Account{
			ID: id, Provider: provider, Identity: identity, Email: email, Label: label,
			Active: active == 1, CreatedAt: now, UpdatedAt: now,
		}
		created = true
		return nil
	})
	if err != nil {
		return Account{}, false, err
	}
	return stored, created, nil
}

// lookupForRegister finds an existing account to dedupe against: identity
// match first; only when the caller has no identity at all does email
// serve as the key, matching the per-provider identity table (Claude and
// Antigravity: email; Codex: chatgpt_account_id falling back to email). An
// authoritative identity never merges on an email, and no identity is never
// silently merged into an unrelated row.
func lookupForRegister(tx *sql.Tx, provider, identity, email string) (Account, error) {
	var query string
	var args []any
	switch {
	case identity != "":
		query = `SELECT ` + accountColumns + ` FROM provider_accounts WHERE provider = ? AND identity = ?`
		args = []any{provider, identity}
	case email != "":
		query = `SELECT ` + accountColumns + ` FROM provider_accounts WHERE provider = ? AND email = ?`
		args = []any{provider, email}
	default:
		return Account{}, nil
	}
	var a Account
	var active int
	err := tx.QueryRow(query, args...).Scan(
		&a.ID, &a.Provider, &a.Identity, &a.Email, &a.Label, &active, &a.CreatedAt, &a.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return Account{}, nil
	}
	if err != nil {
		return Account{}, err
	}
	a.Active = active == 1
	return a, nil
}

// scanAccounts drains a query over full account rows.
func scanAccounts(rows *sql.Rows) ([]Account, error) {
	defer rows.Close()
	var out []Account
	for rows.Next() {
		var a Account
		var active int
		if err := rows.Scan(
			&a.ID, &a.Provider, &a.Identity, &a.Email, &a.Label, &active, &a.CreatedAt, &a.UpdatedAt,
		); err != nil {
			return nil, err
		}
		a.Active = active == 1
		out = append(out, a)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// runTx runs fn inside a transaction, rolling back on error and committing
// when fn returns nil.
func runTx(fn func(tx *sql.Tx) error) error {
	tx, err := gdb.Begin()
	if err != nil {
		return err
	}
	if err := fn(tx); err != nil {
		tx.Rollback()
		return err
	}
	return tx.Commit()
}
