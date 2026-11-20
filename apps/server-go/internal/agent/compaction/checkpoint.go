package compaction

import (
	"crypto/sha256"
	"encoding/hex"
	"sync"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/loop"
)

// Checkpoint keeps the result of one compaction so later turns reuse the exact
// same compacted prefix and only append new messages. Recompacting from the
// full history every turn moves the cut point each time, which rewrites the
// provider's cached prefix on every request; with a checkpoint the cache breaks
// once, when compaction happens, and stays stable until the next threshold
// crossing.
type Checkpoint struct {
	mu        sync.Mutex
	covered   int
	signature string
	prefix    []any
}

// Apply returns the checkpointed prefix followed by the messages added since,
// or false when there is no checkpoint or history no longer matches what it
// covered (rewind, edit, fork); a mismatching checkpoint is dropped.
func (c *Checkpoint) Apply(history []any) ([]any, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.prefix == nil {
		return nil, false
	}
	if len(history) < c.covered || shape(history[:c.covered]) != c.signature {
		c.prefix, c.covered, c.signature = nil, 0, ""
		return nil, false
	}
	out := make([]any, 0, len(c.prefix)+len(history)-c.covered)
	out = append(out, c.prefix...)
	return append(out, history[c.covered:]...), true
}

// Set records that compacted replaces all of history.
func (c *Checkpoint) Set(history, compacted []any) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.prefix = append([]any{}, compacted...)
	c.covered = len(history)
	c.signature = shape(history)
}

// shape fingerprints the stable parts of a history: message kinds and user
// text. Assistant and tool content is left out on purpose, because the same
// message decodes to different Go shapes in memory and from storage.
func shape(history []any) string {
	h := sha256.New()
	for _, m := range history {
		switch msg := m.(type) {
		case loop.UserMessage:
			h.Write([]byte("u:" + msg.Content + "\x00"))
		case *loop.UserMessage:
			if msg != nil {
				h.Write([]byte("u:" + msg.Content + "\x00"))
			}
		case loop.AssistantMessage, *loop.AssistantMessage:
			h.Write([]byte("a\x00"))
		case loop.ToolResultMessage, *loop.ToolResultMessage:
			h.Write([]byte("t\x00"))
		default:
			h.Write([]byte("?\x00"))
		}
	}
	return hex.EncodeToString(h.Sum(nil))
}
