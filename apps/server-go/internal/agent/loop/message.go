// Conversation message types. Field names match the TS agent messages
// (apps/server/agent/src/types/message.ts) so session storage stays
// compatible.
package loop

import (
	"encoding/json"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/tools"
)

type MessageRole string

const (
	RoleUser       MessageRole = "user"
	RoleAssistant  MessageRole = "assistant"
	RoleToolResult MessageRole = "toolResult"
)

type StopReason string

const (
	StopStop    StopReason = "stop"
	StopToolUse StopReason = "toolUse"
	StopError   StopReason = "error"
	StopMaxTok  StopReason = "maxTokens"
)

// Parts of an assistant message.
type TextPart struct {
	Type string `json:"type"` // "text"
	Text string `json:"text"`
}

type ThinkingPart struct {
	Type string `json:"type"` // "thinking"
	Text string `json:"text"`
}

type ToolCallPart struct {
	Type string         `json:"type"` // "toolCall"
	Call tools.ToolCall `json:"call"`
}

type UserMessage struct {
	Role    MessageRole `json:"role"`
	Content string      `json:"content"`
}

type AssistantMessage struct {
	Role       MessageRole `json:"role"`
	ID         string      `json:"id"`
	Content    []any       `json:"content"` // TextPart | ThinkingPart | ToolCallPart
	StopReason StopReason  `json:"stopReason"`
}

type ToolResultMessage struct {
	Role    MessageRole        `json:"role"`
	Results []tools.ToolResult `json:"results"`
}

// messageJSON wraps any message for storage (role discriminator + payload).
func messageJSON(v any) json.RawMessage {
	raw, err := json.Marshal(v)
	if err != nil {
		return json.RawMessage(`{"role":"user","content":"unserializable message"}`)
	}
	return raw
}
