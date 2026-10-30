package routes

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/loop"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/run"
	backgroundsvc "github.com/Adelodunpeter25/console/apps/server-go/internal/services/background"
	terminalsvc "github.com/Adelodunpeter25/console/apps/server-go/internal/services/terminal"
)

func TestWireFrameBackgroundWorkUsesWorkRecord(t *testing.T) {
	work := backgroundsvc.Work{
		ID:        "work_test",
		SessionID: "session-1",
		Kind:      backgroundsvc.KindShell,
		Status:    backgroundsvc.StatusCompleted,
		Metadata:  map[string]string{"job_id": "job_test", "exit_code": "0"},
	}
	name, body, drop := run.WireFrame(loop.Event{
		Kind:    loop.EventBackgroundWork,
		Message: backgroundsvc.Event{Type: backgroundsvc.EventCompleted, Work: work},
	})
	if drop {
		t.Fatal("background work frame was dropped")
	}
	if name != "backgroundWorkCompleted" {
		t.Fatalf("event name = %q", name)
	}

	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	var envelope map[string]any
	if err := json.Unmarshal(raw, &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope["type"] != "backgroundWorkCompleted" {
		t.Fatalf("envelope type = %v", envelope["type"])
	}
	payload, ok := envelope["work"].(map[string]any)
	if !ok {
		t.Fatalf("work payload = %#v", envelope["work"])
	}
	if payload["id"] != "work_test" || payload["kind"] != "shell" || payload["status"] != "completed" {
		t.Fatalf("work payload = %#v", payload)
	}
	if _, ok := payload["Type"]; ok {
		t.Fatalf("nested background event leaked into wire payload: %#v", payload)
	}
}

func TestBackgroundWorkOutputRouteReadsDetachedShell(t *testing.T) {
	app := fiber.New()
	runs := run.NewService(nil)
	jobs := terminalsvc.NewBashJobManager(nil)
	runs.SetBashJobs(jobs)
	RegisterRunRoutes(app, runs)

	snapshot, err := jobs.Start(terminalsvc.StartBashJobOptions{
		Command:        "printf hello-route",
		OwnerSessionID: "session-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(jobs.KillAll)

	var work backgroundsvc.Work
	items := runs.BackgroundWorkManager().List("session-1")
	if len(items) == 0 {
		t.Fatal("background work was not registered")
	}
	work = items[0]

	deadline := time.Now().Add(2 * time.Second)
	for {
		status, err := jobs.Status(snapshot.JobID, "session-1")
		if err == nil && status.Status != terminalsvc.BashJobRunning {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("background shell did not finish")
		}
		time.Sleep(10 * time.Millisecond)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/sessions/session-1/background-work/"+work.ID+"/output", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d body = %s", resp.StatusCode, body)
	}
	if !strings.Contains(string(body), "hello-route") {
		t.Fatalf("body = %s", body)
	}
}

// readOutputFrames reads the stream until it closes and returns the decoded
// `output` frames.
func readOutputFrames(t *testing.T, app *fiber.App, path string) []map[string]any {
	t.Helper()
	resp, err := app.Test(httptest.NewRequest(http.MethodGet, path, nil), 10000)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d body = %s", resp.StatusCode, raw)
	}
	var frames []map[string]any
	for _, block := range strings.Split(string(raw), "\n\n") {
		for _, line := range strings.Split(block, "\n") {
			if !strings.HasPrefix(line, "data: ") {
				continue
			}
			var frame map[string]any
			if err := json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &frame); err != nil {
				t.Fatalf("bad frame %q: %v", line, err)
			}
			frames = append(frames, frame)
		}
	}
	return frames
}

func TestBackgroundWorkOutputStreamPushesChunksThenCloses(t *testing.T) {
	app := fiber.New()
	runs := run.NewService(nil)
	jobs := terminalsvc.NewBashJobManager(nil)
	runs.SetBashJobs(jobs)
	RegisterRunRoutes(app, runs)

	if _, err := jobs.Start(terminalsvc.StartBashJobOptions{
		Command:        "echo one; sleep 0.3; echo two; sleep 0.3; echo three >&2",
		OwnerSessionID: "session-1",
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(jobs.KillAll)
	items := runs.BackgroundWorkManager().List("session-1")
	if len(items) == 0 {
		t.Fatal("background work was not registered")
	}

	frames := readOutputFrames(t, app, "/api/sessions/session-1/background-work/"+items[0].ID+"/output/stream")
	if len(frames) < 2 {
		t.Fatalf("expected several frames as output arrived, got %d: %v", len(frames), frames)
	}
	var stdout string
	var sawStderr bool
	for _, f := range frames {
		stdout += f["stdout"].(string)
		if e, ok := f["stderr"].(string); ok && strings.Contains(e, "three") {
			sawStderr = true
		}
	}
	if stdout != "one\ntwo\n" {
		t.Fatalf("stdout = %q, want %q (chunks must not repeat)", stdout, "one\ntwo\n")
	}
	if !sawStderr {
		t.Fatalf("stderr never streamed: %v", frames)
	}
	last := frames[len(frames)-1]
	if last["status"] != "exited" {
		t.Fatalf("final status = %v, want exited", last["status"])
	}
}

func TestBackgroundWorkOutputStreamResumesFromCursor(t *testing.T) {
	app := fiber.New()
	runs := run.NewService(nil)
	jobs := terminalsvc.NewBashJobManager(nil)
	runs.SetBashJobs(jobs)
	RegisterRunRoutes(app, runs)

	if _, err := jobs.Start(terminalsvc.StartBashJobOptions{
		Command:        "printf abcdef",
		OwnerSessionID: "session-1",
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(jobs.KillAll)
	items := runs.BackgroundWorkManager().List("session-1")

	frames := readOutputFrames(t, app, "/api/sessions/session-1/background-work/"+items[0].ID+"/output/stream?cursor=3")
	var stdout string
	for _, f := range frames {
		stdout += f["stdout"].(string)
	}
	if stdout != "def" {
		t.Fatalf("stdout = %q, want %q", stdout, "def")
	}
}

func TestBackgroundWorkOutputStreamStopsWhenKilled(t *testing.T) {
	app := fiber.New()
	runs := run.NewService(nil)
	jobs := terminalsvc.NewBashJobManager(nil)
	runs.SetBashJobs(jobs)
	RegisterRunRoutes(app, runs)

	snap, err := jobs.Start(terminalsvc.StartBashJobOptions{
		Command:        "echo started; sleep 30",
		OwnerSessionID: "session-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(jobs.KillAll)
	items := runs.BackgroundWorkManager().List("session-1")
	time.AfterFunc(400*time.Millisecond, func() { _, _ = jobs.Kill(snap.JobID, "session-1") })

	frames := readOutputFrames(t, app, "/api/sessions/session-1/background-work/"+items[0].ID+"/output/stream")
	last := frames[len(frames)-1]
	if last["status"] != "killed" {
		t.Fatalf("final status = %v, want killed", last["status"])
	}
}

func TestBackgroundWorkOutputStreamUnknownWorkIs404(t *testing.T) {
	app := fiber.New()
	runs := run.NewService(nil)
	runs.SetBashJobs(terminalsvc.NewBashJobManager(nil))
	RegisterRunRoutes(app, runs)
	resp, err := app.Test(httptest.NewRequest(http.MethodGet, "/api/sessions/session-1/background-work/nope/output/stream", nil))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", resp.StatusCode)
	}
}
