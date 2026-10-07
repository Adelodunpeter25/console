// Project scripts migration coverage: proto-built payloads must match the
// shared golden fixtures, the stream emits oneof frames, and CRUD keeps its
// semantics. Reuses the service harness from scripts_test.go.
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
)

func scriptFixture(t *testing.T, name string) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "proto", "testdata", "scripts", name))
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(string(raw))
}

func TestScriptProtoMatchesFixture(t *testing.T) {
	empty := ""
	exit3 := int32(3)
	check := func(name string, msg proto.Message) {
		t.Helper()
		raw, err := protojson.Marshal(msg)
		if err != nil {
			t.Fatal(err)
		}
		if compactJSON(t, raw) != scriptFixture(t, name) {
			t.Fatalf("%s drifted:\n got %s\nwant %s", name, compactJSON(t, raw), scriptFixture(t, name))
		}
	}

	check("script.json", &consolev1.ProjectScript{
		Id: "dev", Label: "Dev server", Command: "bun dev",
		Shortcut: &empty, Persistent: true,
	})
	check("scripts_result.json", &consolev1.ProjectScriptsResult{
		ProjectId: "p1",
		Scripts: []*consolev1.ProjectScript{{
			Id: "dev", Label: "Dev server", Command: "bun dev",
			Shortcut: &empty, Persistent: true,
		}},
		Source: "console.toml",
	})
	check("run.json", &consolev1.ScriptRun{
		RunId: "r1", ProjectId: "p1", ScriptId: "dev", Label: "Dev",
		Persistent: true, Status: "running",
		StartedAt: "2026-01-01T00:00:00.000Z", Stdout: "hi\n",
	})
	check("event_status.json", &consolev1.ScriptRunEvent{Event: &consolev1.ScriptRunEvent_Status{
		Status: &consolev1.ScriptStatusEvent{Status: "running"},
	}})
	check("event_output.json", &consolev1.ScriptRunEvent{Event: &consolev1.ScriptRunEvent_Output{
		Output: &consolev1.ScriptOutputEvent{Stream: "stdout", Text: "hi\n"},
	}})
	check("event_exit.json", &consolev1.ScriptRunEvent{Event: &consolev1.ScriptRunEvent_Exit{
		Exit: &consolev1.ScriptExitEvent{Status: "failed", ExitCode: &exit3},
	}})
	check("stop.json", &consolev1.StopScriptRunResponse{Stopped: true})

	// Fixtures stay parseable with unknown-field tolerance.
	var event consolev1.ScriptRunEvent
	unmarshal := protojson.UnmarshalOptions{DiscardUnknown: true}
	if err := unmarshal.Unmarshal([]byte(scriptFixture(t, "event_output.json")), &event); err != nil {
		t.Fatal(err)
	}
	out, ok := event.GetEvent().(*consolev1.ScriptRunEvent_Output)
	if !ok || out.Output.GetStream() != "stdout" {
		t.Fatalf("decoded: %+v", &event)
	}
}

func scriptRoutesApp(t *testing.T) (*fiber.App, string) {
	t.Helper()
	scripts, projectID, root := newScriptService(t)
	app := fiber.New()
	routes.RegisterScriptRoutes(app, scripts)
	return app, projectID + "|" + root
}

