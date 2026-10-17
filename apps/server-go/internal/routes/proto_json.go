// Shared protojson helpers for route handlers.
package routes

import (
	"encoding/json"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

var protoMarshal = protojson.MarshalOptions{}
var protoUnmarshal = protojson.UnmarshalOptions{DiscardUnknown: true}

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
