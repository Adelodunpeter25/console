// Loader is what activates the console kernel to bring the console tools into
// a run. It is separate from Manager so the tools load lazily.
package console

import (
	"sync"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/tools"
)

// Loader binds the console tools to one chat session: the session's kernel
// keeps its variables apart from every other session's.
type Loader struct {
	manager   *Manager
	sessionID string
	projectID string
	cwd       string
	mu        sync.Mutex
	loaded    bool
}

// NewLoader returns a loader bound to manager and one session.
func NewLoader(manager *Manager, sessionID, projectID, cwd string) *Loader {
	return &Loader{manager: manager, sessionID: sessionID, projectID: projectID, cwd: cwd}
}

// Loaded reports whether the tools have been brought into some run.
func (l *Loader) Loaded() bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.loaded
}

// Load appends the console tools to registry and marks the group loaded.
func (l *Loader) Load(registry *tools.Registry) (int, error) {
	registry.Add(
		NewConsoleTool(l.manager, l.sessionID, l.projectID, l.cwd),
		NewConsoleResetTool(l.manager, l.sessionID),
	)
	l.mu.Lock()
	l.loaded = true
	l.mu.Unlock()
	return 2, nil
}
