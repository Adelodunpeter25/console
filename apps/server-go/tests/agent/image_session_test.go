// End-to-end check that a tool-result image never reaches the session
// database: a run that reads an image stores a small marker, and the replayed
// history carries no base64.
package tests

import (
	"strings"
	"testing"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/loop"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/tools"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/run"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/types"
	"github.com/Adelodunpeter25/console/apps/server-go/tests/helpers"
)

// A tool result carrying an image is stored without its base64. The stored
// row is what every later turn replays, so this is the check that stops the
// per-turn re-upload.
func TestStoredToolResultHasNoImageBytes(t *testing.T) {
	sessions := helpers.NewRunSessions(t)
	header := helpers.CreateRunSession(t, sessions)

	payload := strings.Repeat("Q", 3_000_000)
	result := tools.ToolResult{
		ToolCallID: "c1",
		ToolName:   "read_file",
		Content: []map[string]any{
			{"type": "text", "text": "Image read: /tmp/shot.png (image/png, 1920x1080)."},
			{"type": "image", "data": payload, "mimeType": "image/png"},
		},
	}
	msg := loop.ToolResultMessage{Role: loop.RoleToolResult, Results: []tools.ToolResult{result}}
	raw, err := loop.ToProtoBytes(msg)
	if err != nil {
		t.Fatal(err)
	}
	if err := sessions.AppendMessage(header.ID, types.AgentMessage{
		ID:   "m1",
		Role: string(loop.RoleToolResult),
		Data: raw,
	}); err != nil {
		t.Fatal(err)
	}

	loaded, err := sessions.Load(header.ID, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if loaded == nil || len(loaded.Messages) != 1 {
		t.Fatalf("expected one stored message, got %d", len(loaded.Messages))
	}
	stored := string(loaded.Messages[0])
	if len(stored) > 8192 {
		t.Fatalf("stored message is too large: %d bytes", len(stored))
	}
	if strings.Contains(stored, payload[:2000]) {
		t.Fatal("stored message still contains the base64 payload")
	}

	// The replayed history must be text-only: no image part to re-upload.
	history := run.DecodeHistory(loaded.Messages)
	if len(history) != 1 {
		t.Fatalf("history length: %d", len(history))
	}
	replayed, ok := history[0].(loop.ToolResultMessage)
	if !ok {
		t.Fatalf("history type: %T", history[0])
	}
	parts, ok := replayed.Results[0].Content.([]any)
	if !ok {
		t.Fatalf("content type: %T", replayed.Results[0].Content)
	}
	for _, part := range parts {
		m, _ := part.(map[string]any)
		if m != nil && m["type"] == "image" {
			t.Fatalf("replayed history still carries an image part: %#v", m)
		}
	}
}
