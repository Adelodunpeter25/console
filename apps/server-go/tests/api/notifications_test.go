// Notifications migration coverage: proto-built banners must match the
// shared golden fixtures byte for byte (this stream has no envelope).
package tests

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"google.golang.org/protobuf/encoding/protojson"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/loop"
	consolev1 "github.com/Adelodunpeter25/console/apps/server-go/internal/gen/console/v1"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/run"
)

func notificationFixture(t *testing.T, name string) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "proto", "testdata", "notification", name))
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(string(raw))
}

func TestNotificationProtoMatchesFixtures(t *testing.T) {
	attn := &consolev1.NotificationEvent{
		Type: "notification", Kind: "needs_attention", SessionId: "s1",
		Title: "Needs Attention", Subtitle: "Fix the parser",
		Body: "sh is requesting permission",
	}
	raw, err := protojson.Marshal(attn)
	if err != nil {
		t.Fatal(err)
	}
	if compactJSON(t, raw) != notificationFixture(t, "attention.json") {
		t.Fatalf("attention drifted:\n got %s\nwant %s", compactJSON(t, raw), notificationFixture(t, "attention.json"))
	}

	done := run.DoneNotification("s1", "Fix the parser", "all done here")
	raw, err = protojson.Marshal(done)
	if err != nil {
		t.Fatal(err)
	}
	if compactJSON(t, raw) != notificationFixture(t, "done.json") {
		t.Fatalf("done drifted:\n got %s\nwant %s", compactJSON(t, raw), notificationFixture(t, "done.json"))
	}

	// Empty subtitle omits, matching the old omitempty. Clients default it
	// to empty (desktop #[serde(default)], Android = "").
	titled := run.DoneNotification("s1", "", "all done here")
	raw, err = protojson.Marshal(titled)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "subtitle") {
		t.Fatalf("empty subtitle must omit: %s", raw)
	}

	// Builders still produce the documented vocabulary through the bus type.
	attention := run.AttentionNotification("s1", loop.Event{
		Kind:       loop.EventPermissionRequest,
		Permission: nil,
	}, "Fix the parser")
	if attention.GetKind() != "needs_attention" || attention.GetSessionId() != "s1" {
		t.Fatalf("attention builder: %+v", attention)
	}
}
