// Assist + notification + usage wire types ported from packages/types.
package types

type SlashCommandInfo struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Builtin     bool   `json:"builtin"`
}

type NotificationEvent struct {
	Type      string `json:"type"` // "notification"
	Kind      string `json:"kind"`
	SessionID string `json:"sessionId"`
	Title     string `json:"title"`
	Subtitle  string `json:"subtitle,omitempty"`
	Body      string `json:"body"`
}
