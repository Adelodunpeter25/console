// Session file changes migration coverage: proto-built rows must match the
// shared golden fixtures, and the changes routes keep their validation.
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

func changesFixture(t *testing.T, name string) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "proto", "testdata", "session", name))
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(string(raw))
}

func TestFileChangeProtoMatchesFixtures(t *testing.T) {
	diff := "--- a\n"
	check := func(name string, msg proto.Message) {
		t.Helper()
		raw, err := protojson.Marshal(msg)
		if err != nil {
			t.Fatal(err)
		}
		if compactJSON(t, raw) != changesFixture(t, name) {
			t.Fatalf("%s drifted:\n got %s\nwant %s", name, compactJSON(t, raw), changesFixture(t, name))
		}
	}

	row := &consolev1.SessionFileChange{
		Path: "src/a.ts", TurnIndex: 3, Status: "modified",
		Additions: 10, Deletions: 2, DiffText: &diff,
		Reviewed: true, UpdatedAt: 1700000000000, UserMessageId: "msg_1",
	}
	elements := []json.RawMessage{json.RawMessage(compactJSON(t, mustProtoBytes(t, row)))}
	encoded, err := json.Marshal(elements)
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(encoded)) != changesFixture(t, "changes.json") {
		t.Fatalf("changes drifted:\n got %s\nwant %s", encoded, changesFixture(t, "changes.json"))
	}
	check("change_diff.json", &consolev1.SessionFileChangeDiff{DiffText: "--- a\n"})

	// Fixture stays parseable with unknown-field tolerance.
	var decoded consolev1.SessionFileChange
	unmarshal := protojson.UnmarshalOptions{DiscardUnknown: true}
	if err := unmarshal.Unmarshal([]byte(changesFixture(t, "changes.json")[1:len(changesFixture(t, "changes.json"))-1]), &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.GetTurnIndex() != 3 || decoded.GetAdditions() != 10 || decoded.GetUserMessageId() != "msg_1" {
		t.Fatalf("decoded: %+v", &decoded)
	}
}

func TestSessionChangesRoutes(t *testing.T) {
	sessions := helpers.NewRunSessions(t)
	header := helpers.CreateRunSession(t, sessions)
	app := fiber.New()
	routes.RegisterSessionRoutes(app, sessions, run.NewService(sessions))

	// Fresh session: empty array, never null.
	req := httptest.NewRequest("GET", "/api/sessions/"+header.ID+"/changes", nil)
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
		t.Fatalf("empty changes must be []: %s", envelope.Data)
	}

	// Validation preserved.
	bad := httptest.NewRequest("GET", "/api/sessions/"+header.ID+"/changes/diff", nil)
	if resp, err := app.Test(bad, 10000); err != nil || resp.StatusCode != 400 {
		t.Fatalf("missing path must 400: %v", resp)
	}
	reviewed := httptest.NewRequest("POST", "/api/sessions/"+header.ID+"/changes/reviewed",
		strings.NewReader(`{"path":"","turnIndex":0,"reviewed":true}`))
	reviewed.Header.Set("Content-Type", "application/json")
	if resp, err := app.Test(reviewed, 10000); err != nil || resp.StatusCode != 400 {
		t.Fatalf("missing path must 400: %v", resp)
	}
}
