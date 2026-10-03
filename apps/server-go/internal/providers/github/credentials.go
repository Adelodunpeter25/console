// GitHub git credentials. Unlike the subscription providers (codex, claude,
// antigravity) these are not LLM session tokens: they exist purely so the
// git CLI can authenticate against private GitHub repositories from this box
// — both for agent-spawned git (GitService, bash tool) and for interactive
// shells (PTY terminals).
//
// Storage mirrors the provider credential files (dir 0700, file 0600) but the
// path resolves through utils.ConsoleStorageDir so dev installs and
// CONSOLE_STORAGE_DIR overrides get their own file. Username and scopes are
// cached here at accept time, which keeps auth status a purely local read —
// the shared GET /api/auth/status endpoint never makes a network call.
package github

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/providers/shared"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/utils"
)

// Credential is the on-disk shape of the credential file. The token is the
// only secret here and must never be logged, returned to a client, or placed
// in a URL.
type Credential struct {
	Token     string    `json:"token"`
	Username  string    `json:"username,omitempty"`
	Scopes    []string  `json:"scopes,omitempty"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// CredentialPath resolves the credential file: GITHUB_CREDENTIALS_PATH,
// otherwise <console storage>/github-creds.json.
func CredentialPath() string {
	return utils.GitHubCredentialsPath()
}

// LoadCredential reads the credential file. A missing file is an error, so
// callers should fall back to CredentialExists.
func LoadCredential() (Credential, error) {
	raw, err := os.ReadFile(CredentialPath())
	if err != nil {
		return Credential{}, err
	}
	var cred Credential
	dec := json.NewDecoder(bytes.NewReader(bytes.TrimSpace(raw)))
	if err := dec.Decode(&cred); err != nil {
		return Credential{}, err
	}
	return cred, nil
}

// SaveCredential writes the credential file (file 0600).
//
// The parent directory is created 0700 by shared.SaveCredentialFile, but
// MkdirAll leaves the mode of a directory that already exists — and in
// production ~/.console is created by other subsystems with the process
// umask, which is typically 0755. That is a weaker posture than the other
// credential files imply, so the mode is re-asserted here. The 0600 file is
// what actually protects the token; this is defense in depth.
func SaveCredential(cred Credential) error {
	if strings.TrimSpace(cred.Token) == "" {
		return errNoToken
	}
	cred.Token = strings.TrimSpace(cred.Token)
	cred.Username = strings.TrimPrefix(strings.TrimSpace(cred.Username), "@")
	cred.UpdatedAt = time.Now().UTC()
	if err := os.MkdirAll(filepath.Dir(CredentialPath()), 0o700); err != nil {
		return err
	}
	if err := os.Chmod(filepath.Dir(CredentialPath()), 0o700); err != nil {
		return err
	}
	return shared.SaveCredentialFile(CredentialPath(), cred)
}

// CredentialExists reports whether a usable token is stored. A file holding an
// empty token reads as "not connected" so a truncated write cannot leave the
// UI claiming a connection that git cannot use.
func CredentialExists() bool {
	cred, err := LoadCredential()
	return err == nil && strings.TrimSpace(cred.Token) != ""
}

// ClearCredential deletes the credential file. A missing file is success, so
// disconnect is idempotent.
func ClearCredential() error {
	if err := os.Remove(CredentialPath()); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}
