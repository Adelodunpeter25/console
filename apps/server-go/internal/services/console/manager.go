// Manager owns the console kernel runtime and turns its method inventory
// into harness tools. The console kernel gives the agent a control plane over
// the desktop app: run project scripts, manage ports, spawn sessions, inspect
// and steer them.
package console

import (
	"sync"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/services"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/services/jskernel"
)

// Manager hands out per-session console kernels.
type Manager struct {
	mu      sync.Mutex
	js      *jskernel.Cache[*Runtime]
	stopped bool
	ports   *services.PortRegistry
}

// maxConsoleRuntimes caps live session kernels.
const maxConsoleRuntimes = 16

// NewManager returns a manager for console kernels.
func NewManager() *Manager {
	return &Manager{js: jskernel.NewCache[*Runtime](maxConsoleRuntimes)}
}

// SetPorts attaches the shared port registry used by console.ports.
func (m *Manager) SetPorts(registry *services.PortRegistry) {
	m.mu.Lock()
	m.ports = registry
	m.mu.Unlock()
}

func (m *Manager) portRegistry() *services.PortRegistry {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.ports
}

// sessionRuntime returns the session's console kernel, creating it on first use.
func (m *Manager) sessionRuntime(sessionID, projectID string) (*Runtime, error) {
	return m.js.Get(sessionID, func() (*Runtime, error) {
		ports := m.portRegistry()
		runtime, err := NewRuntime(ports, projectID)
		if err != nil {
			return nil, err
		}
		runtime.SetCancel(m.Stopped)
		return runtime, nil
	})
}

// dropRuntime forgets a session's kernel: variables and bindings are gone,
// and the next call starts fresh. Safe to call for unknown sessions.
func (m *Manager) dropRuntime(sessionID string) {
	m.js.Drop(sessionID)
}

// SessionRuntimeCount reports how many session kernels are live, for tests
// and status surfaces.
func (m *Manager) SessionRuntimeCount() int {
	return m.js.Len()
}

// Stopped reports whether a stop is in force.
func (m *Manager) Stopped() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.stopped
}
