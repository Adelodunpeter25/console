// Interactive request frames migration coverage: proto-built permission,
// question, browser-action, and error payloads must match the shared golden
// fixtures at full-frame level.
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

func requestFixture(t *testing.T, name string) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "proto", "testdata", "event", name))
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(string(raw))
}

func requestFrame(t *testing.T, kind, key string, msg proto.Message) string {
	t.Helper()
	raw, err := protojson.Marshal(msg)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(map[string]any{
		"type": kind, key: json.RawMessage(raw),
	})
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(string(encoded))
}

func TestRequestFramesProtoMatchFixtures(t *testing.T) {
	reason := "needs review"
	perm := &consolev1.PermissionRequest{
		RequestId: "r1", ToolCallId: "c1", ToolName: "sh",
		Args: []byte(`{"path":"a"}`), Tier: "write", Reason: &reason,
	}
	if got := requestFrame(t, "permissionRequest", "request", perm); got != requestFixture(t, "permission_request.json") {
		t.Fatalf("permission drifted:\n got %s\nwant %s", got, requestFixture(t, "permission_request.json"))
	}

	skippable := false
	batch := "b1"
	ask := &consolev1.AskQuestionRequest{
		RequestId: "r1", Question: "q?", Options: []string{"a", "b"},
		IsMultiSelect: true, Skippable: &skippable, BatchId: &batch,
	}
	if got := requestFrame(t, "askQuestion", "request", ask); got != requestFixture(t, "ask_question.json") {
		t.Fatalf("ask drifted:\n got %s\nwant %s", got, requestFixture(t, "ask_question.json"))
	}

	selector := "#b"
	browser := &consolev1.BrowserActionRequest{
		RequestId: "r1", Action: "click", Selector: &selector, TimeoutMs: 5000,
	}
	if got := requestFrame(t, "browserAction", "request", browser); got != requestFixture(t, "browser_action.json") {
		t.Fatalf("browser drifted:\n got %s\nwant %s", got, requestFixture(t, "browser_action.json"))
	}

	if got := requestFrame(t, "error", "error", &consolev1.ErrorPayload{Message: "boom"}); got != requestFixture(t, "error.json") {
		t.Fatalf("error drifted:\n got %s\nwant %s", got, requestFixture(t, "error.json"))
	}

	// skippable:false stays present (never omitted): clients tell explicit
	// false apart from absent.
	if !strings.Contains(requestFixture(t, "ask_question.json"), `"skippable":false`) {
		t.Fatalf("skippable must be explicit: %s", requestFixture(t, "ask_question.json"))
	}
}
