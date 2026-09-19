// Central type definitions shared by every internal package. Message
// payloads are kept as raw JSON so role-discriminated unions survive the
// port unchanged.
package types

import "encoding/json"

type ProjectInfo struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Path      string `json:"path"`
	CreatedAt int64  `json:"createdAt"`
	UpdatedAt int64  `json:"updatedAt"`
}

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

// AgentMessage mirrors the TS union loosely: identity + role on the struct,
// the full original payload preserved in Data for the provider layer.
type AgentMessage struct {
	ID   string          `json:"id"`
	Role string          `json:"role"`
	Data json.RawMessage `json:"data,omitempty"`
}

type ModelFavorite struct {
	Provider string `json:"provider"`
	ModelID  string `json:"modelId"`
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
