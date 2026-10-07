// Model stream parts migration coverage: proto-built deltas must match the
// shared golden fixtures at full-frame level ({"type", "part"}).
package tests

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"google.golang.org/protobuf/encoding/protojson"

	consolev1 "github.com/Adelodunpeter25/console/apps/server-go/internal/gen/console/v1"
)

func streamPartFixture(t *testing.T, name string) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "proto", "testdata", "event", name))
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(string(raw))
}

// frameBytes assembles the SSE frame exactly like wireFrame does: hand
// envelope, protojson part.
func frameBytes(t *testing.T, part *consolev1.ModelStreamPart) string {
	t.Helper()
	raw, err := protojson.Marshal(part)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(struct {
		Type string          `json:"type"`
		Part json.RawMessage `json:"part"`
	}{Type: "modelStreamPart", Part: raw})
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(string(encoded))
}

func TestStreamPartProtoMatchesFixtures(t *testing.T) {
	text := &consolev1.ModelStreamPart{
		Part: &consolev1.ModelStreamPart_Text{Text: "hi"},
	}
	if frameBytes(t, text) != streamPartFixture(t, "stream_part_text.json") {
		t.Fatalf("text drifted:\n got %s\nwant %s", frameBytes(t, text), streamPartFixture(t, "stream_part_text.json"))
	}

	thinking := &consolev1.ModelStreamPart{
		Part: &consolev1.ModelStreamPart_Thinking{Thinking: "hmm"},
	}
	if frameBytes(t, thinking) != streamPartFixture(t, "stream_part_thinking.json") {
		t.Fatalf("thinking drifted:\n got %s\nwant %s", frameBytes(t, thinking), streamPartFixture(t, "stream_part_thinking.json"))
	}

	toolCall := &consolev1.ModelStreamPart{
		Part: &consolev1.ModelStreamPart_ToolCall{ToolCall: &consolev1.ToolCallPreview{
			Id: "c1", Name: "sh",
		}},
	}
	if frameBytes(t, toolCall) != streamPartFixture(t, "stream_part_tool_call.json") {
		t.Fatalf("toolCall drifted:\n got %s\nwant %s", frameBytes(t, toolCall), streamPartFixture(t, "stream_part_tool_call.json"))
	}

	// Oneof carries exactly one arm: no empty-object part is encodable.
	empty, err := protojson.Marshal(&consolev1.ModelStreamPart{})
	if err != nil {
		t.Fatal(err)
	}
	if compactJSON(t, empty) != `{}` {
		t.Fatalf("empty part must encode bare: %s", empty)
	}
}
