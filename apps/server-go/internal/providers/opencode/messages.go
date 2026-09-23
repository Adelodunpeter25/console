// OpenCode message conversion for Chat Completions and Responses.
package opencode

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/loop"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/tools"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/providers/shared"
)

// ConvertChatMessages maps the loop's typed and JSON-decoded history to
// OpenAI-compatible Chat Completions messages.
func ConvertChatMessages(messages []any) []map[string]any {
	out := make([]map[string]any, 0, len(messages))
	for _, message := range messages {
		if user, ok := asUserMessage(message); ok {
			if strings.TrimSpace(user.Content) == "" && len(user.Attachments) == 0 {
				continue
			}
			out = append(out, map[string]any{
				"role":    "user",
				"content": chatUserContent(user),
			})
			continue
		}
		if assistant, ok := asAssistantMessage(message); ok {
			if wire, include := chatAssistant(assistant); include {
				out = append(out, wire)
			}
			continue
		}
		if results, ok := asToolResultMessage(message); ok {
			for _, result := range results {
				out = append(out, map[string]any{
					"role":         "tool",
					"tool_call_id": result.ToolCallID,
					"content":      toolResultText(result),
				})
			}
		}
	}
	if len(out) == 0 {
		out = append(out, map[string]any{"role": "user", "content": "(continue)"})
	}
	return out
}

// ConvertResponsesMessages maps loop history to Codex-compatible Responses
// input items. Plain third-party reasoning is intentionally omitted.
func ConvertResponsesMessages(messages []any) []map[string]any {
	out := make([]map[string]any, 0, len(messages))
	for _, message := range messages {
		if user, ok := asUserMessage(message); ok {
			if strings.TrimSpace(user.Content) == "" && len(user.Attachments) == 0 {
				continue
			}
			content := []map[string]any{{"type": "input_text", "text": user.Content}}
			for _, attachment := range user.Attachments {
				content = append(content, map[string]any{
					"type":      "input_image",
					"image_url": fmt.Sprintf("data:%s;base64,%s", attachment.MimeType, attachment.Data),
				})
			}
			out = append(out, map[string]any{"role": "user", "content": content})
			continue
		}
		if assistant, ok := asAssistantMessage(message); ok {
			for _, part := range assistant {
				if text, ok := partText(part); ok && text != "" {
					out = append(out, map[string]any{
						"role":    "assistant",
						"content": []map[string]any{{"type": "output_text", "text": text}},
					})
				}
				if call, ok := partToolCall(part); ok {
					out = append(out, map[string]any{
						"type":      "function_call",
						"call_id":   call.ID,
						"name":      call.Name,
						"arguments": callArguments(call),
					})
				}
			}
			continue
		}
		if results, ok := asToolResultMessage(message); ok {
			for _, result := range results {
				out = append(out, map[string]any{
					"type":    "function_call_output",
					"call_id": result.ToolCallID,
					"output":  toolResultText(result),
				})
			}
		}
	}
	if len(out) == 0 {
		out = append(out, map[string]any{
			"role":    "user",
			"content": []map[string]any{{"type": "input_text", "text": "(continue)"}},
		})
	}
	return out
}

func asUserMessage(message any) (loop.UserMessage, bool) {
	switch value := message.(type) {
	case loop.UserMessage:
		return value, true
	case *loop.UserMessage:
		if value != nil {
			return *value, true
		}
	case map[string]any:
		if role, _ := value["role"].(string); role != string(loop.RoleUser) {
			return loop.UserMessage{}, false
		}
		content, _ := value["content"].(string)
		user := loop.UserMessage{Role: loop.RoleUser, Content: content}
		if raw, ok := value["attachments"].([]any); ok {
			for _, item := range raw {
				attachment, ok := item.(map[string]any)
				if !ok {
					continue
				}
				user.Attachments = append(user.Attachments, loop.ImageAttachment{
					Data:     stringValue(attachment["data"]),
					MimeType: stringValue(attachment["mimeType"]),
				})
			}
		} else if raw, ok := value["attachments"].([]map[string]any); ok {
			for _, attachment := range raw {
				user.Attachments = append(user.Attachments, loop.ImageAttachment{
					Data:     stringValue(attachment["data"]),
					MimeType: stringValue(attachment["mimeType"]),
				})
			}
		}
		return user, true
	}
	return loop.UserMessage{}, false
}

func asAssistantMessage(message any) ([]any, bool) {
	switch value := message.(type) {
	case loop.AssistantMessage:
		return value.Content, true
	case *loop.AssistantMessage:
		if value != nil {
			return value.Content, true
		}
	case map[string]any:
		if role, _ := value["role"].(string); role != string(loop.RoleAssistant) {
			return nil, false
		}
		switch content := value["content"].(type) {
		case []any:
			return content, true
		case []map[string]any:
			parts := make([]any, len(content))
			for i := range content {
				parts[i] = content[i]
			}
			return parts, true
		case string:
			if content == "" {
				return nil, true
			}
			return []any{map[string]any{"type": "text", "text": content}}, true
		default:
			return nil, true
		}
	}
	return nil, false
}

