// Claude wire conversion: loop messages/tools to the Anthropic Messages
// format. Port of apps/server/providers/src/claude/convert.ts.
package claude

import (
	"encoding/json"
	"strings"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/loop"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/tools"
)

// ClaudeMessage is one Anthropic conversation turn.
type ClaudeMessage struct {
	Role    string           `json:"role"`
	Content []map[string]any `json:"content"`
}

// ClaudeTool is one Anthropic tool definition.
type ClaudeTool struct {
	Name         string         `json:"name"`
	Description  string         `json:"description,omitempty"`
	InputSchema  map[string]any `json:"input_schema"`
	CacheControl map[string]any `json:"cache_control,omitempty"`
}

// EmptyToolResultText is the placeholder for empty tool results — the
// Messages API rejects empty content.
const EmptyToolResultText = "Tool failed with no output."

func toolResultText(content any) string {
	var text string
	if s, ok := content.(string); ok {
		text = s
	} else {
		raw, _ := json.Marshal(content)
		text = string(raw)
	}
	if strings.TrimSpace(text) == "" {
		return EmptyToolResultText
	}
	return text
}

func parseToolInput(input any) map[string]any {
	if m, ok := input.(map[string]any); ok {
		return m
	}
	if s, ok := input.(string); ok && strings.TrimSpace(s) != "" {
		var m map[string]any
		if err := json.Unmarshal([]byte(s), &m); err == nil {
			return m
		}
	}
	if raw, ok := input.(json.RawMessage); ok && len(raw) > 0 {
		var m map[string]any
		if err := json.Unmarshal(raw, &m); err == nil {
			return m
		}
	}
	return map[string]any{}
}

func normalizeImageMime(mimeType string) string {
	switch strings.TrimSpace(strings.ToLower(mimeType)) {
	case "image/jpg":
		return "image/jpeg"
	case "image/jpeg", "image/png", "image/gif", "image/webp":
		return strings.TrimSpace(strings.ToLower(mimeType))
	default:
		return ""
	}
}

// partText extracts display text from an assistant content part in any of
// the Go shapes (typed structs or decoded maps).
func partText(part any) (text string, isThinking bool) {
	switch p := part.(type) {
	case loop.TextPart:
		return p.Text, false
	case loop.ThinkingPart:
		return p.Text, true
	case map[string]any:
		if t, _ := p["text"].(string); t != "" {
			if p["type"] == "thinking" {
				return t, true
			}
			return t, false
		}
	}
	return "", false
}

// partToolCall extracts a tool call from an assistant content part.
func partToolCall(part any) (id, name string, args any, ok bool) {
	switch p := part.(type) {
	case loop.ToolCallPart:
		return p.Call.ID, p.Call.Name, p.Call.Arguments, true
	case map[string]any:
		if p["type"] != "toolCall" {
			return "", "", nil, false
		}
		call, _ := p["call"].(map[string]any)
		if call == nil {
			return "", "", nil, false
		}
		id, _ := call["id"].(string)
		name, _ := call["name"].(string)
		return id, name, call["arguments"], id != "" && name != ""
	}
	return "", "", nil, false
}

