// Session types shared across the server.
package types

import "encoding/json"

type SessionHeader struct {
	ID           string  `json:"id"`
	Title        string  `json:"title"`
	Cwd          string  `json:"cwd"`
	ProjectID    *string `json:"projectId,omitempty"`
	ModelID      string  `json:"modelId"`
	Provider     string  `json:"provider"`
	ApprovalMode string  `json:"approvalMode"`
	CreatedAt    int64   `json:"createdAt"`
	UpdatedAt    int64   `json:"updatedAt"`
	MessageCount int     `json:"messageCount"`
	Status       string  `json:"status"`
	DeletedAt    *int64  `json:"deletedAt,omitempty"`
	// Worktree is non-nil when the session lives in its own git worktree.
	Worktree *SessionWorktree `json:"worktree,omitempty"`
}

// SessionWorktree records the worktree a session owns.
type SessionWorktree struct {
	Path   string `json:"path"`
	Branch string `json:"branch"`
	Repo   string `json:"repo"`
}

type CreateSessionOptions struct {
	ID           string  `json:"id,omitempty"`
	Title        string  `json:"title,omitempty"`
	Cwd          string  `json:"cwd,omitempty"`
	ProjectID    *string `json:"projectId,omitempty"`
	ModelID      string  `json:"modelId,omitempty"`
	Provider     string  `json:"provider,omitempty"`
	ApprovalMode string  `json:"approvalMode,omitempty"`
	// ProjectNull tracks an explicit JSON null for projectId (scratchpad),
	// distinct from an omitted key (infer from cwd). Set by the route, which
	// is the only JSON decoder for this struct.
	ProjectNull bool `json:"-"`
	// Worktree, when non-nil, asks for a fresh worktree + branch backing
	// this session (opt-in; default off).
	Worktree *CreateWorktreeSpec `json:"worktree,omitempty"`
	// ResolvedWorktree carries the provisioned worktree into Create. Set by
	// the service facade after WorktreeAdd succeeds — never by clients.
	ResolvedWorktree *SessionWorktree `json:"-"`
}

// CreateWorktreeSpec is the client-facing worktree request. An empty Branch
// auto-derives slug(title)-shortid.
type CreateWorktreeSpec struct {
	Branch string `json:"branch,omitempty"`
}

type LoadedSession struct {
	Header     SessionHeader   `json:"header"`
	Messages   []json.RawMessage `json:"messages"`
	HasMore    bool            `json:"hasMore"`
	NextCursor *int64          `json:"nextCursor"`
}

// SubagentActivityItem mirrors the TS row activity JSON.
type SubagentActivityItem struct {
	TurnIndex  int             `json:"turnIndex"`
	ToolCallID string          `json:"toolCallId"`
	ToolName   string          `json:"toolName"`
	Summary    *string         `json:"summary,omitempty"`
	Args       json.RawMessage `json:"args,omitempty"`
	Status     string          `json:"status"`
	Error      *string         `json:"error,omitempty"`
}

// SubagentInfo mirrors the TS getSessionSubagents shape.
type SubagentInfo struct {
	SubagentID       string                 `json:"subagentId"`
	ParentToolCallID string                 `json:"parentToolCallId"`
	Name             string                 `json:"name"`
	Role             string                 `json:"role"`
	Prompt           string                 `json:"prompt"`
	MaxTurns         int                    `json:"maxTurns"`
	CurrentTurn      int                    `json:"currentTurn"`
	Status           string                 `json:"status"`
	Summary          *string                `json:"summary,omitempty"`
	Error            *string                `json:"error,omitempty"`
	Activities       []SubagentActivityItem `json:"activities"`
	CreatedAt        int64                  `json:"createdAt"`
	UpdatedAt        int64                  `json:"updatedAt"`
}

type SessionFileChange struct {
	Path       string  `json:"path"`
	TurnIndex  int     `json:"turnIndex"`
	Status     string  `json:"status"`
	Additions  int     `json:"additions"`
	Deletions  int     `json:"deletions"`
	DiffText   *string `json:"diffText,omitempty"`
	UpdatedAt  int64   `json:"updatedAt"`
}
