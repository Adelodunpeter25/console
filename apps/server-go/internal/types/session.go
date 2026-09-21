// Session types shared across the server.
package types

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
}

type CreateSessionOptions struct {
	ID           string  `json:"id,omitempty"`
	Title        string  `json:"title,omitempty"`
	Cwd          string  `json:"cwd"`
	ProjectID    *string `json:"projectId,omitempty"`
	ModelID      string  `json:"modelId"`
	Provider     string  `json:"provider"`
	ApprovalMode string  `json:"approvalMode,omitempty"`
}

type LoadedSession struct {
	Header     SessionHeader  `json:"header"`
	Messages   []AgentMessage `json:"messages"`
	HasMore    bool           `json:"hasMore"`
	NextCursor *int64         `json:"nextCursor"`
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
