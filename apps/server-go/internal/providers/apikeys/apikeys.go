// Provider API keys: the generic pasted-credential store behind
// PUT/DELETE /api/providers/:id/credential.
//
// This is deliberately *not* an OAuth credential. Those (codex, claude,
// antigravity) are session tokens this server obtained through a browser flow
// and refreshes on its own. An API key is a secret the owner typed once and
// nobody will renew, and it is the same shape for every provider that accepts
// one — so it gets one generic endpoint instead of a route per provider.
//
// Two properties matter more than the shape:
//
//   - The key lives here, on the server. Clients are conduits: a key pasted on
//     a phone reaches the same file as one pasted on desktop, because the
//     server is the only party that ever sends it upstream. Storing it in a
//     client keystore would leave the server needing a second copy anyway.
//   - Reads never return the key. `Status` answers "configured" and "where it
//     came from", never a value or a prefix — a masked prefix still leaks
//     length and lets an attacker confirm a guess.
package apikeys

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/utils"
)

// ErrEmptyKey rejects a blank key. A stored empty string would read as
// "configured" to the UI while sending an empty bearer upstream.
var ErrEmptyKey = errors.New("an API key is required")

// ErrUnknownProvider rejects a provider id that is not in the catalog, so a
// typo cannot silently create a credential nothing will ever read.
var ErrUnknownProvider = errors.New("unknown provider")

// Entry is one provider's stored key.
type Entry struct {
	Value     string    `json:"value"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// store is the on-disk shape. The map is keyed by provider id so a new provider
// needs no schema change.
type store struct {
	Keys map[string]Entry `json:"keys"`
}

// Path resolves the store file: CONSOLE_PROVIDER_KEYS_PATH wins (tests,
// unusual deployments), otherwise <console storage>/provider-keys.json.
func Path() string {
	if override := os.Getenv("CONSOLE_PROVIDER_KEYS_PATH"); override != "" {
		return override
	}
	return filepath.Join(utils.ConsoleStorageDir(), "provider-keys.json")
}

// Load reads the store. A missing file is an empty store, not an error, so
// "never configured" is the normal first-run state rather than a failure.
func Load() (map[string]Entry, error) {
	raw, err := os.ReadFile(Path())
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]Entry{}, nil
		}
		return nil, err
	}
	var s store
	if err := json.Unmarshal(raw, &s); err != nil {
		// A corrupt file must not take the provider down; it also must not be
		// silently overwritten, or one bad write would destroy the user's keys.
		return nil, fmt.Errorf("parse provider keys: %w", err)
	}
	if s.Keys == nil {
		s.Keys = map[string]Entry{}
	}
	return s.Keys, nil
}

// save writes the store (dir 0700, file 0600). Like the GitHub credential
// writer, the directory mode is re-asserted because in production ~/.console
// is created by other subsystems at the process umask, which is typically
// 0755 — a weaker posture than a credential file implies.
func save(keys map[string]Entry) error {
	dir := filepath.Dir(Path())
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(store{Keys: keys}, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(Path(), append(data, '\n'), 0o600)
}

// Set stores a key for a provider, replacing any previous one.
func Set(provider, key string) error {
	key = strings.TrimSpace(key)
	if key == "" {
		return ErrEmptyKey
	}
	keys, err := Load()
	if err != nil {
		return err
	}
	keys[provider] = Entry{Value: key, UpdatedAt: time.Now().UTC()}
	return save(keys)
}

// Get returns the stored key for a provider. ok is false when none is stored
// or the stored value is blank.
func Get(provider string) (key string, ok bool) {
	keys, err := Load()
	if err != nil {
		return "", false
	}
	entry, found := keys[provider]
	if !found || strings.TrimSpace(entry.Value) == "" {
		return "", false
	}
	return entry.Value, true
}

// Has reports whether a usable key is stored for a provider. A truncated write
// leaving an empty value reads as "not configured" so the UI cannot claim a
// credential that upstream calls will reject.
func Has(provider string) bool {
	_, ok := Get(provider)
	return ok
}

// Clear removes a provider's key. Removing one that is not set is success, so
// a disconnect button is idempotent. Other providers' keys are untouched.
func Clear(provider string) error {
	keys, err := Load()
	if err != nil {
		return err
	}
	if _, found := keys[provider]; !found {
		return nil
	}
	delete(keys, provider)
	return save(keys)
}

// Entry returns a provider's stored entry for status reporting.
func EntryFor(provider string) (Entry, bool) {
	keys, err := Load()
	if err != nil {
		return Entry{}, false
	}
	entry, found := keys[provider]
	if !found || strings.TrimSpace(entry.Value) == "" {
		return Entry{}, false
	}
	return entry, true
}