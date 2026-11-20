package loop

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/tools"
)

// PrefixTracker remembers the head of the last request per conversation, so a
// request whose cached prefix differs can be logged with the part that changed.
// A change is expected after compaction and when capabilities are switched on;
// anywhere else it is a cache miss worth investigating.
type PrefixTracker struct {
	mu   sync.Mutex
	last map[string]prefixSnapshot
}

type prefixSnapshot struct {
	system, setup, tools string
	messages             []string
}

const prefixTrackerMax = 256

// Observe records this request and returns what changed since the previous one
// for the same key: "" when the prefix is unchanged (or first seen), else one
// of "system", "setup", "tools", "messages@<index>", "messages shrank".
func (p *PrefixTracker) Observe(key, system, setup string, defs []tools.Definition, history []any) string {
	cur := prefixSnapshot{system: digest(system), setup: digest(setup), tools: digestJSON(defs)}
	cur.messages = make([]string, len(history))
	for i, m := range history {
		cur.messages[i] = digestJSON(m)
	}

	p.mu.Lock()
	defer p.mu.Unlock()
	if p.last == nil || len(p.last) >= prefixTrackerMax {
		p.last = map[string]prefixSnapshot{}
	}
	prev, seen := p.last[key]
	p.last[key] = cur
	if !seen {
		return ""
	}
	switch {
	case prev.system != cur.system:
		return "system"
	case prev.tools != cur.tools:
		return "tools"
	case prev.setup != cur.setup:
		return "setup"
	case len(cur.messages) < len(prev.messages):
		return "messages shrank"
	}
	for i := range prev.messages {
		if prev.messages[i] != cur.messages[i] {
			return fmt.Sprintf("messages@%d", i)
		}
	}
	return ""
}

func digest(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:6])
}

func digestJSON(v any) string {
	raw, err := json.Marshal(v)
	if err != nil {
		return "unhashable"
	}
	return digest(string(raw))
}
