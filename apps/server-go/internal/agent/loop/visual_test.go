package loop

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/tools"
)

func TestToolResultVisualRoundTrip(t *testing.T) {
	spec := json.RawMessage(`{"version":1,"title":"T","blocks":[{"type":"text","text":"x"}]}`)
	original := ToolResultMessage{
		Role: RoleToolResult,
		Results: []tools.ToolResult{{
			ToolCallID: "c1",
			ToolName:   "visualize",
			Content:    []map[string]any{{"type": "text", "text": "Rendered visual (1 note)."}},
			Visual:     &tools.VisualArtifact{Version: 1, Spec: spec},
		}},
	}
	raw, err := ToProtoBytes(original)
	if err != nil {
		t.Fatalf("ToProtoBytes: %v", err)
	}
	decoded, err := MessageFromProtoBytes(raw)
	if err != nil {
		t.Fatalf("MessageFromProtoBytes: %v", err)
	}
	msg, ok := decoded.(ToolResultMessage)
	if !ok {
		t.Fatalf("decoded type = %T, want ToolResultMessage", decoded)
	}
	if len(msg.Results) != 1 || msg.Results[0].Visual == nil {
		t.Fatalf("visual missing after round trip: %#v", msg.Results)
	}
	if string(msg.Results[0].Visual.Spec) != string(spec) {
		t.Fatalf("spec = %s, want %s", msg.Results[0].Visual.Spec, spec)
	}
}

// The shared golden fixture is what a persisted visual tool result looks like
// on the wire. The server must keep producing exactly that shape.
func TestToolResultVisualMatchesGoldenFixture(t *testing.T) {
	path := filepath.Join("..", "..", "..", "..", "..", "proto", "testdata", "message", "toolresult_visual.json")
	fixture, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	spec := json.RawMessage(`{"version":1,"title":"Weekly signups","subtitle":"Last 8 weeks","blocks":[{"type":"chart","kind":"line","title":"Signups by week","xLabel":"Week","yLabel":"Users","series":[{"name":"This quarter","points":[{"x":0,"y":120,"label":"W1"},{"x":1,"y":184,"label":"W2"},{"x":2,"y":164,"label":"W3"}]}]}]}`)
	raw, err := ToProtoBytes(ToolResultMessage{
		Role: RoleToolResult,
		Results: []tools.ToolResult{{
			ToolCallID: "c1",
			ToolName:   "visualize",
			Content:    []map[string]any{{"type": "text", "text": "Rendered visual \"Weekly signups\" (1 chart)."}},
			Visual:     &tools.VisualArtifact{Version: 1, Spec: spec},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	var got, want bytes.Buffer
	if err := json.Compact(&got, raw); err != nil {
		t.Fatal(err)
	}
	if err := json.Compact(&want, fixture); err != nil {
		t.Fatal(err)
	}
	if got.String() != want.String() {
		t.Fatalf("wire shape drifted:\n got %s\nwant %s", got.String(), want.String())
	}
}

func TestMalformedVisualIsDropped(t *testing.T) {
	items, err := toolResultsToProto([]tools.ToolResult{{
		ToolCallID: "c1",
		Content:    "summary",
		Visual:     &tools.VisualArtifact{Version: 1, Spec: json.RawMessage(`{"version":1}`)},
	}})
	if err != nil {
		t.Fatal(err)
	}
	items[0].Visual.Spec = []byte("{not json")
	results, err := protoToToolResults(items)
	if err != nil {
		t.Fatalf("protoToToolResults: %v", err)
	}
	if results[0].Visual != nil {
		t.Fatal("malformed visual should be dropped, not restored")
	}
	if results[0].Content != "summary" {
		t.Fatalf("content = %#v, want summary preserved", results[0].Content)
	}
}
