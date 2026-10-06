// Stored-history decoding: session DB rows back into loop messages.
// Stored bytes are the canonical proto shape (see agent/loop ToProtoBytes);
// each row decodes through the loop boundary converters, and unparseable
// rows are skipped, matching the old role-probe leniency.
package run

import (
	"encoding/json"
	"strings"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/loop"
)

func decodeHistory(stored []json.RawMessage) []any {
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
	msg, err := loop.MessageFromProtoBytes(raw)
	if err != nil {
		return loop.AssistantMessage{}, false
	}
	assistant, ok := msg.(loop.AssistantMessage)
	return assistant, ok
}
