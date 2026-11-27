// Conversion between loop messages/tools and the Devin Cascade wire
// shapes. Ports oh-my-pi's buildChatMessagePrompts/buildUserPrompt: history
// is re-sent whole each turn as ChatMessagePrompts under a stable cascade
// id, with deterministic message ids so the server's cache stays warm.
package devin

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"github.com/MoeForge/moego/json"
	"strings"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/loop"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/tools"
	exav1 "github.com/Adelodunpeter25/console/apps/server-go/internal/gen/exa/v1"
)

// deterministicUUID formats the leading 128 bits of seed's SHA-256 as a
// UUID-shaped string (8-4-4-4-12 hex). Deterministic across turns so message
// ids stay stable when history is rebuilt (oh-my-pi deterministicUuid).
func deterministicUUID(seed string) string {
	sum := sha256.Sum256([]byte(seed))
	h := hex.EncodeToString(sum[:16])
	return h[0:8] + "-" + h[8:12] + "-" + h[12:16] + "-" + h[16:20] + "-" + h[20:32]
}

// buildPrompts maps loop history onto Cascade ChatMessagePrompts.
// cascadeID scopes the deterministic message ids.
func buildPrompts(messages []any, cascadeID string) []*exav1.ChatMessagePrompt {
	prompts := make([]*exav1.ChatMessagePrompt, 0, len(messages))
	for i, m := range messages {
		switch msg := m.(type) {
		case loop.UserMessage:
			prompts = append(prompts, userPrompt(msg, deterministicUUID(fmt.Sprintf("%s\x00%d\x00user", cascadeID, i))))
		case *loop.UserMessage:
			if msg != nil {
				prompts = append(prompts, userPrompt(*msg, deterministicUUID(fmt.Sprintf("%s\x00%d\x00user", cascadeID, i))))
			}
		case loop.AssistantMessage:
			if p := assistantPrompt(msg, cascadeID, i); p != nil {
				prompts = append(prompts, p)
			}
		case *loop.AssistantMessage:
			if msg != nil {
				if p := assistantPrompt(*msg, cascadeID, i); p != nil {
					prompts = append(prompts, p)
				}
			}
		case loop.ToolResultMessage:
			prompts = append(prompts, toolResultPrompts(msg, cascadeID, i)...)
		case *loop.ToolResultMessage:
			if msg != nil {
				prompts = append(prompts, toolResultPrompts(*msg, cascadeID, i)...)
			}
		}
	}
	return prompts
}

// BuildPromptsWithCache maps history plus the per-request tail onto prompts and
// marks the last stable message as a cache breakpoint, so the conversation
// prefix is cached like the system prompt is. The tail sits after the marker:
// it changes every request and must stay out of the cached prefix.
func BuildPromptsWithCache(messages []any, cascadeID, tail string, retention loop.CacheRetention) []*exav1.ChatMessagePrompt {
	prompts := buildPrompts(loop.WithTail(tail, messages), cascadeID)
	if retention == loop.CacheNone {
		return prompts
	}
	idx := len(prompts) - 1
	if strings.TrimSpace(tail) != "" {
		idx--
	}
	if idx >= 0 {
		prompts[idx].PromptCacheOptions = &exav1.PromptCacheOptions{Type: exav1.CacheControlType_CACHE_CONTROL_TYPE_EPHEMERAL}
	}
	return prompts
}

// userPrompt flattens one user turn into a USER prompt with inline images.
func userPrompt(msg loop.UserMessage, messageID string) *exav1.ChatMessagePrompt {
	p := &exav1.ChatMessagePrompt{
		MessageId: messageID,
		Source:    exav1.ChatMessageSource_CHAT_MESSAGE_SOURCE_USER,
		Prompt:    msg.Content,
	}
	for _, a := range msg.Attachments {
		if a.Data == "" {
			continue
		}
		p.Images = append(p.Images, &exav1.ImageData{Base64Data: a.Data, MimeType: a.MimeType})
	}
	return p
}

// assistantPrompt maps one assistant message onto a SYSTEM-source prompt:
// text parts join `prompt`, thinking parts join `thinking`/`signature`, and
// tool calls become ChatToolCalls.
func assistantPrompt(msg loop.AssistantMessage, cascadeID string, index int) *exav1.ChatMessagePrompt {
	var promptText, thinkingText, signature string
	var calls []*exav1.ChatToolCall
	for _, part := range msg.Content {
		switch part := part.(type) {
		case loop.TextPart:
			promptText += part.Text
		case map[string]any:
			// Persisted-history fallback shape (same as codex's converter).
			if part["type"] == "text" {
				if t, _ := part["text"].(string); t != "" {
					promptText += t
				}
			}
		case loop.ThinkingPart:
			thinkingText += part.Text
			if part.Signature != "" {
				signature = part.Signature
			}
		case loop.ToolCallPart:
			args := string(part.Call.Arguments)
			if args == "" {
				args = "{}"
			}
			calls = append(calls, &exav1.ChatToolCall{
				Id:            part.Call.ID,
				Name:          part.Call.Name,
				ArgumentsJson: args,
			})
		}
	}
	if promptText == "" && thinkingText == "" && signature == "" && len(calls) == 0 {
		return nil
	}
	return &exav1.ChatMessagePrompt{
		MessageId: "bot-" + deterministicUUID(fmt.Sprintf("%s\x00%d\x00assistant", cascadeID, index)),
		Source:    exav1.ChatMessageSource_CHAT_MESSAGE_SOURCE_SYSTEM,
		Prompt:    promptText,
		Thinking:  thinkingText,
		Signature: signature,
		ToolCalls: calls,
	}
}

