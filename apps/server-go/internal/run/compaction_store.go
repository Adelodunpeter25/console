package run

import (
	"sync"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/compaction"
)

type compactionStore struct {
	mu      sync.Mutex
	entries map[string]*compaction.Checkpoint
}

// checkpoint returns the session's checkpoint, creating it on first use.
func (c *compactionStore) checkpoint(sessionID string) *compaction.Checkpoint {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.entries == nil {
		c.entries = map[string]*compaction.Checkpoint{}
	}
	cp, ok := c.entries[sessionID]
	if !ok {
		cp = &compaction.Checkpoint{}
		c.entries[sessionID] = cp
	}
	return cp
}
