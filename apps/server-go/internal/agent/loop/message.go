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
	// ThoughtSignature is Gemini's opaque reasoning-continuity token,
	// echoed back verbatim on replay. Unused by Claude/Codex.
	ThoughtSignature string `json:"thoughtSignature,omitempty"`
}

type ThinkingPart struct {
	Type string `json:"type"` // "thinking"
	Text string `json:"text"`
}

// CacheRetention mirrors @console/types CacheRetention: prompt-cache tier.
type CacheRetention string

const (
	CacheShort CacheRetention = "short"
	CacheLong  CacheRetention = "long"
	CacheNone  CacheRetention = "none"
)

// CacheStatus mirrors @console/types CacheStatus.
type CacheStatus string

const (
	CacheHit         CacheStatus = "hit"
	CacheMiss        CacheStatus = "miss"
	CacheWrite       CacheStatus = "write"
	CacheUnknown     CacheStatus = "unknown"
	CacheUnsupported CacheStatus = "unsupported"
)

// TurnUsage is the normalized per-turn token usage with cache breakdown.
// Port of packages/types/src/cache.ts TurnUsage.
type TurnUsage struct {
	Input           int         `json:"input"`
	CacheRead       int         `json:"cacheRead"`
	CacheWrite      int         `json:"cacheWrite"`
	Output          int         `json:"output"`
	ReasoningTokens *int        `json:"reasoningTokens,omitempty"`
	TotalTokens     int         `json:"totalTokens"`
	CacheStatus     CacheStatus `json:"cacheStatus"`
}

type ToolCallPart struct {
	Type string         `json:"type"` // "toolCall"
	Call tools.ToolCall `json:"call"`
}

type UserMessage struct {
	Role        MessageRole       `json:"role"`
	Content     string            `json:"content"`
	Attachments []ImageAttachment `json:"attachments,omitempty"`
}

// ImageAttachment is base64 image data sent inline with a user message.
type ImageAttachment struct {
	Data     string `json:"data"`
	MimeType string `json:"mimeType"`
}

type AssistantMessage struct {
	Role       MessageRole `json:"role"`
	ID         string      `json:"id"`
	Content    []any       `json:"content"` // TextPart | ThinkingPart | ToolCallPart
	StopReason StopReason  `json:"stopReason"`
	Usage      *TurnUsage  `json:"usage,omitempty"`
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
