// Stored-history decoding: session DB rows back into loop messages.
// Stored bytes are the canonical proto shape (see agent/loop ToProtoBytes);
// each row decodes through the loop boundary converters, and unparseable
// rows are skipped, matching the old role-probe leniency.
package run

import (
	"encoding/json"
	"strings"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/loop"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/tools"
)

func DecodeHistory(stored []json.RawMessage) []any {
	out := make([]any, 0, len(stored))
	for _, raw := range stored {
		if len(raw) == 0 {
			continue
		}
		if msg, err := loop.MessageFromProtoBytes(raw); err == nil {
			out = append(out, msg)
			continue
		}
		// Transitional: pre-migration rows still carry the role-keyed
		// shape. New writes are always canonical; drop this fallback
		// once dev stores turn over.
		if msg, ok := legacyDecodeMessage(raw); ok {
			out = append(out, msg)
		}
	}
	return out
}

// legacyDecodeMessage parses a pre-migration role-keyed row into loop
// structs. See DecodeHistory.
func legacyDecodeMessage(raw []byte) (any, bool) {
	var probe struct {
		Role string `json:"role"`
	}
	if err := json.Unmarshal(raw, &probe); err != nil {
		return nil, false
	}
	switch probe.Role {
	case string(loop.RoleUser):
		var user loop.UserMessage
		if err := json.Unmarshal(raw, &user); err == nil {
			return user, true
		}
	case string(loop.RoleAssistant):
		if assistant, ok := legacyDecodeAssistant(raw); ok {
			return assistant, true
		}
	case string(loop.RoleToolResult):
		var result loop.ToolResultMessage
		if err := json.Unmarshal(raw, &result); err == nil {
			return result, true
		}
	}
	return nil, false
}

func legacyDecodeAssistant(raw json.RawMessage) (loop.AssistantMessage, bool) {
	var envelope struct {
		Role       loop.MessageRole  `json:"role"`
		ID         string            `json:"id"`
		Content    []json.RawMessage `json:"content"`
		StopReason loop.StopReason   `json:"stopReason"`
		Usage      *loop.TurnUsage   `json:"usage"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return loop.AssistantMessage{}, false
	}
	assistant := loop.AssistantMessage{
		Role: envelope.Role, ID: envelope.ID, StopReason: envelope.StopReason, Usage: envelope.Usage,
	}
	for _, part := range envelope.Content {
		var kind struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal(part, &kind); err != nil {
			continue
		}
		switch kind.Type {
		case "text":
			var text loop.TextPart
			if err := json.Unmarshal(part, &text); err == nil {
				assistant.Content = append(assistant.Content, text)
			}
		case "thinking":
			var thinking loop.ThinkingPart
			if err := json.Unmarshal(part, &thinking); err == nil {
				assistant.Content = append(assistant.Content, thinking)
			}
		case "toolCall":
			var call struct {
				Call tools.ToolCall `json:"call"`
			}
			if err := json.Unmarshal(part, &call); err == nil {
				assistant.Content = append(assistant.Content, loop.ToolCallPart{Type: "toolCall", Call: call.Call})
			}
		}
	}
	return assistant, true
}

// decodeAssistantText extracts concatenated non-blank text parts from a
// stored assistant message (for done-banner previews).
func decodeAssistantText(raw json.RawMessage) (string, bool) {
	msg, err := loop.MessageFromProtoBytes(raw)
	if err != nil {
		// Transitional pre-migration fallback (see DecodeHistory).
		legacy, ok := legacyDecodeMessage(raw)
		if !ok {
			return "", false
		}
		assistant, ok := legacy.(loop.AssistantMessage)
		if !ok {
			return "", false
		}
		if text := assistantText(assistant); text != "" {
			return text, true
		}
		return "", false
	}
	assistant, ok := msg.(loop.AssistantMessage)
	if !ok {
		return "", false
	}
	text := assistantText(assistant)
	if text == "" {
		return "", false
	}
	return text, true
}

func assistantText(assistant loop.AssistantMessage) string {
	var parts []string
	for _, part := range assistant.Content {
		if text, ok := part.(loop.TextPart); ok && strings.TrimSpace(text.Text) != "" {
			parts = append(parts, text.Text)
		}
	}
	if len(parts) == 0 {
		return ""
	}
	return strings.Join(parts, " ")
}

func decodeAssistant(raw json.RawMessage) (loop.AssistantMessage, bool) {
	msg, err := loop.MessageFromProtoBytes(raw)
	if err != nil {
		return loop.AssistantMessage{}, false
	}
	assistant, ok := msg.(loop.AssistantMessage)
	return assistant, ok
}
