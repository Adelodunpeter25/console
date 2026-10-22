package visualize

import (
	"sync"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/tools"
)

// Loader is what /visualize invokes to bring the tool into a run. It mirrors
// cua.Loader: a run that never invoked the command never sees the tool. The
// tool itself is stateless, but the loader keeps the activation path and the
// session boundary in one place for future additions.
type Loader struct {
	sessionID string
	mu        sync.Mutex
	loaded    bool
}

// NewLoader returns a loader bound to one chat session.
func NewLoader(sessionID string) *Loader {
	return &Loader{sessionID: sessionID}
}

// Loaded reports whether the tool has been brought into some run.
func (l *Loader) Loaded() bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.loaded
}

// Load appends the visualize tool to registry and marks the group loaded.
func (l *Loader) Load(registry *tools.Registry) (int, error) {
	registry.Add(NewTool())
	l.mu.Lock()
	l.loaded = true
	l.mu.Unlock()
	return 1, nil
}
