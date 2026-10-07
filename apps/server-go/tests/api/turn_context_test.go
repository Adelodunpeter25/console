// Turn close + context migration coverage: proto-built turn and snapshot
// payloads must match the shared golden fixtures at full-frame level, and
// GET /context serves the canonical shape.
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

func turnFixture(t *testing.T, name string) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "proto", "testdata", "event", name))
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(string(raw))
}

func TestTurnEndProtoMatchesFixture(t *testing.T) {
	turn := &consolev1.AgentAssistantMessage{
		Id: "a1", StopReason: "end_turn",
		Content: []*consolev1.AssistantContentPart{
			{Part: &consolev1.AssistantContentPart_Text{Text: &consolev1.TextPart{Text: "hi"}}},
			{Part: &consolev1.AssistantContentPart_ToolCall{ToolCall: &consolev1.ToolCall{
				Id: "c1", Name: "sh", Arguments: []byte(`{"cmd":"ls"}`),
			}}},
		},
	}
	raw, err := protojson.Marshal(turn)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(map[string]any{
		"type": "modelStreamEnd", "turnId": "t1", "turn": json.RawMessage(raw),
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(encoded)) != turnFixture(t, "model_stream_end.json") {
		t.Fatalf("turn drifted:\n got %s\nwant %s", encoded, turnFixture(t, "model_stream_end.json"))
	}

	// Fixture round-trips through the proto type.
	var decoded consolev1.AgentAssistantMessage
	payload := struct {
		Turn json.RawMessage `json:"turn"`
	}{}
	if err := json.Unmarshal([]byte(turnFixture(t, "model_stream_end.json")), &payload); err != nil {
		t.Fatal(err)
	}
	if err := (protojson.UnmarshalOptions{DiscardUnknown: true}).Unmarshal(payload.Turn, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.GetId() != "a1" || len(decoded.GetContent()) != 2 {
		t.Fatalf("decoded: %+v", &decoded)
	}
	var _ proto.Message = &decoded
}

func TestContextSnapshotProtoMatchesFixture(t *testing.T) {
	snap := &consolev1.ContextSnapshot{
		UsedTokens: 25000, ContextWindow: 200000, PercentUsed: 12.5,
		ThresholdRatio: 0.8, ModelId: "m", Provider: "p", Source: "live",
	}
	raw, err := protojson.Marshal(snap)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(map[string]any{
		"type": "contextUpdate", "context": json.RawMessage(raw),
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(encoded)) != turnFixture(t, "context_update.json") {
		t.Fatalf("snapshot drifted:\n got %s\nwant %s", encoded, turnFixture(t, "context_update.json"))
	}
}

func TestContextRouteServesProtoShape(t *testing.T) {
	sessions := helpers.NewRunSessions(t)
	header := helpers.CreateRunSession(t, sessions)
	app := fiber.New()
	routes.RegisterRunRoutes(app, run.NewService(sessions))

	req := httptest.NewRequest("GET", "/api/sessions/"+header.ID+"/context", nil)
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
		t.Fatalf("context envelope: %s", raw)
	}
	var snap consolev1.ContextSnapshot
	if err := (protojson.UnmarshalOptions{DiscardUnknown: true}).Unmarshal(envelope.Data, &snap); err != nil {
		t.Fatalf("context data must decode as proto: %v", err)
	}
}