func asToolResultMessage(message any) ([]tools.ToolResult, bool) {
	switch value := message.(type) {
	case loop.ToolResultMessage:
		return value.Results, true
	case *loop.ToolResultMessage:
		if value != nil {
			return value.Results, true
		}
	case map[string]any:
		if role, _ := value["role"].(string); role != string(loop.RoleToolResult) {
			return nil, false
		}
		var rawResults []any
		if values, ok := value["results"].([]any); ok {
			rawResults = values
		} else if values, ok := value["results"].([]map[string]any); ok {
			rawResults = make([]any, len(values))
			for i := range values {
				rawResults[i] = values[i]
			}
		}
		results := make([]tools.ToolResult, 0, len(rawResults))
		for _, raw := range rawResults {
			result, ok := raw.(map[string]any)
			if !ok {
				continue
			}
			toolResult := tools.ToolResult{
				ToolCallID: stringValue(result["toolCallId"]),
				ToolName:   stringValue(result["toolName"]),
				Content:    result["content"],
			}
			if isError, ok := result["isError"].(bool); ok {
				toolResult.IsError = isError
			}
			if rawArgs, ok := result["args"]; ok {
				toolResult.Args = rawJSON(rawArgs)
			}
			results = append(results, toolResult)
		}
		return results, true
	}
	return nil, false
}

func chatUserContent(user loop.UserMessage) any {
	if len(user.Attachments) == 0 {
		return user.Content
	}
	content := []map[string]any{{"type": "text", "text": user.Content}}
	for _, attachment := range user.Attachments {
		content = append(content, map[string]any{
			"type": "image_url",
			"image_url": map[string]any{
				"url": fmt.Sprintf("data:%s;base64,%s", attachment.MimeType, attachment.Data),
			},
		})
	}
	return content
}

func chatAssistant(parts []any) (map[string]any, bool) {
	textParts := make([]string, 0, len(parts))
	toolCalls := make([]map[string]any, 0)
	for _, part := range parts {
		if text, ok := partText(part); ok && text != "" {
			textParts = append(textParts, text)
		}
		if call, ok := partToolCall(part); ok {
			toolCalls = append(toolCalls, map[string]any{
				"id":   call.ID,
				"type": "function",
				"function": map[string]any{
					"name":      call.Name,
					"arguments": callArguments(call),
				},
			})
		}
	}
	if len(textParts) == 0 && len(toolCalls) == 0 {
		return nil, false
	}
	message := map[string]any{"role": "assistant"}
	if len(textParts) > 0 {
		message["content"] = strings.Join(textParts, "")
	} else {
		message["content"] = nil
	}
	if len(toolCalls) > 0 {
		message["tool_calls"] = toolCalls
	}
	return message, true
}

func partText(part any) (string, bool) {
	switch value := part.(type) {
	case loop.TextPart:
		return value.Text, true
	case map[string]any:
		if value["type"] == "text" {
			return stringValue(value["text"]), true
		}
	}
	return "", false
}

func partToolCall(part any) (tools.ToolCall, bool) {
	switch value := part.(type) {
	case loop.ToolCallPart:
		return value.Call, true
	case tools.ToolCall:
		return value, true
	case map[string]any:
		if value["type"] != "toolCall" {
			return tools.ToolCall{}, false
		}
		switch call := value["call"].(type) {
		case tools.ToolCall:
			return call, true
		case map[string]any:
			return tools.ToolCall{
				ID:        stringValue(call["id"]),
				Name:      stringValue(call["name"]),
				Arguments: rawJSON(call["arguments"]),
			}, true
		default:
			return tools.ToolCall{}, true
		}
	}
	return tools.ToolCall{}, false
}

func callArguments(call tools.ToolCall) string {
	if len(call.Arguments) == 0 || string(call.Arguments) == "null" {
		return "{}"
	}
	if json.Valid(call.Arguments) {
		return string(call.Arguments)
	}
	raw, _ := json.Marshal(string(call.Arguments))
	return string(raw)
}

func toolResultText(result tools.ToolResult) string {
	text := flattenToolResult(result.Content)
	if result.IsError {
		return "Error: " + text
	}
	return text
}

func flattenToolResult(content any) string {
	if text, ok := content.(string); ok {
		return text
	}
	if items, ok := content.([]any); ok {
		parts := make([]string, 0, len(items))
		for _, item := range items {
			if object, ok := item.(map[string]any); ok {
				if text, ok := object["text"].(string); ok {
					parts = append(parts, text)
					continue
				}
			}
			parts = append(parts, shared.ToolResultText(item))
		}
		return strings.Join(parts, "\n")
	}
	if items, ok := content.([]map[string]any); ok {
		parts := make([]string, 0, len(items))
		for _, item := range items {
			if text, ok := item["text"].(string); ok {
				parts = append(parts, text)
				continue
			}
			parts = append(parts, shared.ToolResultText(item))
		}
		return strings.Join(parts, "\n")
	}
	return shared.ToolResultText(content)
}

func stringValue(value any) string {
	text, _ := value.(string)
	return text
}

func rawJSON(value any) json.RawMessage {
	switch typed := value.(type) {
	case json.RawMessage:
		return typed
	case string:
		if typed == "" {
			return nil
		}
		return json.RawMessage(typed)
	case nil:
		return nil
	default:
		raw, _ := json.Marshal(typed)
		return raw
	}
}
