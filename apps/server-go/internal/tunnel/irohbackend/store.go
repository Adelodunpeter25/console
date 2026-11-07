package irohbackend

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

const (
	fileKey  = "iroh.key"  // 32-byte secret key, 0600
	fileInfo = "iroh.json" // node ID, written after bind
)

// NodeInfo is what pairing needs from the tunnel dir.
type NodeInfo struct {
	// NodeID is the 64-char hex identity peers dial by.
	NodeID string `json:"nodeId"`
}

// Store keeps the endpoint keypair and node info in a directory (0700,
// files 0600), like the relay and opentunnel stores.
type Store struct{ Dir string }

// loadInfo reads the stored node ID, if any.
func (s Store) loadInfo() (*NodeInfo, error) {
	data, err := os.ReadFile(filepath.Join(s.Dir, fileInfo))
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("iroh: read %s: %w", fileInfo, err)
	}
	var info NodeInfo
	if err := json.Unmarshal(data, &info); err != nil {
		return nil, fmt.Errorf("iroh: %s is corrupt: %w", fileInfo, err)
	}
	return &info, nil
}

// LoadKey returns the stored 32-byte key, or nil when there is none.
func (s Store) LoadKey() ([]byte, error) {
	key, err := os.ReadFile(filepath.Join(s.Dir, fileKey))
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("iroh: read key: %w", err)
	}
	if len(key) != sha256.Size {
		return nil, fmt.Errorf("iroh: %s is corrupt: %d bytes, want 32", fileKey, len(key))
	}
	return key, nil
}

// LoadOrCreate returns the pinned key — creating one on the first run —
// plus the stored info, which the manager updates once the node ID is
// known. A stable key is what makes the node ID survive restarts.
func (s Store) LoadOrCreate() ([]byte, NodeInfo, error) {
	key, err := s.LoadKey()
	if err != nil {
		return nil, NodeInfo{}, err
	}
	if key != nil {
		info, _ := s.loadInfo()
		if info == nil {
			info = &NodeInfo{}
		}
		return key, *info, nil
	}
	fresh := make([]byte, sha256.Size)
	if _, err := rand.Read(fresh); err != nil {
		return nil, NodeInfo{}, fmt.Errorf("iroh: key entropy: %w", err)
	}
	if err := os.MkdirAll(s.Dir, 0o700); err != nil {
		return nil, NodeInfo{}, fmt.Errorf("iroh: create %s: %w", s.Dir, err)
	}
	if err := os.WriteFile(filepath.Join(s.Dir, fileKey), fresh, 0o600); err != nil {
		return nil, NodeInfo{}, fmt.Errorf("iroh: write key: %w", err)
	}
	return fresh, NodeInfo{}, nil
}

// SaveInfo stores the node ID next to the key.
func (s Store) SaveInfo(info NodeInfo) error {
	if err := os.MkdirAll(s.Dir, 0o700); err != nil {
		return fmt.Errorf("iroh: create %s: %w", s.Dir, err)
	}
	data, err := json.MarshalIndent(info, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(s.Dir, fileInfo), data, 0o600)
}

// ReadInfo reports the node ID the directory holds, if any. It is the
// shape tunnel.ReadInfo consumes, so it belongs to the package, not the
// store instance.
func ReadInfo(dir string) (NodeInfo, bool) {
	data, err := os.ReadFile(filepath.Join(dir, fileInfo))
	if err != nil {
		return NodeInfo{}, false
	}
	var info NodeInfo
	if json.Unmarshal(data, &info) != nil || info.NodeID == "" {
		return NodeInfo{}, false
	}
	return info, true
}

// Delete forgets the keypair and node ID together, so a reset is atomic:
// no key without a node ID, no node ID without a key.
func (s Store) Delete() error {
	for _, name := range []string{fileKey, fileInfo} {
		if err := os.Remove(filepath.Join(s.Dir, name)); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}
