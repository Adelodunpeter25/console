package cfrelay

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	fileIdentity = "relay.json"
	fileToken    = "relay-token"
)

type identityFile struct {
	Name string `json:"name"`
	URL  string `json:"url"`
	API  string `json:"api"`
}

// Identity is a relay name this machine owns. The name, and so the URL,
// survives restarts because the token is kept on disk.
type Identity struct {
	Name  string
	URL   string
	API   string
	Token string
}

// Info is what the CLI shows an operator, read without touching the token.
type Info struct {
	URL string
	// Ready is true once a name exists; there is no certificate to wait for.
	Ready bool
}

// Store keeps one relay identity in a directory (0700, files 0600).
type Store struct{ Dir string }

func (s Store) path(name string) string { return filepath.Join(s.Dir, name) }

// ReadInfo reports the relay address the directory holds, if any.
func ReadInfo(dir string) (Info, bool) {
	data, err := os.ReadFile(filepath.Join(dir, fileIdentity))
	if err != nil {
		return Info{}, false
	}
	var f identityFile
	if json.Unmarshal(data, &f) != nil || f.URL == "" {
		return Info{}, false
	}
	return Info{URL: f.URL, Ready: true}, true
}

func (s Store) writeFile(name string, data []byte) error {
	if err := os.MkdirAll(s.Dir, 0o700); err != nil {
		return fmt.Errorf("cfrelay: create %s: %w", s.Dir, err)
	}
	tmp := s.path(name + ".tmp")
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return fmt.Errorf("cfrelay: write %s: %w", name, err)
	}
	if err := os.Rename(tmp, s.path(name)); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("cfrelay: replace %s: %w", name, err)
	}
	return nil
}

// Save stores the identity. The token goes first so an identity file never
// exists without its token.
func (s Store) Save(id Identity) error {
	if err := s.writeFile(fileToken, []byte(id.Token)); err != nil {
		return err
	}
	data, err := json.MarshalIndent(identityFile{Name: id.Name, URL: id.URL, API: id.API}, "", "  ")
	if err != nil {
		return err
	}
	return s.writeFile(fileIdentity, data)
}

// Load reads the stored identity, or nil when there is none.
func (s Store) Load() (*Identity, error) {
	data, err := os.ReadFile(s.path(fileIdentity))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("cfrelay: read %s: %w", fileIdentity, err)
	}
	var f identityFile
	if err := json.Unmarshal(data, &f); err != nil || f.Name == "" || f.URL == "" {
		return nil, fmt.Errorf("cfrelay: %s is corrupt", fileIdentity)
	}
	token, err := os.ReadFile(s.path(fileToken))
	if err != nil {
		return nil, fmt.Errorf("cfrelay: read token: %w", err)
	}
	return &Identity{Name: f.Name, URL: f.URL, API: f.API, Token: strings.TrimSpace(string(token))}, nil
}

// Clear forgets the identity. Only used when the relay no longer accepts it.
func (s Store) Clear() error {
	for _, name := range []string{fileIdentity, fileToken} {
		if err := os.Remove(s.path(name)); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}
