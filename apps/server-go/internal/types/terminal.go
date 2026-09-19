// Terminal WebSocket message types, matching the Bun server protocol.
package types

// TerminalSpawnParams arrive as query params on the upgrade request:
// /api/terminals?cwd=...&cols=...&rows=...&shell=...&label=...&proto=binary
type TerminalSpawnParams struct {
	Cwd    string
	Shell  string
	Cols   int
	Rows   int
	Label  string
	Binary bool
}

// TerminalClientMessage frames (JSON text or binary [tag, ...payload]).
type TerminalClientMessage struct {
	Type string `json:"type"` // input | resize | kill
	Data string `json:"data,omitempty"`
	Cols int    `json:"cols,omitempty"`
	Rows int    `json:"rows,omitempty"`
}

type TerminalSpawnedMessage struct {
	Type  string `json:"type"` // "spawned"
	ID    string `json:"id"`
	Cwd   string `json:"cwd"`
	Shell string `json:"shell"`
	Label string `json:"label,omitempty"`
	Cols  int    `json:"cols"`
	Rows  int    `json:"rows"`
}

type TerminalErrorMessage struct {
	Type    string `json:"type"` // "error"
	Message string `json:"message"`
}

type TerminalExitMessage struct {
	Type string `json:"type"` // "exit"
	Code int    `json:"code"`
}
