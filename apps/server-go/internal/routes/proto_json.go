// Shared protojson helpers for route handlers.
package routes

import (
	"encoding/json"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	consolev1 "github.com/Adelodunpeter25/console/apps/server-go/internal/gen/console/v1"
)

var protoMarshal = protojson.MarshalOptions{}
var protoUnmarshal = protojson.UnmarshalOptions{DiscardUnknown: true}

// messagesToProto decodes stored protojson rows into the canonical messages.
// A row that no longer parses is skipped, matching the clients' lenient
// decoders — a bad row must never fail a whole page.
func messagesToProto(raw []json.RawMessage) []*consolev1.AgentMessage {
	out := make([]*consolev1.AgentMessage, 0, len(raw))
	for _, row := range raw {
		var msg consolev1.AgentMessage
		if err := protoUnmarshal.Unmarshal(row, &msg); err != nil {
			continue
		}
		out = append(out, &msg)
	}
	return out
}

// marshalProtoList encodes a slice of messages as a JSON array of protojson
// objects, preserving the old bare-array list shapes (e.g. favorites,
// projects) while sourcing every element from generated types.
func marshalProtoList[T proto.Message](items []T) (json.RawMessage, error) {
	elements := make([]json.RawMessage, 0, len(items))
	for _, item := range items {
		raw, err := protoMarshal.Marshal(item)
		if err != nil {
			return nil, err
		}
		elements = append(elements, raw)
	}
	return json.Marshal(elements)
}

// extractWakeRecords collects wake-turn markers from synthetic user messages
// before visibleMessages drops them from the client transcript.
func extractWakeRecords(raw []json.RawMessage) []*consolev1.WakeRecord {
	var out []*consolev1.WakeRecord
	for _, row := range raw {
		var msg consolev1.AgentMessage
		if err := protoUnmarshal.Unmarshal(row, &msg); err != nil {
			continue
		}
		user := msg.GetUser()
		if user == nil || !user.GetSynthetic() {
			continue
		}
		label := user.GetWakeLabel()
		if label == "" {
			label = "Background work completed, continuing agent"
		}
		out = append(out, &consolev1.WakeRecord{
			Message: label,
			At:      msg.GetCreatedAt(),
		})
	}
	return out
}

// visibleMessages drops server-injected (synthetic) user turns, such as the
// wake-up message for finished background work. The model keeps them in its
// history; clients never render them.
func visibleMessages(messages []json.RawMessage) []json.RawMessage {
	out := make([]json.RawMessage, 0, len(messages))
	for _, raw := range messages {
		var probe struct {
			User *struct {
				Synthetic bool `json:"synthetic"`
			} `json:"user"`
		}
		if json.Unmarshal(raw, &probe) == nil && probe.User != nil && probe.User.Synthetic {
			continue
		}
		out = append(out, raw)
	}
	return out
}
