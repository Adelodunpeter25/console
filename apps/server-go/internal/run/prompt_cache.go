// Per-session system prompt cache (docs/plan/harness-token-efficiency.md,
// Task 2.2). The prompt is built once per session and reused on every
// turn so the provider's prompt cache stays warm: rebuilding it each turn
// would invalidate the cached prefix. The prompt holds only session-stable
// facts (no date, no git branch — the model looks those up when needed), so
// it is rebuilt only when cwd or model changes, or after compaction. The
// session's setup text is frozen with it: whatever changes later (mode,
// finished background work, newly enabled capabilities) rides at the request
// tail instead, so the cached prefix stays byte-identical. In memory only: a server restart rebuilds it once.
package run

import (
	"slices"
	"sync"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/systemprompt"
)

type promptKey struct {
	cwd, model string
}

type cachedPrompt struct {
	key    promptKey
	result systemprompt.Result
	// setup is the frozen setup text and setupCaps the capabilities whose
	// references it contains; setupSet is false until built.
	setup     string
	setupCaps []string
	setupSet  bool
}

type promptCache struct {
	mu      sync.Mutex
	entries map[string]cachedPrompt
}

// get returns the session's cached prompt, building it when missing or
// when cwd/model/mode differ from the cached build.
func (c *promptCache) get(sessionID string, opts systemprompt.BuildOptions) systemprompt.Result {
	key := promptKey{cwd: opts.Cwd, model: opts.Model}
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

// frozenSetup returns the session's setup text. It is built once and reused
// byte-for-byte; the only rebuild trigger is a change in the set of enabled
// capabilities (one planned miss), never per-turn content.
func (c *promptCache) frozenSetup(sessionID string, caps []string, build func() string) string {
	c.mu.Lock()
	defer c.mu.Unlock()
	entry, ok := c.entries[sessionID]
	if !ok {
		return build()
	}
	if !entry.setupSet || !slices.Equal(entry.setupCaps, caps) {
		entry.setup, entry.setupCaps, entry.setupSet = build(), caps, true
		c.entries[sessionID] = entry
	}
	return entry.setup
}

// forget drops the session's prompt so the next run rebuilds it.
func (c *promptCache) forget(sessionID string) {
	c.mu.Lock()
	delete(c.entries, sessionID)
	c.mu.Unlock()
}
