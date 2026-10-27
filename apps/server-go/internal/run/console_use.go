// Per-session memory of console kernel activation. Once activated, the
// console tool is available on every run of that session. In memory only,
// like tool groups: a server restart forgets them.
package run

import "sync"

type consoleUseSessions struct {
	mu     sync.Mutex
	marked map[string]bool
}

// mark records an activated session (no-op when already recorded).
func (c *consoleUseSessions) mark(sessionID string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.marked == nil {
		c.marked = map[string]bool{}
	}
	c.marked[sessionID] = true
}

// isActive reports whether a session is marked.
func (c *consoleUseSessions) isActive(sessionID string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.marked[sessionID]
}
