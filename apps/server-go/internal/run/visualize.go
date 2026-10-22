// Per-session memory of /visualize activation. Typing /visualize marks the
// session; every later run of that session restores the tool and skill, so the
// model keeps the capability across user messages. In memory only, like
// computer-use: a server restart forgets activation, and the user can invoke
// the command again. Persisted visual artifacts still render.
package run

import "sync"

type visualizeSessions struct {
	mu     sync.Mutex
	marked map[string]bool
}

// mark records an activated session (no-op when already recorded).
func (v *visualizeSessions) mark(sessionID string) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.marked == nil {
		v.marked = map[string]bool{}
	}
	v.marked[sessionID] = true
}

// isActive reports whether the session invoked /visualize.
func (v *visualizeSessions) isActive(sessionID string) bool {
	v.mu.Lock()
	defer v.mu.Unlock()
	return v.marked[sessionID]
}
