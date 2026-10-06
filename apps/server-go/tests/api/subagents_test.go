// Subagents migration coverage: proto-built rows must match the shared
// golden fixture, and the endpoint keeps its shape.
package tests

import (
	"encoding/json"
	"io"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	consolev1 "github.com/Adelodunpeter25/console/apps/server-go/internal/gen/console/v1"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/routes"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/run"
	"github.com/Adelodunpeter25/console/apps/server-go/tests/helpers"
)

func subagentFixture(t *testing.T, name string) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "proto", "testdata", "session", name))
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(string(raw))
}

func TestSubagentProtoMatchesFixture(t *testing.T) {
	msg := &consolev1.SubagentInfo{
		SubagentId: "sub1", ParentToolCallId: "call1", Name: "Explore",
		Role: "explorer", Prompt: "find things",
		MaxTurns: 5, CurrentTurn: 2, Status: "running",
		Summary: strptr("looking"),
		Activities: []*consolev1.SubagentActivityItem{{
			TurnIndex: 2, ToolCallId: "c9", ToolName: "read",
			Summary: strptr("reading"),
			Args:    []byte(`{"path":"/x"}`),
			Status:  "completed",
		}},
		CreatedAt: 1700000000000, UpdatedAt: 1700000000001,
	}
	elements := []json.RawMessage{json.RawMessage(compactJSON(t, mustProtoBytes(t, msg)))}
	encoded, err := json.Marshal(elements)
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(encoded)) != subagentFixture(t, "subagents.json") {
		t.Fatalf("subagents drifted:\n got %s\nwant %s", encoded, subagentFixture(t, "subagents.json"))
	}

	// Fixture stays parseable with unknown-field tolerance.
	var decoded consolev1.SubagentInfo
	unmarshal := protojson.UnmarshalOptions{DiscardUnknown: true}
	if err := unmarshal.Unmarshal([]byte(subagentFixture(t, "subagents.json")[1:len(subagentFixture(t, "subagents.json"))-1]), &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.GetMaxTurns() != 5 || len(decoded.GetActivities()) != 1 {
		t.Fatalf("decoded: %+v", &decoded)
	}
}

func mustProtoBytes(t *testing.T, msg proto.Message) []byte {
	t.Helper()
	raw, err := protojson.Marshal(msg)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func TestSessionSubagentsRoute(t *testing.T) {
	sessions := helpers.NewRunSessions(t)
	header := helpers.CreateRunSession(t, sessions)
	app := fiber.New()
	routes.RegisterSessionRoutes(app, sessions, run.NewService(sessions))

	// No live subagents: empty array, never null.
	req := httptest.NewRequest("GET", "/api/sessions/"+header.ID+"/subagents", nil)
	resp, err := app.Test(req, 10000)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := io.ReadAll(resp.Body)
	var envelope struct {
		Success bool            `json:"success"`
		Data    json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil || !envelope.Success {
		t.Fatalf("envelope: %s", raw)
	}
	if strings.TrimSpace(string(envelope.Data)) != "[]" {
		t.Fatalf("empty subagents must be []: %s", envelope.Data)
	}

	// Unknown session still succeeds with an empty list (service returns
	// empty, not an error, for unknown ids).
	req = httptest.NewRequest("GET", "/api/sessions/nope/subagents", nil)
	if resp, err := app.Test(req, 10000); err != nil || resp.StatusCode != 200 {
		t.Fatalf("unknown session: %v", resp)
	}
}
