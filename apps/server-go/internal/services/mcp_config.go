// MCP server config store: user-level (not per-project) definitions kept in
// <storageDir>/mcp-servers.json. Holds no secrets — auth entries only carry a
// tokenRef pointing into mcp-credentials.json (see mcp_credentials.go).
package services

import (
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
	MCPTransportHTTP  = "http"
	MCPTransportStdio = "stdio"

	MCPAuthNone   = "none"
	MCPAuthStatic = "static"
	MCPAuthOAuth2 = "oauth2"

	mcpMaxLabel = 200
)

var mcpIDPattern = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

// MCPAuthConfig says how to authenticate; the secret itself lives in the
// credential store under TokenRef.
type MCPAuthConfig struct {
	Type     string `json:"type"`
	TokenRef string `json:"tokenRef,omitempty"`
}

type MCPServerConfig struct {
	ID            string            `json:"id"`
	Label         string            `json:"label"`
	Transport     string            `json:"transport"`
	URL           string            `json:"url,omitempty"`
	Command       string            `json:"command,omitempty"`
	Args          []string          `json:"args,omitempty"`
	Env           map[string]string `json:"env,omitempty"`
	Auth          *MCPAuthConfig    `json:"auth,omitempty"`
	TierOverrides map[string]string `json:"tierOverrides,omitempty"`
	Enabled       bool              `json:"enabled"`
	CreatedAt     int64             `json:"createdAt"`
	UpdatedAt     int64             `json:"updatedAt"`
}

type mcpConfigFile struct {
	Servers map[string]MCPServerConfig `json:"servers"`
}

// MCPConfigStore persists server configs to a single JSON file.
type MCPConfigStore struct {
	mu   sync.Mutex
	path string
}

// NewMCPConfigStore stores under dir; an empty dir uses the console storage dir.
func NewMCPConfigStore(dir string) *MCPConfigStore {
	if dir == "" {
		dir = utils.ConsoleStorageDir()
	}
	return &MCPConfigStore{path: filepath.Join(dir, "mcp-servers.json")}
}

// ValidateMCPServer checks a config before it is saved.
func ValidateMCPServer(c MCPServerConfig) error {
	if !mcpIDPattern.MatchString(c.ID) {
		return fmt.Errorf("mcp server id %q must match [A-Za-z0-9_-]+", c.ID)
	}
	if c.Label == "" || len(c.Label) > mcpMaxLabel {
		return fmt.Errorf("mcp server %s: label must be a non-empty string", c.ID)
	}
	switch c.Transport {
	case MCPTransportHTTP:
		if c.URL == "" {
			return fmt.Errorf("mcp server %s: url is required for http transport", c.ID)
		}
	case MCPTransportStdio:
		if c.Command == "" {
			return fmt.Errorf("mcp server %s: command is required for stdio transport", c.ID)
		}
	default:
		return fmt.Errorf("mcp server %s: transport must be %q or %q", c.ID, MCPTransportHTTP, MCPTransportStdio)
	}
	if c.Auth != nil {
		switch c.Auth.Type {
		case MCPAuthNone, MCPAuthStatic, MCPAuthOAuth2:
		default:
			return fmt.Errorf("mcp server %s: unknown auth type %q", c.ID, c.Auth.Type)
		}
		if c.Auth.Type != MCPAuthNone && c.Auth.TokenRef == "" {
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

func (s *MCPConfigStore) load() (mcpConfigFile, error) {
	file := mcpConfigFile{Servers: map[string]MCPServerConfig{}}
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
		file.Servers = map[string]MCPServerConfig{}
	}
	return file, nil
}

func (s *MCPConfigStore) write(file mcpConfigFile) error {
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
func (s *MCPConfigStore) List() ([]MCPServerConfig, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	file, err := s.load()
	if err != nil {
		return nil, err
	}
	out := make([]MCPServerConfig, 0, len(file.Servers))
	for _, c := range file.Servers {
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

// Get returns one server; ok is false when it doesn't exist.
func (s *MCPConfigStore) Get(id string) (MCPServerConfig, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	file, err := s.load()
	if err != nil {
		return MCPServerConfig{}, false, err
	}
	c, ok := file.Servers[id]
	return c, ok, nil
}

// Save validates and upserts a server, preserving CreatedAt on update.
func (s *MCPConfigStore) Save(c MCPServerConfig) (MCPServerConfig, error) {
	if err := ValidateMCPServer(c); err != nil {
		return MCPServerConfig{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	file, err := s.load()
	if err != nil {
		return MCPServerConfig{}, err
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
func (s *MCPConfigStore) Delete(id string) (bool, error) {
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
