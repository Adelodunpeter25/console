package codemode

import (
	"sync"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/permissions"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/services/jskernel"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/services/mcp"
)

// Manager owns per-session codemode runtimes.
type Manager struct {
	mu    sync.Mutex
	cache *jskernel.Cache[*Runtime]
	opts  Options
}

// maxCodeRuntimes caps live codemode session kernels.
const maxCodeRuntimes = 16

// NewManager returns a manager for session codemode runtimes. The optional mcp
// manager and mode supplier are installed as mcp.* methods in every runtime.
func NewManager(mcpManager *mcp.Manager, mode func() permissions.Mode) *Manager {
	return &Manager{
		cache: jskernel.NewCache[*Runtime](maxCodeRuntimes),
		opts:  Options{MCP: mcpManager, Mode: mode},
	}
}

// Get returns the persistent runtime for a session, updating its caller.
func (m *Manager) Get(sessionID string, caller Caller) (*Runtime, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	rt, err := m.cache.Get(sessionID, func() (*Runtime, error) {
		return NewRuntime(caller, m.opts)
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
