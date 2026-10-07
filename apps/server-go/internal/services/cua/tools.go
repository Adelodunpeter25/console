// Loader is what /computer-use invokes to bring the two computer tools into
// a run. It is separate from Manager so the tools load lazily, and so a run
// that never invoked the command never sees a computer-use tool at all.
package cua

import (
	"sync"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/tools"
)

// Loader binds the computer tools to one chat session: the session's kernel
// keeps its variables apart from every other session's.
type Loader struct {
	manager   *Manager
	sessionID string
	mu        sync.Mutex
	loaded    bool
}

// NewLoader returns a loader bound to manager and one session.
func NewLoader(manager *Manager, sessionID string) *Loader {
	return &Loader{manager: manager, sessionID: sessionID}
}

// Loaded reports whether the tools have been brought into some run.
func (l *Loader) Loaded() bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.loaded
}

// Load appends the computer tools to registry and marks the group loaded. The
// driver must be present: with no library there is nothing to drive, and the
// caller reports that instead of loading an empty group.
func (l *Loader) Load(registry *tools.Registry) (int, error) {
	if _, err := l.manager.Driver(); err != nil {
		return 0, &ErrNoDriver{Cause: err}
	}
	registry.Add(
		NewComputerTool(l.manager, l.sessionID),
		NewComputerResetTool(l.manager, l.sessionID),
	)
	l.mu.Lock()
	l.loaded = true
	l.mu.Unlock()
	return 2, nil
}