// ConvertMessages maps loop history to Anthropic turns: strict role
// alternation, orphan-result sanitizing, and a trailing cache breakpoint.
func ConvertMessages(messages []any, retention loop.CacheRetention) []ClaudeMessage {
	turns := make([]ClaudeMessage, 0, len(messages))
	for _, m := range messages {
		switch msg := m.(type) {
		case loop.UserMessage:
			text := msg.Content
			attachments := msg.Attachments
			if strings.TrimSpace(text) == "" && len(attachments) == 0 {
				continue
			}
			content := make([]map[string]any, 0, len(attachments)+1)
			if strings.TrimSpace(text) != "" {
				content = append(content, map[string]any{"type": "text", "text": text})
			} else {
				content = append(content, map[string]any{"type": "text", "text": "(see attached images)"})
			}
			for _, att := range attachments {
				mediaType := normalizeImageMime(att.MimeType)
				if mediaType == "" {
					content = append(content, map[string]any{
						"type": "text", "text": "[unsupported image: " + att.MimeType + "]",
					})
					continue
				}
				content = append(content, map[string]any{
					"type": "image",
					"source": map[string]any{
						"type": "base64", "media_type": mediaType, "data": att.Data,
					},
				})
			}
			turns = append(turns, ClaudeMessage{Role: "user", Content: content})
		case *loop.UserMessage:
			if msg != nil {
				turns = append(turns, ConvertMessages([]any{*msg}, retention)...)
			}
		case loop.AssistantMessage:
			var texts, toolUses []map[string]any
			for _, part := range msg.Content {
				if text, thinking := partText(part); text != "" {
					// Prior thinking replays unsigned, which the signing
					// endpoint rejects — demote to text instead of dropping.
					_ = thinking
					texts = append(texts, map[string]any{"type": "text", "text": text})
				} else if id, name, args, ok := partToolCall(part); ok {
					toolUses = append(toolUses, map[string]any{
						"type": "tool_use", "id": id, "name": name, "input": parseToolInput(args),
					})
				}
			}
			// All text must precede tool_use blocks in an Anthropic turn.
			content := append(texts, toolUses...)
			if len(content) > 0 {
				turns = append(turns, ClaudeMessage{Role: "assistant", Content: content})
			}
		case loop.ToolResultMessage:
			content := make([]map[string]any, 0, len(msg.Results))
			for _, r := range msg.Results {
				block := map[string]any{
					"type": "tool_result", "tool_use_id": r.ToolCallID,
					"content": toolResultText(r.Content),
				}
				if r.IsError {
					block["is_error"] = true
				}
				content = append(content, block)
			}
			if len(content) > 0 {
				turns = append(turns, ClaudeMessage{Role: "user", Content: content})
			}
		}
	}

	// Merge adjacent same-role turns (Anthropic requires strict alternation).
	merged := make([]ClaudeMessage, 0, len(turns))
	for _, turn := range turns {
		if n := len(merged); n > 0 && merged[n-1].Role == turn.Role {
			merged[n-1].Content = append(merged[n-1].Content, turn.Content...)
		} else {
			merged = append(merged, ClaudeMessage{Role: turn.Role, Content: append([]map[string]any{}, turn.Content...)})
		}
	}

	// Conversations must start and end with a user turn.
	if len(merged) == 0 {
		merged = append(merged, ClaudeMessage{Role: "user", Content: []map[string]any{{"type": "text", "text": "(continue)"}}})
	} else {
		if merged[0].Role != "user" {
			merged = append([]ClaudeMessage{{Role: "user", Content: []map[string]any{{"type": "text", "text": "(session started)"}}}}, merged...)
		}
		if merged[len(merged)-1].Role != "user" {
			merged = append(merged, ClaudeMessage{Role: "user", Content: []map[string]any{{"type": "text", "text": "(continue)"}}})
		}
	}

	// Sanitize tool results: every tool_result needs a matching tool_use in
	// the immediately preceding assistant turn; orphans become text.
	for i := range merged {
		if merged[i].Role != "user" {
			continue
		}
		valid := map[string]bool{}
		if i > 0 && merged[i-1].Role == "assistant" {
			for _, block := range merged[i-1].Content {
				if block["type"] == "tool_use" {
					if id, _ := block["id"].(string); id != "" {
						valid[id] = true
					}
				}
			}
		}
		for j, block := range merged[i].Content {
			if block["type"] != "tool_result" {
				continue
			}
			id, _ := block["tool_use_id"].(string)
			if valid[id] {
				continue
			}
			text := ""
			if s, ok := block["content"].(string); ok {
				text = s
			} else {
				raw, _ := json.Marshal(block["content"])
				text = string(raw)
			}
			merged[i].Content[j] = map[string]any{"type": "text", "text": "[Tool result: " + text + "]"}
		}
	}

	// Trailing cache breakpoint over the conversation prefix. Skipped when
	// the caller opts out.
	if retention != loop.CacheNone && len(merged) > 0 {
		last := &merged[len(merged)-1]
		if n := len(last.Content); n > 0 {
			last.Content[n-1]["cache_control"] = map[string]any{"type": "ephemeral"}
		}
	}
	return merged
}

// NormalizeSchema upgrades draft-07 boolean exclusiveMinimum/Maximum to
// draft-2020-12 numeric form (same upgrade Codex applies).
func NormalizeSchema(value any) any {
	switch v := value.(type) {
	case []any:
		out := make([]any, len(v))
		for i, item := range v {
			out[i] = NormalizeSchema(item)
		}
		return out
	case map[string]any:
		normalized := make(map[string]any, len(v))
		for key, child := range v {
			normalized[key] = NormalizeSchema(child)
		}
		if em, ok := normalized["exclusiveMinimum"]; ok {
			if em == true {
				if minimum, ok := normalized["minimum"].(float64); ok {
					normalized["exclusiveMinimum"] = minimum
				} else {
					delete(normalized, "exclusiveMinimum")
				}
			} else if em == false {
				delete(normalized, "exclusiveMinimum")
			}
		}
		if em, ok := normalized["exclusiveMaximum"]; ok {
			if em == true {
				if maximum, ok := normalized["maximum"].(float64); ok {
					normalized["exclusiveMaximum"] = maximum
				} else {
					delete(normalized, "exclusiveMaximum")
				}
			} else if em == false {
				delete(normalized, "exclusiveMaximum")
			}
		}
		return normalized
	default:
		return value
	}
}

// ConvertTools maps tool definitions to Anthropic tools, with a cache
// breakpoint on the last (static) definition unless opted out.
func ConvertTools(defs []tools.Definition, retention loop.CacheRetention) []ClaudeTool {
	out := make([]ClaudeTool, 0, len(defs))
	for i, t := range defs {
		schema := map[string]any{"type": "object"}
		if t.InputSchema != nil {
			if normalized, ok := NormalizeSchema(t.InputSchema).(map[string]any); ok {
				schema = normalized
			}
		}
		tool := ClaudeTool{Name: t.Name, InputSchema: schema}
		if t.Description != "" {
			tool.Description = t.Description
		}
		if retention != loop.CacheNone && i == len(defs)-1 {
			tool.CacheControl = map[string]any{"type": "ephemeral"}
		}
		out = append(out, tool)
	}
	return out
}
