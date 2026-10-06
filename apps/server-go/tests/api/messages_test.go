// Messages migration coverage: loop structs round-trip through the
// canonical proto bytes, which must match the shared golden fixtures.
package tests

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"google.golang.org/protobuf/encoding/protojson"

	consolev1 "github.com/Adelodunpeter25/console/apps/server-go/internal/gen/console/v1"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/loop"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/tools"
)

func messageFixture(t *testing.T, name string) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "proto", "testdata", "message", name))
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(string(raw))
}

func TestMessageProtoMatchesFixtures(t *testing.T) {
	user := loop.UserMessage{
		Role: loop.RoleUser, Content: "Hello",
		ContextFiles: []string{"/a.ts"},
		Attachments:  []loop.ImageAttachment{{Data: "QUJD", MimeType: "image/png"}},
	}
	assistant := loop.AssistantMessage{
		Role: loop.RoleAssistant, ID: "m1", StopReason: loop.StopStop,
		Content: []any{
			loop.TextPart{Type: "text", Text: "Hi", ThoughtSignature: "sig"},
			loop.ThinkingPart{Type: "thinking", Text: "hmm"},
			loop.ToolCallPart{Type: "toolCall", Call: tools.ToolCall{
				ID: "c1", Name: "read", Arguments: json.RawMessage(`{"path":"/a"}`),
			}},
		},
	}
	results := loop.ToolResultMessage{
		Role: loop.RoleToolResult,
		Results: []tools.ToolResult{{
			ToolCallID: "c1", ToolName: "read", Content: map[string]any{"ok": true},
		}},
	}

	cases := map[string]any{
		"user.json":       user,
		"assistant.json":  assistant,
		"toolresult.json": results,
	}
	for name, msg := range cases {
		raw, err := loop.ToProtoBytes(msg)
		if err != nil {
			t.Fatal(err)
		}
		if compactJSON(t, raw) != messageFixture(t, name) {
			t.Fatalf("%s drifted:\n got %s\nwant %s", name, compactJSON(t, raw), messageFixture(t, name))
		}
		// Round trip back to loop structs for replay.
		back, err := loop.MessageFromProtoBytes([]byte(messageFixture(t, name)))
		if err != nil {
			t.Fatal(err)
		}
		switch v := back.(type) {
		case loop.UserMessage:
			if v.Content != "Hello" || len(v.Attachments) != 1 {
				t.Fatalf("user round trip: %+v", v)
			}
		case loop.AssistantMessage:
			if v.ID != "m1" || len(v.Content) != 3 {
				t.Fatalf("assistant round trip: %+v", v)
			}
		case loop.ToolResultMessage:
			if len(v.Results) != 1 || v.Results[0].ToolCallID != "c1" {
				t.Fatalf("results round trip: %+v", v)
			}
		default:
			t.Fatalf("unexpected type %T", back)
		}
	}

	// Fixtures stay parseable with unknown-field tolerance.
	var decoded consolev1.AgentMessage
	unmarshal := protojson.UnmarshalOptions{DiscardUnknown: true}
	if err := unmarshal.Unmarshal([]byte(messageFixture(t, "user.json")), &decoded); err != nil {
		t.Fatal(err)
	}
	wrapped, ok := decoded.GetMessage().(*consolev1.AgentMessage_User)
	if !ok || wrapped.User.GetContent() != "Hello" {
		t.Fatalf("decoded: %+v", &decoded)
	}
}
