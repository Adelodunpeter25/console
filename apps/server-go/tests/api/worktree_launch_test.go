// HTTP coverage for POST /api/sessions/with-worktree — the instant worktree
// launch: the session row returns immediately, the worktree provisions and
// the first prompt runs in the background, and a failed launch reports
// launchState=failed with a retry.
package tests

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"google.golang.org/protobuf/encoding/protojson"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/loop"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/stream"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/db"
	consolev1 "github.com/Adelodunpeter25/console/apps/server-go/internal/gen/console/v1"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/run"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/routes"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/services"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/types"
)

// mockLaunchProvider completes one turn with a text event — enough for the
// launch path to persist the user message and settle.
type mockLaunchProvider struct{}

func (m *mockLaunchProvider) RunTurn(ctx context.Context, req loop.TurnRequest, s *stream.Stream[loop.Event]) error {
	s.Push(loop.Event{Kind: loop.EventText, Text: "ok"})
	s.Complete()
	return nil
}

// worktreeLaunchApp builds an app with only the launch routes, backed by an
// in-memory DB, provisioning worktrees into a temp root. The run service's
// provider lookup is a scripted mock, so a launched prompt runs without the
// network.
func worktreeLaunchApp(t *testing.T) (*fiber.App, *services.SessionService) {
	t.Helper()
	manager, err := db.Open(db.OpenOptions{Path: ":memory:"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(manager.Close)
	t.Setenv("CONSOLE_WORKTREES_DIR", t.TempDir())
	sessions := services.NewSessionService(manager)
	runSvc := run.NewService(sessions)
	runSvc.Lookup = func(id string) (loop.Provider, error) {
		return &mockLaunchProvider{}, nil
	}
	app := fiber.New()
	routes.RegisterWorktreeLaunchRoutes(app, sessions, runSvc)
	return app, sessions
}

func worktreeLaunchPost(t *testing.T, app *fiber.App, path, body string) (int, string) {
	t.Helper()
	req := httptest.NewRequest("POST", path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req, 30000)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(raw)
}

// launchRepo builds a real git repo so provisioning has something to branch
// from. Isolated HOME keeps the test machine's git config out of the fixture.
func launchRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	cmd := exec.Command("git", "init", "-b", "main")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_CONFIG_NOSYSTEM=1", "HOME="+t.TempDir())
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, out)
	}
	runRepoGit(t, dir, "-c", "user.name=t", "-c", "user.email=t@t", "commit", "--allow-empty", "-m", "init")
	return dir
}

