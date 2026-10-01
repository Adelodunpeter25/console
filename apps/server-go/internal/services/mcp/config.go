// MCP server config store: user-level (not per-project) definitions kept in
// <storageDir>/mcp-servers.json. Holds no secrets — auth entries only carry a
// tokenRef pointing into mcp-credentials.json (see mcp_credentials.go).
package mcp

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"sync"
	"time"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/utils"
)

const (
	TransportHTTP  = "http"
	TransportStdio = "stdio"

	AuthNone   = "none"
	AuthStatic = "static"
	AuthOAuth2 = "oauth2"

	maxLabel = 200
)

var idPattern = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

// AuthConfig says how to authenticate; the secret itself lives in the
// credential store under TokenRef.
type AuthConfig struct {
	Type     string `json:"type"`
	TokenRef string `json:"tokenRef,omitempty"`
}

// EnvMap is a server's environment variables.
//
// It decodes from either shape clients send: the object form this package
// stores on disk and returns (`{"KEY":"VAL"}`), or the pair-array form the
// desktop client posts (`[["KEY","VAL"], ...]`). Accepting both keeps saving
// from the desktop from failing with a 400 over a representation difference
// rather than a real problem. Encoding always produces the object form, so
// stored configs and API responses stay canonical.
type EnvMap map[string]string

// UnmarshalJSON implements json.Unmarshaler.
func (e *EnvMap) UnmarshalJSON(data []byte) error {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 || string(trimmed) == "null" {
		*e = nil
		return nil
	}
	// Object form — the shape this package writes.
	if trimmed[0] == '{' {
		var obj map[string]string
		if err := json.Unmarshal(trimmed, &obj); err != nil {
			return err
		}
		*e = obj
		return nil
	}
	// Pair-array form.
	var pairs [][2]string
	if err := json.Unmarshal(trimmed, &pairs); err != nil {
		return fmt.Errorf("env must be an object of string values or an array of [key, value] pairs: %w", err)
	}
	obj := make(map[string]string, len(pairs))
	for _, pair := range pairs {
		obj[pair[0]] = pair[1]
	}
	*e = obj
	return nil
}

type ServerConfig struct {
	ID            string            `json:"id"`
	Label         string            `json:"label"`
	Transport     string            `json:"transport"`
	URL           string            `json:"url,omitempty"`
	Command       string            `json:"command,omitempty"`
	Args          []string          `json:"args,omitempty"`
	Env           EnvMap            `json:"env,omitempty"`
	Auth          *AuthConfig       `json:"auth,omitempty"`
	TierOverrides map[string]string `json:"tierOverrides,omitempty"`
	Enabled       bool              `json:"enabled"`
	CreatedAt     int64             `json:"createdAt"`
	UpdatedAt     int64             `json:"updatedAt"`
}

type configFile struct {
	Servers map[string]ServerConfig `json:"servers"`
}

// ConfigStore persists server configs to a single JSON file.
type ConfigStore struct {
	mu   sync.Mutex
	path string
}

// NewConfigStore stores under dir; an empty dir uses the console storage dir.
func NewConfigStore(dir string) *ConfigStore {
	if dir == "" {
		dir = utils.ConsoleStorageDir()
	}
	return &ConfigStore{path: filepath.Join(dir, "mcp-servers.json")}
}

// ValidateServer checks a config before it is saved.
func ValidateServer(c ServerConfig) error {
	if !idPattern.MatchString(c.ID) {
		return fmt.Errorf("mcp server id %q must match [A-Za-z0-9_-]+", c.ID)
	}
	if c.Label == "" || len(c.Label) > maxLabel {
		return fmt.Errorf("mcp server %s: label must be a non-empty string", c.ID)
	}
	switch c.Transport {
	case TransportHTTP:
		if c.URL == "" {
			return fmt.Errorf("mcp server %s: url is required for http transport", c.ID)
		}
	case TransportStdio:
		if c.Command == "" {
			return fmt.Errorf("mcp server %s: command is required for stdio transport", c.ID)
		}
	default:
		return fmt.Errorf("mcp server %s: transport must be %q or %q", c.ID, TransportHTTP, TransportStdio)
	}
	if c.Auth != nil {
		switch c.Auth.Type {
		case AuthNone, AuthStatic, AuthOAuth2:
		default:
			return fmt.Errorf("mcp server %s: unknown auth type %q", c.ID, c.Auth.Type)
		}
		if c.Auth.Type != AuthNone && c.Auth.TokenRef == "" {
			return fmt.Errorf("mcp server %s: auth.tokenRef is required for %s auth", c.ID, c.Auth.Type)
		}
	}
	for tool, tier := range c.TierOverrides {
		switch tier {
		case "read", "write", "exec":
		default:
			return fmt.Errorf("mcp server %s: tierOverrides[%s] must be read, write or exec", c.ID, tool)
		}
	}
	return nil
}

func (s *ConfigStore) load() (configFile, error) {
	file := configFile{Servers: map[string]ServerConfig{}}
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
	if file.Servers == nil {
		file.Servers = map[string]ServerConfig{}
	}
	return file, nil
}

func (s *ConfigStore) write(file configFile) error {
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

// List returns all servers sorted by id.
func (s *ConfigStore) List() ([]ServerConfig, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	file, err := s.load()
	if err != nil {
		return nil, err
	}
	out := make([]ServerConfig, 0, len(file.Servers))
	for _, c := range file.Servers {
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

// Get returns one server; ok is false when it doesn't exist.
func (s *ConfigStore) Get(id string) (ServerConfig, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	file, err := s.load()
	if err != nil {
		return ServerConfig{}, false, err
	}
	c, ok := file.Servers[id]
	return c, ok, nil
}

// Save validates and upserts a server, preserving CreatedAt on update.
func (s *ConfigStore) Save(c ServerConfig) (ServerConfig, error) {
	if err := ValidateServer(c); err != nil {
		return ServerConfig{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	file, err := s.load()
	if err != nil {
		return ServerConfig{}, err
	}
	now := time.Now().UnixMilli()
	if prev, ok := file.Servers[c.ID]; ok {
		c.CreatedAt = prev.CreatedAt
	} else {
		c.CreatedAt = now
	}
	c.UpdatedAt = now
	file.Servers[c.ID] = c
	return c, s.write(file)
}

// Delete removes a server; it reports whether one existed.
func (s *ConfigStore) Delete(id string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	file, err := s.load()
	if err != nil {
		return false, err
	}
	if _, ok := file.Servers[id]; !ok {
		return false, nil
	}
	delete(file.Servers, id)
	return true, s.write(file)
}
