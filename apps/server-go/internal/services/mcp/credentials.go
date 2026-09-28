// MCP credential store: secrets for MCP servers, kept apart from
// mcp-servers.json in <storageDir>/mcp-credentials.json (file mode 0600),
// keyed by the tokenRef a server config points at.
package mcp

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/utils"
)

const (
	CredentialStatic = "static"
	CredentialOAuth  = "oauth2"
)

// Credential is one stored secret. Only the fields for its Kind are used.
type Credential struct {
	Kind string `json:"kind"`

	// static: full Authorization header value, e.g. "Bearer x" or "Basic y".
	Header string `json:"header,omitempty"`

	// oauth2: dynamic-registration client id plus the current token pair.
	ClientID     string `json:"clientId,omitempty"`
	AccessToken  string `json:"accessToken,omitempty"`
	RefreshToken string `json:"refreshToken,omitempty"`
	ExpiresAt    int64  `json:"expiresAt,omitempty"`
}

type credentialFile struct {
	Credentials map[string]Credential `json:"credentials"`
}

type CredentialStore struct {
	mu   sync.Mutex
	path string
}

// NewCredentialStore stores under dir; an empty dir uses the console storage dir.
func NewCredentialStore(dir string) *CredentialStore {
	if dir == "" {
		dir = utils.ConsoleStorageDir()
	}
	return &CredentialStore{path: filepath.Join(dir, "mcp-credentials.json")}
}

func (s *CredentialStore) load() (credentialFile, error) {
	file := credentialFile{Credentials: map[string]Credential{}}
	data, err := os.ReadFile(s.path)
	if os.IsNotExist(err) {
		return file, nil
	}
	if err != nil {
		return file, err
	}
	if err := json.Unmarshal(data, &file); err != nil {
		return file, fmt.Errorf("invalid %s: %w", filepath.Base(s.path), err)
	}
	if file.Credentials == nil {
		file.Credentials = map[string]Credential{}
	}
	return file, nil
}

func (s *CredentialStore) write(file credentialFile) error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(file, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, append(data, '\n'), 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}

// Get returns the credential for ref; ok is false when none is stored.
func (s *CredentialStore) Get(ref string) (Credential, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	file, err := s.load()
	if err != nil {
		return Credential{}, false, err
	}
	c, ok := file.Credentials[ref]
	return c, ok, nil
}

// Set stores (or replaces) the credential for ref.
func (s *CredentialStore) Set(ref string, c Credential) error {
	if !idPattern.MatchString(ref) {
		return fmt.Errorf("credential ref %q must match [A-Za-z0-9_-]+", ref)
	}
	switch c.Kind {
	case CredentialStatic:
		if c.Header == "" {
			return fmt.Errorf("static credential %s needs a header value", ref)
		}
	case CredentialOAuth:
	default:
		return fmt.Errorf("credential %s: unknown kind %q", ref, c.Kind)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	file, err := s.load()
	if err != nil {
		return err
	}
	file.Credentials[ref] = c
	return s.write(file)
}

// Delete removes the credential for ref; it reports whether one existed.
// Used for "reset auth" — the next connect starts a fresh registration.
func (s *CredentialStore) Delete(ref string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	file, err := s.load()
	if err != nil {
		return false, err
	}
	if _, ok := file.Credentials[ref]; !ok {
		return false, nil
	}
	delete(file.Credentials, ref)
	return true, s.write(file)
}