func TestScriptRoutesListAndStop(t *testing.T) {
	app, ids := scriptRoutesApp(t)
	parts := strings.SplitN(ids, "|", 2)
	projectID, root := parts[0], parts[1]
	writeConsoleToml(t, root, `
[scripts.dev]
label = "Dev server"
command = "echo hi"
persistent = true

[scripts.plain]
label = "Plain"
command = "echo yo"

[scripts.sleeper]
label = "Sleeper"
command = "sleep 30"
`)

	get := func(path string) (int, []byte) {
		req := httptest.NewRequest("GET", path, nil)
		resp, err := app.Test(req, 10000)
		if err != nil {
			t.Fatal(err)
		}
		raw, _ := io.ReadAll(resp.Body)
		return resp.StatusCode, raw
	}

	code, raw := get("/api/projects/" + projectID + "/scripts")
	if code != 200 {
		t.Fatalf("list: %d %s", code, raw)
	}
	var envelope struct {
		Success bool `json:"success"`
		Data    struct {
			ProjectID string `json:"projectId"`
			Scripts   []struct {
				ID       string  `json:"id"`
				Shortcut *string `json:"shortcut"`
				Persist  bool    `json:"persistent"`
			} `json:"scripts"`
			Source string `json:"source"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil || !envelope.Success {
		t.Fatalf("envelope: %s", raw)
	}
	if len(envelope.Data.Scripts) != 3 || envelope.Data.Source != "console.toml" {
		t.Fatalf("list: %s", raw)
	}
	// Unset shortcuts encode as "" (never null, never omitted).
	for _, s := range envelope.Data.Scripts {
		if s.Shortcut == nil || *s.Shortcut != "" {
			t.Fatalf("shortcut must be empty string: %+v", s)
		}
	}

	// Unknown project is still a 400.
	if code, _ := get("/api/projects/nope/scripts"); code != 400 {
		t.Fatalf("unknown project must 400: %d", code)
	}

	// Stop flow over HTTP: unknown run 404s, stopped run matches the fixture.
	stop := func(runID string) (int, string) {
		req := httptest.NewRequest("POST", "/api/projects/"+projectID+"/scripts/runs/"+runID+"/stop", nil)
		resp, err := app.Test(req, 10000)
		if err != nil {
			t.Fatal(err)
		}
		raw, _ := io.ReadAll(resp.Body)
		var env struct {
			Success bool            `json:"success"`
			Data    json.RawMessage `json:"data"`
		}
		if err := json.Unmarshal(raw, &env); err != nil {
			t.Fatalf("envelope: %s", raw)
		}
		return resp.StatusCode, strings.TrimSpace(string(env.Data))
	}
	if code, _ := stop("nope"); code != 404 {
		t.Fatalf("unknown stop must 404: %d", code)
	}

	// Start a sleeper, stop it, and compare the stop payload to the fixture.
	startReq := httptest.NewRequest("POST", "/api/projects/"+projectID+"/scripts/sleeper/runs", nil)
	startResp, err := app.Test(startReq, 10000)
	if err != nil {
		t.Fatal(err)
	}
	startRaw, _ := io.ReadAll(startResp.Body)
	var startEnv struct {
		Success bool `json:"success"`
		Data    struct {
			RunID string `json:"runId"`
		} `json:"data"`
	}
	if err := json.Unmarshal(startRaw, &startEnv); err != nil {
		t.Fatalf("start envelope: %s", startRaw)
	}
	if startResp.StatusCode != 201 || !startEnv.Success || startEnv.Data.RunID == "" {
		t.Fatalf("start: %d %s", startResp.StatusCode, startRaw)
	}
	if code, data := stop(startEnv.Data.RunID); code != 200 || data != scriptFixture(t, "stop.json") {
		t.Fatalf("stop: %d %s", code, data)
	}
}

func TestScriptStreamEmitsOneofFrames(t *testing.T) {
	app, ids := scriptRoutesApp(t)
	parts := strings.SplitN(ids, "|", 2)
	projectID, root := parts[0], parts[1]
	writeConsoleToml(t, root, `
[scripts.echoer]
label = "Echoer"
command = "echo hello-stream"
`)

	startReq := httptest.NewRequest("POST", "/api/projects/"+projectID+"/scripts/echoer/runs", nil)
	startResp, err := app.Test(startReq, 10000)
	if err != nil {
		t.Fatal(err)
	}
	startRaw, _ := io.ReadAll(startResp.Body)
	var startEnv struct {
		Success bool `json:"success"`
		Data    struct {
			RunID string `json:"runId"`
		} `json:"data"`
	}
	if err := json.Unmarshal(startRaw, &startEnv); err != nil || !startEnv.Success {
		t.Fatalf("start envelope: %s", startRaw)
	}

	streamReq := httptest.NewRequest(
		"GET", "/api/projects/"+projectID+"/scripts/runs/"+startEnv.Data.RunID+"/stream", nil,
	)
	streamResp, err := app.Test(streamReq, 30000)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(streamResp.Body)
	unmarshal := protojson.UnmarshalOptions{DiscardUnknown: true}
	seen := map[string]bool{}
	sawStdoutText := false
	for _, frame := range strings.Split(string(body), "\n\n") {
		var name, data string
		for _, line := range strings.Split(frame, "\n") {
			if v, ok := strings.CutPrefix(line, "event: "); ok {
				name = strings.TrimSpace(v)
			}
			if v, ok := strings.CutPrefix(line, "data: "); ok {
				data = strings.TrimSpace(v)
			}
		}
		if name == "" || data == "" {
			continue
		}
		var event consolev1.ScriptRunEvent
		if err := unmarshal.Unmarshal([]byte(data), &event); err != nil {
			t.Fatalf("frame %s does not decode: %v (%s)", name, err, data)
		}
		if strings.Contains(data, `"type"`) {
			t.Fatalf("frame must use oneof shape, got legacy type key: %s", data)
		}
		seen[name] = true
		switch v := event.GetEvent().(type) {
		case *consolev1.ScriptRunEvent_Output:
			if v.Output.GetStream() == "stdout" && strings.Contains(v.Output.GetText(), "hello-stream") {
				sawStdoutText = true
			}
		case *consolev1.ScriptRunEvent_Exit:
			if v.Exit.GetStatus() == "" {
				t.Fatalf("exit without status: %s", data)
			}
		}
	}
	for _, want := range []string{"status", "output", "exit"} {
		if !seen[want] {
			t.Fatalf("missing %s frame in:\n%s", want, body)
		}
	}
	if !sawStdoutText {
		t.Fatalf("no stdout frame with script output in:\n%s", body)
	}
}
