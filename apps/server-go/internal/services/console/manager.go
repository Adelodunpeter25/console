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
	mu       sync.Mutex
	js       *jskernel.Cache[*Runtime]
	stopped  bool
	ports    *services.PortRegistry
	scripts  *services.ProjectScriptsService
	sessions *services.SessionService
	projects *services.ProjectService
	active   func(string) bool
	start    func(string, string, string, string, string, string) error
	abort    func(string) bool
	prompt   PromptFunc
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

// SetScripts attaches the project script service used by console.scripts.
func (m *Manager) SetScripts(scripts *services.ProjectScriptsService) {
	m.mu.Lock()
	m.scripts = scripts
	m.mu.Unlock()
}

func (m *Manager) scriptService() *services.ProjectScriptsService {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.scripts
}

// SetSessions attaches session and project services plus run hooks used by the
// console session control surface.
func (m *Manager) SetSessions(sessions *services.SessionService, projects *services.ProjectService, active func(string) bool, abort func(string) bool, start func(string, string, string, string, string, string) error, prompt PromptFunc) {
	m.mu.Lock()
	m.sessions, m.projects, m.active, m.abort, m.start, m.prompt = sessions, projects, active, abort, start, prompt
	m.mu.Unlock()
}

func (m *Manager) sessionConfig() (*services.SessionService, *services.ProjectService, func(string) bool, func(string) bool, func(string, string, string, string, string, string) error, PromptFunc) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.sessions, m.projects, m.active, m.abort, m.start, m.prompt
}

// sessionRuntime returns the session's console kernel, creating it on first use.
func (m *Manager) sessionRuntime(sessionID, projectID, cwd string) (*Runtime, error) {
	return m.js.Get(sessionID, func() (*Runtime, error) {
		ports := m.portRegistry()
		scripts := m.scriptService()
		sessions, projects, active, abort, start, prompt := m.sessionConfig()
		runtime, err := NewRuntime(ports, scripts, sessions, projects, active, start, abort, prompt, projectID, cwd)
		if err != nil {
			return nil, err
		}
		runtime.sessionID = sessionID
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
