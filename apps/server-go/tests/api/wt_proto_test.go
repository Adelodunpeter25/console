// WebTransport wire fixtures: the WT envelope and request shapes must match
// the shared golden fixtures, the contract future clients code against.
package tests

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	consolev1 "github.com/Adelodunpeter25/console/apps/server-go/internal/gen/console/v1"
)

func wtFixture(t *testing.T, name string) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "proto", "testdata", "wt", name))
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(string(raw))
}

func TestWtFrameProtoMatchesFixture(t *testing.T) {
	frame := &consolev1.WtFrame{Seq: 42, Type: "modelStreamPart", Payload: []byte("hi")}
	raw, err := protojson.Marshal(frame)
	if err != nil {
		t.Fatal(err)
	}
	if compactJSON(t, raw) != wtFixture(t, "frame.json") {
		t.Fatalf("frame drifted:\n got %s\nwant %s", compactJSON(t, raw), wtFixture(t, "frame.json"))
	}
	var decoded consolev1.WtFrame
	if err := (protojson.UnmarshalOptions{DiscardUnknown: true}).Unmarshal([]byte(wtFixture(t, "frame.json")), &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.GetSeq() != 42 || decoded.GetType() != "modelStreamPart" || string(decoded.GetPayload()) != "hi" {
		t.Fatalf("decoded: %+v", &decoded)
	}
}

func TestWtChatRunStartProtoMatchesFixture(t *testing.T) {
	req := &consolev1.WtChatRunRequest{
		SessionId: "s1",
		Request: &consolev1.WtChatRunRequest_Start{
			Start: &consolev1.WtChatRunStart{Text: "hello", ContextFiles: []string{"a.ts"}},
		},
	}
	raw, err := protojson.Marshal(req)
	if err != nil {
		t.Fatal(err)
	}
	if compactJSON(t, raw) != wtFixture(t, "chat_run_start.json") {
		t.Fatalf("chat run start drifted:\n got %s\nwant %s", compactJSON(t, raw), wtFixture(t, "chat_run_start.json"))
	}
}

func TestWtBackgroundWorkEventProtoMatchesFixture(t *testing.T) {
	event := &consolev1.BackgroundWorkEvent{
		Type: "backgroundWorkStarted",
		Work: &consolev1.BackgroundWork{
			Id: "work_1", SessionId: "s1", Kind: "shell", Status: "running",
			CreatedAt: "2026-01-01T00:00:00Z",
		},
	}
	raw, err := protojson.Marshal(event)
	if err != nil {
		t.Fatal(err)
	}
	if compactJSON(t, raw) != wtFixture(t, "background_work_event.json") {
		t.Fatalf("background work event drifted:\n got %s\nwant %s", compactJSON(t, raw), wtFixture(t, "background_work_event.json"))
	}
	var _ proto.Message = event
}
