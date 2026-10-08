// Todo update + subagent lifecycle frames migration coverage. The subagent
// payloads are flattened under the frame's type tag (no nested object),
// which subagentFrame splices into the envelope; this test pins that shape
// alongside the raw proto payloads.
package tests

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/loop"
	consolev1 "github.com/Adelodunpeter25/console/apps/server-go/internal/gen/console/v1"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/routes"
)

func frameFixture(t *testing.T, name string) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "proto", "testdata", "event", name))
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(string(raw))
}

// flatten splices a protojson payload under the type tag, then renders the
// envelope the way the SSE layer does (map keys alphabetical).
func flatten(t *testing.T, kind string, msg proto.Message) string {
	t.Helper()
	raw, err := protojson.Marshal(msg)
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		t.Fatal(err)
	}
	fields["type"], _ = json.Marshal(kind)
	keys := make([]string, 0, len(fields))
	for k := range fields {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	b.WriteByte('{')
	for i, k := range keys {
		if i > 0 {
			b.WriteByte(',')
		}
		key, _ := json.Marshal(k)
		b.Write(key)
		b.WriteByte(':')
		b.Write(fields[k])
	}
	b.WriteByte('}')
	return b.String()
}

func TestTodoUpdateFrameMatchesFixture(t *testing.T) {
	items := []*consolev1.TodoItem{{Id: 1, Content: "a", Status: "pending"}}
	elements := make([]json.RawMessage, 0, len(items))
	for _, item := range items {
		raw, err := protojson.Marshal(item)
		if err != nil {
			t.Fatal(err)
		}
		elements = append(elements, raw)
	}
	data, err := json.Marshal(elements)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(map[string]any{
		"type": "todoUpdate", "items": json.RawMessage(data), "action": "set",
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(encoded)) != frameFixture(t, "todo_update.json") {
		t.Fatalf("todoUpdate drifted:\n got %s\nwant %s", encoded, frameFixture(t, "todo_update.json"))
	}
}

func TestSubagentFramesMatchFixtures(t *testing.T) {
	parent := "c1"
	start := &consolev1.SubagentStartEvent{
		SubagentId: "s1", ParentToolCallId: &parent, Name: "w",
		Role: "tester", Prompt: "do it", MaxTurns: 10,
	}
	if got := flatten(t, "subagentStart", start); got != frameFixture(t, "subagent_start.json") {
		t.Fatalf("subagentStart drifted:\n got %s\nwant %s", got, frameFixture(t, "subagent_start.json"))
	}

	callID, toolName := "c2", "sh"
	activity := &consolev1.SubagentActivityEvent{
		SubagentId: "s1", TurnIndex: 2, ToolCallId: &callID, ToolName: &toolName,
		Args: []byte(`{"cmd":"ls"}`), Status: "running",
	}
	if got := flatten(t, "subagentActivity", activity); got != frameFixture(t, "subagent_activity.json") {
		t.Fatalf("subagentActivity drifted:\n got %s\nwant %s", got, frameFixture(t, "subagent_activity.json"))
	}

	summary := "done"
	end := &consolev1.SubagentEndEvent{
		SubagentId: "s1", Status: "completed", Summary: &summary, TotalTurns: 3,
	}
	if got := flatten(t, "subagentEnd", end); got != frameFixture(t, "subagent_end.json") {
		t.Fatalf("subagentEnd drifted:\n got %s\nwant %s", got, frameFixture(t, "subagent_end.json"))
	}
}

// TestSubagentFrameRouting pins which loop payload maps to which schema
// message, and that unknown payload types drop instead of emitting junk.
func TestSubagentFrameRouting(t *testing.T) {
	start := loop.SubagentStartInfo{SubagentID: "s1", Name: "w", Role: "r", Prompt: "p", MaxTurns: 10}
	_, body, drop := routes.SubagentFrame(loop.Event{
		Kind: loop.EventSubagentStart, Subagent: start,
	})
	if drop {
		t.Fatal("subagentStart must not drop")
	}
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"subagentId":"s1"`) {
		t.Fatalf("flattened frame missing id: %s", raw)
	}

	if _, _, drop := routes.SubagentFrame(loop.Event{Kind: loop.EventSubagentStart, Subagent: "unexpected"}); !drop {
		t.Fatal("unknown subagent payload must drop")
	}
}
