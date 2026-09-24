// Per-session system prompt cache (docs/plan/harness-token-efficiency.md,
// Task 2.2). The prompt is built once per session and reused on every
// turn so the provider's prompt cache stays warm: rebuilding it each turn
// would change the date at midnight or the branch after a checkout and
// invalidate the cached prefix. It is rebuilt only when cwd, model or
// approval mode changes, or after compaction. In memory only: a server
// restart rebuilds it once.
package run

import (
	"sync"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/systemprompt"
)

type promptKey struct {
	cwd, model string
	mode       systemprompt.ApprovalMode
}

type cachedPrompt struct {
	key    promptKey
	result systemprompt.Result
}

type promptCache struct {
	mu      sync.Mutex
	entries map[string]cachedPrompt
}

// get returns the session's cached prompt, building it when missing or
// when cwd/model/mode differ from the cached build.
func (c *promptCache) get(sessionID string, opts systemprompt.BuildOptions) systemprompt.Result {
	key := promptKey{cwd: opts.Cwd, model: opts.Model, mode: opts.ApprovalMode}
	c.mu.Lock()
	defer c.mu.Unlock()
	if entry, ok := c.entries[sessionID]; ok && entry.key == key {
		return entry.result
	}
	result := systemprompt.BuildSystemPrompt(opts)
	if c.entries == nil {
		c.entries = map[string]cachedPrompt{}
	}
	c.entries[sessionID] = cachedPrompt{key: key, result: result}
	return result
}

// forget drops the session's prompt so the next run rebuilds it.
func (c *promptCache) forget(sessionID string) {
	c.mu.Lock()
	delete(c.entries, sessionID)
	c.mu.Unlock()
}
