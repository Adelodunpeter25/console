// Per-session memory of computer-use activation. Typing /computer-use marks
// the session; every later run of that session restores the two computer
// tools up front, so they stay callable across user messages. In memory only,
// like tool groups: a server restart forgets them, and the user can invoke
// the command again.
package run

import "sync"

type computerUseSessions struct {
	mu     sync.Mutex
	marked map[string]bool
}

// mark records an activated session (no-op when already recorded).
func (c *computerUseSessions) mark(sessionID string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.marked == nil {
		c.marked = map[string]bool{}
	}
	c.marked[sessionID] = true
}

// isActive reports whether the session invoked /computer-use.
func (c *computerUseSessions) isActive(sessionID string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.marked[sessionID]
}
