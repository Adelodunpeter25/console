// Queued prompts migration coverage: proto-built rows must match the shared
// golden fixture, and the queue routes keep their validation.
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

func queueFixture(t *testing.T) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "proto", "testdata", "session", "queue.json"))
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(string(raw))
}

func TestQueuedPromptProtoMatchesFixture(t *testing.T) {
	dims := &consolev1.BrowserAnnotationRect{X: 1, Y: 2, Width: 3, Height: 4}
	msg := &consolev1.QueuedPrompt{
		Id: "q1", SessionId: "s1", Prompt: "fix it",
		ContextFiles: []string{"a.ts"},
		Attachments:  []*consolev1.ImageAttachment{{Data: "aGk=", MimeType: "image/png"}},
		Annotations: []*consolev1.BrowserAnnotation{{
			Id: "an1", Url: "https://x", Selector: "#b", HtmlSnippet: "<b>",
			Dimensions: dims, ComputedStyles: map[string]string{"color": "red"},
			UserComment: "look",
		}},
		ModelId: &[]string{"m"}[0], Provider: &[]string{"p"}[0],
		ApprovalMode: &[]string{"auto"}[0], CreatedAt: "2024-05-01T12:00:00Z",
	}
	raw, err := protojson.Marshal(msg)
	if err != nil {
		t.Fatal(err)
	}
	if compactJSON(t, raw) != queueFixture(t) {
		t.Fatalf("queue drifted:\n got %s\nwant %s", compactJSON(t, raw), queueFixture(t))
	}

	// Fixture stays parseable with unknown-field tolerance.
	var decoded consolev1.QueuedPrompt
	if err := (protojson.UnmarshalOptions{DiscardUnknown: true}).Unmarshal([]byte(queueFixture(t)), &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.GetPrompt() != "fix it" || len(decoded.GetAnnotations()) != 1 {
		t.Fatalf("decoded: %+v", &decoded)
	}

	// Minimal row omits every optional/repeated key, like the old omitempty.
	var _ proto.Message = msg
	minimal, err := protojson.Marshal(&consolev1.QueuedPrompt{Id: "q", SessionId: "s", Prompt: "hi", CreatedAt: "2024-05-01T12:00:00Z"})
	if err != nil {
		t.Fatal(err)
	}
	if compactJSON(t, minimal) != `{"id":"q","sessionId":"s","prompt":"hi","createdAt":"2024-05-01T12:00:00Z"}` {
		t.Fatalf("minimal shape: %s", minimal)
	}
}

func TestQueueRoutes(t *testing.T) {
	sessions := helpers.NewRunSessions(t)
	header := helpers.CreateRunSession(t, sessions)
	app := fiber.New()
	routes.RegisterRunRoutes(app, run.NewService(sessions))

	post := func(body string) (int, map[string]any) {
		t.Helper()
		req := httptest.NewRequest("POST", "/api/sessions/"+header.ID+"/queue", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		resp, err := app.Test(req, 10000)
		if err != nil {
			t.Fatal(err)
		}
		raw, _ := io.ReadAll(resp.Body)
		var envelope map[string]any
		if err := json.Unmarshal(raw, &envelope); err != nil {
			t.Fatalf("envelope: %s", raw)
		}
		return resp.StatusCode, envelope
	}

	// Validation preserved: prompt required.
	if code, _ := post(`{"prompt":""}`); code != 400 {
		t.Fatalf("empty prompt must 400, got %d", code)
	}

	code, envelope := post(`{"prompt":"fix it","contextFiles":["a.ts"],"modelId":"m","provider":"p","approvalMode":"auto","attachments":[{"data":"aGk=","mimeType":"image/png"}],"annotations":[{"id":"an1","url":"https://x","selector":"#b","htmlSnippet":"<b>","userComment":"look"}]}`)
	if code != 200 {
		t.Fatalf("queue post: %d %v", code, envelope)
	}
	data, _ := envelope["data"].(map[string]any)
	if data["prompt"] != "fix it" || data["sessionId"] != header.ID {
		t.Fatalf("queued row: %v", data)
	}
	if data["contextFiles"] == nil || data["modelId"] != "m" {
		t.Fatalf("optional fields must survive: %v", data)
	}
	if _, ok := data["thinkingLevel"]; ok {
		t.Fatalf("server never populates thinkingLevel: %v", data)
	}
	if data["attachments"] == nil || data["annotations"] == nil {
		t.Fatalf("attachments/annotations must survive: %v", data)
	}

	// GET returns the staged row.
	req := httptest.NewRequest("GET", "/api/sessions/"+header.ID+"/queue", nil)
	resp, err := app.Test(req, 10000)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := io.ReadAll(resp.Body)
	var get struct {
		Success bool             `json:"success"`
		Data    *json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(raw, &get); err != nil || !get.Success || get.Data == nil {
		t.Fatalf("get queue: %s", raw)
	}
	var row consolev1.QueuedPrompt
	if err := (protojson.UnmarshalOptions{DiscardUnknown: true}).Unmarshal(*get.Data, &row); err != nil {
		t.Fatalf("get row must decode as proto: %v", err)
	}

	// PUT on a fresh session 404s.
	other := helpers.CreateRunSession(t, sessions)
	putReq := httptest.NewRequest("PUT", "/api/sessions/"+other.ID+"/queue", strings.NewReader(`{"prompt":"x"}`))
	putReq.Header.Set("Content-Type", "application/json")
	if resp, err := app.Test(putReq, 10000); err != nil || resp.StatusCode != 404 {
		t.Fatalf("put without staged prompt must 404: %v", resp)
	}

	// DELETE discards; GET is null after.
	delReq := httptest.NewRequest("DELETE", "/api/sessions/"+header.ID+"/queue", nil)
	delResp, err := app.Test(delReq, 10000)
	if err != nil || delResp.StatusCode != 200 {
		t.Fatalf("delete: %v %v", delResp, err)
	}
	req2 := httptest.NewRequest("GET", "/api/sessions/"+header.ID+"/queue", nil)
	resp2, err := app.Test(req2, 10000)
	if err != nil {
		t.Fatal(err)
	}
	raw2, _ := io.ReadAll(resp2.Body)
	var get2 struct {
		Success bool `json:"success"`
	}
	if err := json.Unmarshal(raw2, &get2); err != nil || !get2.Success {
		t.Fatalf("get after delete: %s", raw2)
	}
	if !strings.Contains(string(raw2), `"data":null`) {
		t.Fatalf("empty queue must be null: %s", raw2)
	}
}
