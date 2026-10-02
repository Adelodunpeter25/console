// Per-session memory of lazily loaded tool groups (MCP servers). Once the
// model loads a group in a session, every later run of that session restores
// it up front, so the tools stay callable across user messages and history
// that references them keeps working. Restored groups are appended in load
// order, keeping the tool-list prefix stable for the provider's prompt
// cache. In memory only: a server restart forgets them, and the model can
// call loadTools again.
package run

import "sync"

type toolGroups struct {
	mu      sync.Mutex
	entries map[string][]string
}

// list returns the session's loaded groups in load order.
func (g *toolGroups) list(sessionID string) []string {
	g.mu.Lock()
	defer g.mu.Unlock()
	return append([]string(nil), g.entries[sessionID]...)
}

// add records a loaded group (no-op when already recorded).
func (g *toolGroups) add(sessionID, group string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	for _, existing := range g.entries[sessionID] {
		if existing == group {
			return
		}
	}
	if g.entries == nil {
		g.entries = map[string][]string{}
	}
	g.entries[sessionID] = append(g.entries[sessionID], group)
}

// mcpSetupHint is appended to the per-session setup message when MCP
// servers are enabled, so the model knows external tools exist.
const mcpSetupHint = "# External tools\nExternal tool groups (MCP servers) are available through the `loadTools` tool. Its description lists each group and its tools."
