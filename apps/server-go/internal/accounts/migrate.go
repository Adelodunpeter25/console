// Legacy migration: the pre-accounts layout kept exactly one credential
// file per provider (~/.console/claude-creds.json and friends). On the
// first boot after the account store exists, each provider with zero
// account rows and an existing legacy file gains one account whose
// credential file is a copy of the legacy one, marked active.
//
// The legacy file itself stays in place for one release — providers keep
// reading it until account-aware loading lands — so a failed or partial
// migration is harmless and rollback is trivial. Identity/email are
// unknown at migration time (they live inside per-provider credential
// shapes); re-login through the normal flow fills them in via Register.
package accounts

import (
	"fmt"
	"os"
	"path/filepath"
)

// legacyProviders is the set of OAuth providers that had a single
// credential file before accounts. Devin answers 501 and OpenCode stores
// API keys, so neither has legacy credentials to migrate.
var legacyProviders = []string{"claude", "codex", "antigravity"}

// migrateLocked imports legacy credentials for every provider that has no
// accounts yet. Called from Configure with mu held.
func migrateLocked() error {
	for _, provider := range legacyProviders {
		if err := migrateProviderLocked(provider); err != nil {
			return fmt.Errorf("migrate %s: %w", provider, err)
		}
	}
	return nil
}

func migrateProviderLocked(provider string) error {
	var n int
	if err := gdb.QueryRow(
		`SELECT COUNT(*) FROM provider_accounts WHERE provider = ?`, provider,
	).Scan(&n); err != nil {
		return err
	}
	if n > 0 {
		return nil // the provider already has accounts
	}
	legacy, ok := legacyCredentialPath(provider)
	if !ok {
		return nil
	}
	data, err := os.ReadFile(legacy)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // never logged in to this provider: nothing to import
		}
		return err
	}

	// Copy the credential file before inserting the row: a copy failure
	// leaves no account at all, so the next boot retries cleanly. The
	// reverse order would strand a row whose file never materialized.
	id := "acc_legacy_" + provider
	if err := os.MkdirAll(filepath.Dir(credentialPathLocked(provider, id)), 0o700); err != nil {
		return err
	}
	if err := os.WriteFile(credentialPathLocked(provider, id), data, 0o600); err != nil {
		return err
	}
	now := nowMS()
	if _, err := gdb.Exec(
		`INSERT INTO provider_accounts (id, provider, identity, email, label, active, created_at, updated_at)
		 VALUES (?, ?, '', '', ?, 1, ?, ?)`,
		id, provider, "", now, now,
	); err != nil {
		return err
	}
	return nil
}

// legacyCredentialPath resolves the single-credential path a provider uses
// today: its *_CREDENTIALS_PATH override or ~/.console/<file>. This mirrors
// providers/shared.CredentialPath — which pins ~/.console regardless of dev
// mode — rather than importing the providers packages, so providers can
// import the account store without a cycle.
func legacyCredentialPath(provider string) (string, bool) {
	var envKey, fileName string
	switch provider {
	case "claude":
		envKey, fileName = "CLAUDE_CREDENTIALS_PATH", "claude-creds.json"
	case "codex":
		envKey, fileName = "CODEX_CREDENTIALS_PATH", "codex-creds.json"
	case "antigravity":
		envKey, fileName = "ANTIGRAVITY_CREDENTIALS_PATH", "antigravity-creds.json"
	default:
		return "", false
	}
	if p := os.Getenv(envKey); p != "" {
		return p, true
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".console", fileName), true
}
