// Project script types ported from
// apps/server/api/src/services/project-scripts/types.ts.
package types

type ProjectScript struct {
	ID         string  `json:"id"`
	Label      string  `json:"label"`
	Command    string  `json:"command"`
	Shortcut   *string `json:"shortcut"`
	Persistent bool    `json:"persistent"`
}

type ProjectScriptsResult struct {
	ProjectID string          `json:"projectId"`
	Scripts   []ProjectScript `json:"scripts"`
	Source    string          `json:"source"` // "console.toml" | "missing"
}

type ScriptRunStatus string // running | succeeded | failed | stopped

type ScriptRun struct {
	RunID      string          `json:"runId"`
	ProjectID  string          `json:"projectId"`
	ScriptID   string          `json:"scriptId"`
	Label      string          `json:"label"`
	Persistent bool            `json:"persistent"`
	Status     ScriptRunStatus `json:"status"`
	StartedAt  string          `json:"startedAt"`
	EndedAt    *string         `json:"endedAt"`
	ExitCode   *int            `json:"exitCode"`
	Stdout     string          `json:"stdout"`
	Stderr     string          `json:"stderr"`
}

type ScriptRunEvent struct {
	Type string `json:"type"` // status | output | exit
	// status event
	Status ScriptRunStatus `json:"status,omitempty"`
	// output event
	Stream string `json:"stream,omitempty"` // stdout | stderr
	Text   string `json:"text,omitempty"`
	// exit event
	ExitCode *int `json:"exitCode,omitempty"`
}