// launchBody posts to the launch endpoint and returns the new session id.
func launchBody(t *testing.T, app *fiber.App, repo, extra string) string {
	t.Helper()
	body := fmt.Sprintf(`{"cwd":%q,"title":"t","provider":"mock","modelId":"m","approvalMode":"full-access","worktree":{},"prompt":{"prompt":"hello","provider":"mock","modelId":"m","approvalMode":"full-access"}%s}`, repo, extra)
	code, raw := worktreeLaunchPost(t, app, "/api/sessions/with-worktree", body)
	if code != 200 {
		t.Fatalf("with-worktree: %d %s", code, raw)
	}
	var envelope struct {
		Success bool            `json:"success"`
		Data    json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal([]byte(raw), &envelope); err != nil || !envelope.Success {
		t.Fatalf("envelope: %s", raw)
	}
	header := &consolev1.SessionHeader{}
	opts := protojson.UnmarshalOptions{DiscardUnknown: true}
	if err := opts.Unmarshal(envelope.Data, header); err != nil {
		t.Fatalf("session: %s", envelope.Data)
	}
	return header.GetId()
}

// waitLaunch polls the registry until the launch leaves provisioning, then
// returns its state (nil = cleared and ready).
func waitLaunch(t *testing.T, id string) *services.SessionLaunch {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		if launch := services.WorktreeLaunches.Get(id); launch == nil {
			return nil
		} else if launch.State == services.LaunchFailed {
			return launch
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("launch for %s never left provisioning", id)
	return nil
}

func TestWorktreeLaunchCreatesSessionThenWorktree(t *testing.T) {
	app, sessions := worktreeLaunchApp(t)
	repo := launchRepo(t)

	// Create returns immediately with a real session id.
	id := launchBody(t, app, repo, "")
	if id == "" {
		t.Fatalf("session was not created")
	}
	t.Cleanup(func() { services.WorktreeLaunches.Clear(id) })

	// Background provisioning finishes: worktree attached, prompt running.
	if launch := waitLaunch(t, id); launch != nil {
		t.Fatalf("launch failed: %s", launch.Error)
	}
	header, err := sessions.Header(id)
	if err != nil || header == nil {
		t.Fatalf("header: %v", err)
	}
	if header.Worktree == nil {
		t.Fatalf("session has no worktree after launch")
	}
	if _, err := os.Stat(header.Worktree.Path); err != nil {
		t.Fatalf("worktree dir missing: %v", err)
	}
	// The stored prompt ran: the first user message landed.
	if header.MessageCount < 1 {
		t.Fatalf("prompt did not run: messageCount=%d", header.MessageCount)
	}
}

func TestWorktreeLaunchFailureIsRetryable(t *testing.T) {
	app, sessions := worktreeLaunchApp(t)
	repo := launchRepo(t)

	result := launchBody(t, app, repo, `,"worktree":{"baseBranch":"no-such-branch"}`)
	t.Cleanup(func() { services.WorktreeLaunches.Clear(result) })

	launch := waitLaunch(t, result)
	if launch == nil || launch.State != services.LaunchFailed {
		t.Fatalf("expected failed launch, got %+v", launch)
	}

	// Fix the cause and retry: the same payload now provisions and runs.
	runRepoGit(t, repo, "branch", "no-such-branch")
	code, raw := worktreeLaunchPost(t, app, "/api/sessions/"+result+"/with-worktree/retry", "")
	if code != 200 {
		t.Fatalf("retry: %d %s", code, raw)
	}
	if launch := waitLaunch(t, result); launch != nil {
		t.Fatalf("retry failed: %s", launch.Error)
	}
	header, err := sessions.Header(result)
	if err != nil || header == nil || header.Worktree == nil {
		t.Fatalf("worktree missing after retry: %v", err)
	}
}

func TestWorktreeLaunchValidatesBody(t *testing.T) {
	app, _ := worktreeLaunchApp(t)
	for name, body := range map[string]string{
		"missing worktree": `{"cwd":"/tmp","prompt":{"prompt":"hi"}}`,
		"missing prompt":   `{"cwd":"/tmp","worktree":{}}`,
		"missing cwd":      `{"worktree":{},"prompt":{"prompt":"hi"}}`,
	} {
		code, raw := worktreeLaunchPost(t, app, "/api/sessions/with-worktree", body)
		if code != 400 {
			t.Fatalf("%s: want 400, got %d %s", name, code, raw)
		}
	}
}

func TestWorktreeRetryWithoutLaunchIs404(t *testing.T) {
	app, _ := worktreeLaunchApp(t)
	code, _ := worktreeLaunchPost(t, app, "/api/sessions/ghost/with-worktree/retry", "")
	if code != 404 {
		t.Fatalf("want 404, got %d", code)
	}
}

// A crash between worktree attach and registry clear must not leave the
// header reporting "provisioning" forever. The worktree itself is durable
// proof that provisioning finished, so the header omits launch_state.
func TestProvisioningLaunchWithWorktreeReportsReady(t *testing.T) {
	manager, err := db.Open(db.OpenOptions{Path: ":memory:"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(manager.Close)
	t.Setenv("CONSOLE_WORKTREES_DIR", t.TempDir())
	sessions := services.NewSessionService(manager)
	runSvc := run.NewService(sessions)
	app := fiber.New()
	routes.RegisterSessionRoutes(app, sessions, runSvc)
	routes.RegisterWorktreeLaunchRoutes(app, sessions, runSvc)

	repo := launchRepo(t)
	header, err := sessions.Create(types.CreateSessionOptions{Title: "t", Cwd: repo})
	if err != nil {
		t.Fatal(err)
	}
	// Simulate the crash window: the launch is still registered as
	// provisioning, but the worktree was already attached.
	services.WorktreeLaunches.Start(header.ID, types.CreateWorktreeSpec{}, services.LaunchPrompt{Text: "hello"})
	t.Cleanup(func() { services.WorktreeLaunches.Clear(header.ID) })
	if _, err := sessions.AttachWorktree(header.ID, &types.CreateWorktreeSpec{}); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest("GET", "/api/sessions/"+header.ID, nil)
	req.Header.Set("Accept", "application/json")
	resp, err := app.Test(req, 30000)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		t.Fatalf("get session: %d %s", resp.StatusCode, raw)
	}
	var envelope struct {
		Data struct {
			Header json.RawMessage `json:"header"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		t.Fatalf("envelope: %s", raw)
	}
	got := &consolev1.SessionHeader{}
	if err := (protojson.UnmarshalOptions{DiscardUnknown: true}).Unmarshal(envelope.Data.Header, got); err != nil {
		t.Fatalf("header: %s", envelope.Data.Header)
	}
	if got.GetLaunchState() != "" {
		t.Fatalf("launchState = %q, want empty for session with worktree", got.GetLaunchState())
	}
	if got.Worktree == nil {
		t.Fatal("worktree missing from header")
	}
}
