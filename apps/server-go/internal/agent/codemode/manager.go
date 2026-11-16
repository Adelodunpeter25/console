package codemode

import (
	"sync"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/services/jskernel"
)

// Manager owns per-session codemode runtimes.
type Manager struct {
	mu    sync.Mutex
	cache *jskernel.Cache[*Runtime]
}

// maxCodeRuntimes caps live codemode session kernels.
const maxCodeRuntimes = 16

// NewManager returns a manager for session codemode runtimes.
func NewManager() *Manager {
	return &Manager{
		cache: jskernel.NewCache[*Runtime](maxCodeRuntimes),
	}
}

// Get returns the persistent runtime for a session, updating its caller.
func (m *Manager) Get(sessionID string, caller Caller) (*Runtime, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	rt, err := m.cache.Get(sessionID, func() (*Runtime, error) {
		return NewRuntime(caller)
	})
	if err != nil {
		return nil, err
	}
	rt.SetCaller(caller)
	return rt, nil
}

// Drop forgets one session's runtime.
func (m *Manager) Drop(sessionID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.cache.Drop(sessionID)
}
