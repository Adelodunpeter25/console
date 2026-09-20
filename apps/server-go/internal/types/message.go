package types

import "encoding/json"

// AgentMessage mirrors the TS union loosely: identity + role on the struct,
// the full original payload preserved in Data for the provider layer.
type AgentMessage struct {
	ID   string          `json:"id"`
	Role string          `json:"role"`
	Data json.RawMessage `json:"data,omitempty"`
}
