// Session headers migration coverage: proto-built headers must match the
// shared golden fixtures, and the CRUD routes keep their semantics. Message
// payloads stay raw until the messages slice migrates.
package tests

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
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

func sessionFixture(t *testing.T, name string) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "proto", "testdata", "session", name))
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(string(raw))
}

func canonicalHeader() *consolev1.SessionHeader {
	return &consolev1.SessionHeader{
		Id: "s1", Title: "Demo", Cwd: "/tmp",
		ProjectId: strptr("p1"), ModelId: "gpt-5", Provider: "openai",
		ApprovalMode: "always-ask", ThinkingLevel: strptr("medium"),
		CreatedAt: 1700000000000, UpdatedAt: 1700000000001,
		MessageCount: func() *int32 { v := int32(3); return &v }(),
		Status:       "done",
		Worktree:     &consolev1.SessionWorktree{Path: "/tmp/wt", Branch: "feat", Repo: "/tmp"},
	}
}

func TestSessionHeaderProtoMatchesFixtures(t *testing.T) {
	check := func(name string, msg proto.Message) {
		t.Helper()
		raw, err := protojson.Marshal(msg)
		if err != nil {
			t.Fatal(err)
		}
		if compactJSON(t, raw) != sessionFixture(t, name) {
			t.Fatalf("%s drifted:\n got %s\nwant %s", name, compactJSON(t, raw), sessionFixture(t, name))
		}
	}

	check("header.json", canonicalHeader())
	single, err := protojson.Marshal(canonicalHeader())
	if err != nil {
		t.Fatal(err)
	}
	list, err := json.Marshal([]json.RawMessage{json.RawMessage(compactJSON(t, single))})
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(list)) != sessionFixture(t, "list.json") {
		t.Fatalf("list drifted:\n got %s\nwant %s", list, sessionFixture(t, "list.json"))
	}
	check("delete.json", &consolev1.SessionDeleteResponse{Id: "s1", Deleted: true})
	check("restore.json", &consolev1.SessionRestoreResponse{Id: "s1", Restored: true})
	check("permanent.json", &consolev1.SessionPermanentDeleteResponse{Id: "s1", PermanentlyDeleted: true})

	// Fixture stays parseable with unknown-field tolerance.
	var decoded consolev1.SessionHeader
	unmarshal := protojson.UnmarshalOptions{DiscardUnknown: true}
	if err := unmarshal.Unmarshal([]byte(sessionFixture(t, "header.json")), &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.GetModelId() != "gpt-5" || decoded.GetMessageCount() != 3 {
		t.Fatalf("decoded: %+v", &decoded)
	}
}

func sessionRoutesApp(t *testing.T) (*fiber.App, string) {
	t.Helper()
	sessions := helpers.NewRunSessions(t)
	app := fiber.New()
	routes.RegisterSessionRoutes(app, sessions, run.NewService(sessions))
	return app, ""
}

func TestSessionHeaderRoutes(t *testing.T) {
	app, _ := sessionRoutesApp(t)

	call := func(method, target, body string) (int, map[string]any) {
		var req *http.Request
		if body == "" {
			req = httptest.NewRequest(method, target, nil)
		} else {
			req = httptest.NewRequest(method, target, strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
		}
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
			t.Fatalf("%s %s envelope: %s", method, target, raw)
		}
		var data map[string]any
		if err := json.Unmarshal(envelope.Data, &data); err != nil {
			// Array payloads (list) decode separately below.
			return resp.StatusCode, nil
		}
		return resp.StatusCode, data
	}

	// Create: timestamps arrive as strings, status as string.
	code, created := call("POST", "/api/sessions/", fmt.Sprintf(`{"title":"Demo","cwd":%q}`, t.TempDir()))
	if code != 200 {
		t.Fatalf("create: %d %v", code, created)
	}
	id, _ := created["id"].(string)
	if id == "" {
		t.Fatalf("create: %v", created)
	}
	if _, isString := created["createdAt"].(string); !isString {
		t.Fatalf("createdAt must be a string: %v", created)
	}
	if _, isString := created["updatedAt"].(string); !isString {
		t.Fatalf("updatedAt must be a string: %v", created)
	}

	// List contains the session; patch renames it.
	if code, _ := call("GET", "/api/sessions/", ""); code != 200 {
		t.Fatalf("list: %d", code)
	}
	if code, patched := call("PATCH", "/api/sessions/"+id, `{"title":"Renamed"}`); code != 200 || patched["title"] != "Renamed" {
		t.Fatalf("patch: %d %v", code, patched)
	}

	// Detail mixes the proto header with raw messages.
	req := httptest.NewRequest("GET", "/api/sessions/"+id, nil)
	resp, err := app.Test(req, 10000)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := io.ReadAll(resp.Body)
	var detail struct {
		Success bool `json:"success"`
		Data    struct {
			Header   map[string]any `json:"header"`
			Messages []any          `json:"messages"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &detail); err != nil || !detail.Success {
		t.Fatalf("detail envelope: %s", raw)
	}
	if _, isString := detail.Data.Header["createdAt"].(string); !isString {
		t.Fatalf("detail header timestamps must be strings: %v", detail.Data.Header)
	}

	// Delete returns the exact shape; unknown ids 404.
	delReq := httptest.NewRequest("DELETE", "/api/sessions/"+id, nil)
	delResp, err := app.Test(delReq, 10000)
	if err != nil {
		t.Fatal(err)
	}
	delRaw, _ := io.ReadAll(delResp.Body)
	var delEnv struct {
		Success bool            `json:"success"`
		Data    json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(delRaw, &delEnv); err != nil || !delEnv.Success {
		t.Fatalf("delete envelope: %s", delRaw)
	}
	var delData map[string]any
	if err := json.Unmarshal(delEnv.Data, &delData); err != nil || delData["id"] != id || delData["deleted"] != true {
		t.Fatalf("delete data: %s", delEnv.Data)
	}
	missing := httptest.NewRequest("GET", "/api/sessions/nope", nil)
	if resp, err := app.Test(missing, 10000); err != nil || resp.StatusCode != 404 {
		t.Fatalf("missing session must 404: %v", resp)
	}
}
