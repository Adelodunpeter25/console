// Stored-history decoding: session DB rows back into loop messages.
// Stored bytes are the canonical proto shape (see agent/loop ToProtoBytes);
// each row decodes through the loop boundary converters, and unparseable
// rows are skipped, matching the old role-probe leniency.
package run

import (
	"github.com/MoeForge/moego/json"
	"log/slog"
	"strings"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/loop"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/tools"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/types"
)

const interruptedToolText = "Tool execution was interrupted (server stopped or restarted)."

func DecodeHistory(stored []json.RawMessage) []any {
	out := make([]any, 0, len(stored))
	for _, raw := range stored {
		if len(raw) == 0 {
			continue
		}
		if msg, err := loop.MessageFromProtoBytes(raw); err == nil {
			out = append(out, msg)
		}
	}
	return out
}

// LoadHistory reads session history directly through the split storage path,
// bypassing monolithic JSON reassembly and redundant base64 conversions.
func (s *Service) LoadHistory(sessionID string) ([]any, error) {
	splits, err := s.sessions.LoadSplit(sessionID)
	if err != nil {
		return nil, err
	}
	out := make([]any, 0, len(splits))
	for _, row := range splits {
		if row.Content != "" {
			if msg, err := loop.MessageFromProtoBytes([]byte(row.Content)); err == nil {
				out = append(out, msg)
			}
			continue
		}
		if row.Skeleton == "" {
			continue
		}
		if msg, err := loop.MessageFromSplit([]byte(row.Skeleton), row.Parts); err == nil {
			out = append(out, msg)
		}
	}
	return s.healHistory(sessionID, out)
}

func (s *Service) healHistory(sessionID string, history []any) ([]any, error) {
	healed, changed := HealHistoryMessages(history)
	if !changed || s.sessions == nil || s.IsActive(sessionID) {
		return healed, nil
	}
	msgs := make([]types.AgentMessage, 0, len(healed))
	for _, m := range healed {
		protoBytes, err := loop.ToProtoBytes(m)
		if err != nil {
			continue
		}
		role := ""
		id := ""
		switch msg := m.(type) {
		case loop.UserMessage:
			role = string(loop.RoleUser)
		case *loop.UserMessage:
			role = string(loop.RoleUser)
		case loop.AssistantMessage:
			role = string(loop.RoleAssistant)
			id = msg.ID
		case *loop.AssistantMessage:
			role = string(loop.RoleAssistant)
			id = msg.ID
		case loop.ToolResultMessage:
			role = string(loop.RoleToolResult)
		case *loop.ToolResultMessage:
			role = string(loop.RoleToolResult)
		}
		if id == "" {
			id = "msg_" + randomHex(16)
		}
		msgs = append(msgs, types.AgentMessage{
			ID:   id,
			Role: role,
			Data: protoBytes,
		})
	}
	if err := s.sessions.ReplaceMessages(sessionID, msgs); err != nil {
		slog.Warn("heal history: replace messages failed", "session", sessionID, "error", err)
	}
	return healed, nil
}

// HealHistoryMessages inspects decoded history and ensures every assistant message
// that contains tool calls is immediately followed by a tool-result message covering
// all of its calls. Any missing results are filled with interrupted error results.
func HealHistoryMessages(history []any) ([]any, bool) {
	out := make([]any, 0, len(history))
	changed := false
	for i := 0; i < len(history); i++ {
		msg := history[i]
		out = append(out, msg)

		asst, ok := asAssistant(msg)
		if !ok {
			continue
		}
		calls := assistantToolCalls(asst)
		if len(calls) == 0 {
			continue
		}

		if i+1 < len(history) {
			if toolRes, ok := asToolResult(history[i+1]); ok {
				answered := map[string]bool{}
				for _, r := range toolRes.Results {
					answered[r.ToolCallID] = true
				}
				var missing []tools.ToolResult
				for _, c := range calls {
					if !answered[c.ID] {
						missing = append(missing, tools.ToolResult{
							ToolCallID: c.ID,
							ToolName:   c.Name,
							Content:    interruptedToolText,
							IsError:    true,
						})
					}
				}
				if len(missing) > 0 {
					changed = true
					toolRes.Results = append(toolRes.Results, missing...)
					history[i+1] = toolRes
				}
				continue
			}
		}

		changed = true
		resMsg := loop.ToolResultMessage{
			Role:    loop.RoleToolResult,
			Results: make([]tools.ToolResult, 0, len(calls)),
		}
		for _, c := range calls {
			resMsg.Results = append(resMsg.Results, tools.ToolResult{
				ToolCallID: c.ID,
				ToolName:   c.Name,
				Content:    interruptedToolText,
				IsError:    true,
			})
		}
		out = append(out, resMsg)
	}
	return out, changed
}

func asAssistant(msg any) (loop.AssistantMessage, bool) {
	switch m := msg.(type) {
	case loop.AssistantMessage:
		return m, true
	case *loop.AssistantMessage:
		if m != nil {
			return *m, true
		}
	}
	return loop.AssistantMessage{}, false
}

func asToolResult(msg any) (loop.ToolResultMessage, bool) {
	switch m := msg.(type) {
	case loop.ToolResultMessage:
		return m, true
	case *loop.ToolResultMessage:
		if m != nil {
			return *m, true
		}
	}
	return loop.ToolResultMessage{}, false
}

func assistantToolCalls(asst loop.AssistantMessage) []tools.ToolCall {
	var calls []tools.ToolCall
	for _, p := range asst.Content {
		if tc, ok := p.(loop.ToolCallPart); ok {
			calls = append(calls, tc.Call)
		}
	}
	return calls
}

// decodeAssistantText extracts concatenated non-blank text parts from a
// stored assistant message (for done-banner previews).
func decodeAssistantText(raw json.RawMessage) (string, bool) {
	msg, err := loop.MessageFromProtoBytes(raw)
	if err != nil {
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
