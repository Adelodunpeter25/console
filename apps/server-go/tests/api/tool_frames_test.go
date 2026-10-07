// Tool execution frames migration coverage: proto-built call/result
// payloads must match the shared golden fixtures at full-frame level.
// Arguments and result content cross as raw JSON bytes (base64), reusing
// the transcript schema; the loop's echoed call args are dropped.
package tests

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	consolev1 "github.com/Adelodunpeter25/console/apps/server-go/internal/gen/console/v1"
)

func toolFixture(t *testing.T, name string) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "proto", "testdata", "event", name))
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(string(raw))
}

func toolFrame(t *testing.T, key string, payload any) string {
	t.Helper()
	encoded, err := json.Marshal(map[string]any{"type": key, key2field(key): payload})
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(string(encoded))
}

func key2field(kind string) string {
	switch kind {
	case "toolExecutionStart":
		return "calls"
	case "toolExecutionResult":
		return "result"
	default:
		return "results"
	}
}

func mustProtoJSON(t *testing.T, msg proto.Message) json.RawMessage {
	t.Helper()
	raw, err := protojson.Marshal(msg)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func TestToolFramesProtoMatchFixtures(t *testing.T) {
	sig := "sig"
	toolName := "sh"
	calls := []*consolev1.ToolCall{
		{Id: "c1", Name: "sh", Arguments: []byte(`{"cmd":"ls"}`), ThoughtSignature: &sig},
		{Id: "c2", Name: "read"},
	}
	callPayload := make([]json.RawMessage, 0, len(calls))
	for _, c := range calls {
		callPayload = append(callPayload, mustProtoJSON(t, c))
	}
	if got := toolFrame(t, "toolExecutionStart", callPayload); got != toolFixture(t, "tool_execution_start.json") {
		t.Fatalf("start drifted:\n got %s\nwant %s", got, toolFixture(t, "tool_execution_start.json"))
	}

	result := &consolev1.ToolResult{
		ToolCallId: "c1", ToolName: &toolName, Content: []byte(`"ok"`),
	}
	if got := toolFrame(t, "toolExecutionResult", mustProtoJSON(t, result)); got != toolFixture(t, "tool_execution_result.json") {
		t.Fatalf("result drifted:\n got %s\nwant %s", got, toolFixture(t, "tool_execution_result.json"))
	}

	isErr := true
	results := []*consolev1.ToolResult{
		{ToolCallId: "c1", ToolName: &toolName, Content: []byte(`{"a":2,"b":1}`)},
		{ToolCallId: "c2", Content: []byte(`"ok"`), IsError: &isErr},
	}
	resultPayload := make([]json.RawMessage, 0, len(results))
	for _, r := range results {
		resultPayload = append(resultPayload, mustProtoJSON(t, r))
	}
	if got := toolFrame(t, "toolExecutionEnd", resultPayload); got != toolFixture(t, "tool_execution_end.json") {
		t.Fatalf("end drifted:\n got %s\nwant %s", got, toolFixture(t, "tool_execution_end.json"))
	}

	// Fixtures round-trip through the proto types (unknown-tolerant).
	var decoded consolev1.ToolCall
	unmarshal := protojson.UnmarshalOptions{DiscardUnknown: true}
	if err := unmarshal.Unmarshal([]byte(`{"id":"c1","name":"sh","arguments":"eyJjbWQiOiJscyJ9","futureField":1}`), &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.GetId() != "c1" || string(decoded.GetArguments()) != `{"cmd":"ls"}` {
		t.Fatalf("decoded: %+v", &decoded)
	}
}