// toolResultPrompts emits one TOOL prompt per result — Cascade's prompt
// carries a single toolCallId, so a multi-result message splits.
func toolResultPrompts(msg loop.ToolResultMessage, cascadeID string, index int) []*exav1.ChatMessagePrompt {
	out := make([]*exav1.ChatMessagePrompt, 0, len(msg.Results))
	for _, r := range msg.Results {
		text, images := toolResultContent(r.Content)
		out = append(out, &exav1.ChatMessagePrompt{
			MessageId:         deterministicUUID(fmt.Sprintf("%s\x00%d\x00tool\x00%s", cascadeID, index, r.ToolCallID)),
			Source:            exav1.ChatMessageSource_CHAT_MESSAGE_SOURCE_TOOL,
			ToolCallId:        r.ToolCallID,
			ToolResultIsError: r.IsError,
			Prompt:            text,
			Images:            images,
		})
	}
	return out
}

// toolResultContent renders a tool result as prompt text plus inline
// images: strings pass through, structured content is JSON-marshaled, and
// image items in a content list become ImageData.
func toolResultContent(content any) (string, []*exav1.ImageData) {
	var text string
	var images []*exav1.ImageData
	switch v := content.(type) {
	case string:
		text = v
	case []any:
		var b strings.Builder
		for _, item := range v {
			m, ok := item.(map[string]any)
			if !ok {
				continue
			}
			switch m["type"] {
			case "image":
				data, _ := m["data"].(string)
				mime, _ := m["mimeType"].(string)
				if data != "" {
					images = append(images, &exav1.ImageData{Base64Data: data, MimeType: mime})
				}
			case "text":
				if t, _ := m["text"].(string); t != "" {
					if b.Len() > 0 {
						b.WriteByte('\n')
					}
					b.WriteString(t)
				}
			}
		}
		text = b.String()
		if text == "" {
			raw, _ := json.Marshal(v)
			text = string(raw)
		}
	default:
		if content == nil {
			text = ""
		} else {
			raw, _ := json.Marshal(content)
			text = string(raw)
		}
	}
	if strings.TrimSpace(text) == "" && len(images) == 0 {
		text = "Tool failed with no output."
	}
	return text, images
}

// buildToolDefinitions maps loop tool definitions onto ChatToolDefinition.
// googleSchema selects the Gemini-compatible normalization (type arrays
// rejected by that backend).
func buildToolDefinitions(defs []tools.Definition, googleSchema bool) []*exav1.ChatToolDefinition {
	out := make([]*exav1.ChatToolDefinition, 0, len(defs))
	for _, d := range defs {
		schema := d.InputSchema
		if schema == nil {
			schema = map[string]any{"type": "object"}
		}
		if googleSchema {
			schema = normalizeSchemaForGoogle(schema)
		}
		raw, err := json.Marshal(schema)
		if err != nil {
			continue
		}
		out = append(out, &exav1.ChatToolDefinition{
			Name:             d.Name,
			Description:      d.Description,
			JsonSchemaString: string(raw),
			Strict:           false,
		})
	}
	return out
}

// normalizeSchemaForGoogle rewrites the JSON Schema features Devin's Gemini
// backend rejects: `"type": ["X","null"]` unions become `"type": "X"` plus
// `"nullable": true`, recursively. This is the narrow slice of oh-my-pi's
// normalizeSchemaForGoogle that Console's invopop-derived schemas can
// actually produce.
func normalizeSchemaForGoogle(schema map[string]any) map[string]any {
	return normalizeSchemaValue(schema).(map[string]any)
}

func normalizeSchemaValue(v any) any {
	switch t := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(t))
		for k, val := range t {
			out[k] = normalizeSchemaValue(val)
		}
		if typ, ok := out["type"].([]any); ok {
			var nonNull string
			hasNull := false
			valid := len(typ) > 0
			for _, item := range typ {
				s, ok := item.(string)
				if !ok {
					valid = false
					break
				}
				if s == "null" {
					hasNull = true
				} else if nonNull == "" {
					nonNull = s
				} else {
					// Multi-type unions beyond null are left alone.
					valid = false
					break
				}
			}
			if valid && hasNull && nonNull != "" {
				out["type"] = nonNull
				out["nullable"] = true
			}
		}
		return out
	case []any:
		out := make([]any, len(t))
		for i, item := range t {
			out[i] = normalizeSchemaValue(item)
		}
		return out
	default:
		return v
	}
}

// isGeminiModelUID reports whether a chat model uid routes to a Gemini
// backend: the server's enum-style prefix or a plain "gemini" in the uid.
// Mirrors oh-my-pi's classifyModel("devin", ...) == "gemini" check closely
// enough for the tool-schema normalization decision.
func isGeminiModelUID(uid string) bool {
	lower := strings.ToLower(uid)
	return strings.HasPrefix(lower, "model_google_gemini_") || strings.Contains(lower, "gemini")
}
