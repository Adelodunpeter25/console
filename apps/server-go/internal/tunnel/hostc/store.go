package hostc

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// Store persists a tunnel so a restart can resume the same public URL.
//
// The persisted tunnel carries the id and the connect token, which is what
// lets the client reconnect to the tunnel the server still holds instead of
// creating a new one with a new URL.
type Store interface {
	// Load returns the persisted tunnel, or nil when there is none.
	Load() (*Tunnel, error)
	// Save persists the tunnel.
	Save(tunnel *Tunnel) error
	// Clear forgets the tunnel.
	Clear() error
}

// FileStore keeps the tunnel as a JSON file.
type FileStore struct {
	path string
}

// NewFileStore stores the tunnel in the file at path.
func NewFileStore(path string) *FileStore {
	return &FileStore{path: path}
}

// Path is the file the tunnel is kept in.
func (s *FileStore) Path() string { return s.path }

// Load returns the persisted tunnel, or nil when there is none. A file that
// cannot be read is an error, except when it simply is not there.
func (s *FileStore) Load() (*Tunnel, error) {
	data, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("hostc: read the tunnel file: %w", err)
	}
	if len(data) == 0 {
		return nil, nil
	}
	var tunnel Tunnel
	if err := json.Unmarshal(data, &tunnel); err != nil {
		return nil, fmt.Errorf("hostc: the tunnel file is corrupt: %w", err)
	}
	if tunnel.ID == "" || tunnel.ConnectURL == "" || tunnel.Token == "" {
		return nil, nil
	}
	return &tunnel, nil
}

// Save persists the tunnel. The file holds a bearer token, so it is written
// for the owner only.
func (s *FileStore) Save(tunnel *Tunnel) error {
	if tunnel == nil {
		return s.Clear()
	}
	data, err := json.MarshalIndent(tunnel, "", "  ")
	if err != nil {
		return fmt.Errorf("hostc: encode the tunnel: %w", err)
	}
	if dir := filepath.Dir(s.path); dir != "" {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return fmt.Errorf("hostc: create the tunnel directory: %w", err)
		}
	}
	// Write to a temporary file and rename, so a crash mid-write cannot
	// leave a half-written token behind.
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return fmt.Errorf("hostc: write the tunnel file: %w", err)
	}
	if err := os.Rename(tmp, s.path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("hostc: replace the tunnel file: %w", err)
	}
	return nil
}

// Clear forgets the tunnel.
func (s *FileStore) Clear() error {
	if err := os.Remove(s.path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("hostc: remove the tunnel file: %w", err)
	}
	return nil
}
