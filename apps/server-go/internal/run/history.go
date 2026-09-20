// Stored-history decoding: session DB rows back into loop messages.
// The loop persists messageJSON(message), so Data carries the same role
// discriminator the TS AgentMessage union uses.
package run

import (
	"encoding/json"
	"strings"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/loop"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/tools"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/types"
)

func decodeHistory(stored []types.AgentMessage) []any {
	out := make([]any, 0, len(stored))
	for _, m := range stored {
		if len(m.Data) == 0 {
			continue
		}
		var probe struct {
			Role string `json:"role"`
		}
		if err := json.Unmarshal(m.Data, &probe); err != nil {
			continue
		}
		switch probe.Role {
		case string(loop.RoleUser):
			var user loop.UserMessage
			if err := json.Unmarshal(m.Data, &user); err == nil {
				out = append(out, user)
			}
		case string(loop.RoleAssistant):
			if assistant, ok := decodeAssistant(m.Data); ok {
				out = append(out, assistant)
			}
		case string(loop.RoleToolResult):
			var result loop.ToolResultMessage
			if err := json.Unmarshal(m.Data, &result); err == nil {
				out = append(out, result)
			}
		}
	}
	return out
}

// decodeAssistantText extracts concatenated non-blank text parts from a
// stored assistant message (for done-banner previews).
func decodeAssistantText(raw json.RawMessage) (string, bool) {
	assistant, ok := decodeAssistant(raw)
	if !ok || assistant.Role != loop.RoleAssistant {
		return "", false
	}
	var parts []string
	for _, part := range assistant.Content {
		if text, ok := part.(loop.TextPart); ok && strings.TrimSpace(text.Text) != "" {
			parts = append(parts, text.Text)
		}
	}
	if len(parts) == 0 {
		return "", false
	}
	return strings.Join(parts, " "), true
}

func decodeAssistant(raw json.RawMessage) (loop.AssistantMessage, bool) {
	var envelope struct {
		Role       loop.MessageRole `json:"role"`
		ID         string           `json:"id"`
		Content    []json.RawMessage `json:"content"`
		StopReason loop.StopReason  `json:"stopReason"`
		Usage      *loop.TurnUsage  `json:"usage"`
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
