// HTTP coverage for POST /api/sessions/from-repo — the one-tap repo-picker
// path: clone (or reuse), register, and create the session the client will
// prompt next.
package tests

import (
	"encoding/json"
	"io"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"
	"google.golang.org/protobuf/encoding/protojson"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/db"
	consolev1 "github.com/Adelodunpeter25/console/apps/server-go/internal/gen/console/v1"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/routes"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/services"
	projectssvc "github.com/Adelodunpeter25/console/apps/server-go/internal/services/projects"
)

// repoTaskApp builds an app with only the from-repo route, backed by an
// in-memory DB, cloning into a temp projects root.
func repoTaskApp(t *testing.T) *fiber.App {
	t.Helper()
	manager, err := db.Open(db.OpenOptions{Path: ":memory:"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(manager.Close)
	t.Setenv("CONSOLE_PROJECTS_DIR", t.TempDir())
	app := fiber.New()
	routes.RegisterRepoTaskRoutes(app,
		projectssvc.NewProjectService(manager),
		projectssvc.NewProjectBootstrapService(),
		services.NewSessionService(manager))
	return app
}

// repoTaskPost posts a JSON body and returns status plus the raw response.
func repoTaskPost(t *testing.T, app *fiber.App, body string) (int, string) {
	t.Helper()
	req := httptest.NewRequest("POST", "/api/sessions/from-repo", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req, 30000)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(raw)
}

// decodeRepoTask pulls the project and session protos out of the
// {success, data:{project, session}} envelope.
func decodeRepoTask(t *testing.T, raw string) (*consolev1.ProjectInfo, *consolev1.SessionHeader) {
	t.Helper()
	var envelope struct {
		Success bool `json:"success"`
		Data    struct {
			Project json.RawMessage `json:"project"`
			Session json.RawMessage `json:"session"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(raw), &envelope); err != nil || !envelope.Success {
		t.Fatalf("envelope: %s", raw)
	}
	opts := protojson.UnmarshalOptions{DiscardUnknown: true}
	proj := &consolev1.ProjectInfo{}
	if err := opts.Unmarshal(envelope.Data.Project, proj); err != nil {
		t.Fatalf("project: %s", envelope.Data.Project)
	}
	sess := &consolev1.SessionHeader{}
	if err := opts.Unmarshal(envelope.Data.Session, sess); err != nil {
		t.Fatalf("session: %s", envelope.Data.Session)
	}
	return proj, sess
}

// runRepoGit runs git in dir, failing the test on error. Isolated HOME keeps
// the test machine's git config out of the fixture.
func runRepoGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_CONFIG_NOSYSTEM=1", "HOME="+t.TempDir())
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

// repoTaskOriginRepo builds a local repo to clone from. A local path needs no
// credentials, so these tests never touch the network.
func repoTaskOriginRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	runRepoGit(t, dir, "init", "-b", "main")
	runRepoGit(t, dir, "-c", "user.name=t", "-c", "user.email=t@t", "commit", "--allow-empty", "-m", "init")
	return dir
}

func TestFromRepoClonesRegistersCreatesSession(t *testing.T) {
	app := repoTaskApp(t)
	origin := repoTaskOriginRepo(t)

	code, raw := repoTaskPost(t, app, `{"url":`+jsonStr(origin)+`}`)
	if code != 200 {
		t.Fatalf("from-repo: %d %s", code, raw)
	}
	proj, sess := decodeRepoTask(t, raw)

	if proj.GetName() != filepath.Base(origin) {
		t.Fatalf("project name = %q", proj.GetName())
	}
	wantDir := filepath.Join(os.Getenv("CONSOLE_PROJECTS_DIR"), filepath.Base(origin))
	if proj.GetPath() != wantDir {
		t.Fatalf("project path = %q, want %q", proj.GetPath(), wantDir)
	}
	if !bootstrapGitOK(t, proj.GetPath(), "rev-parse", "--is-inside-work-tree") {
		t.Fatalf("cloned project is not a git repo")
	}
	if sess.GetId() == "" {
		t.Fatalf("session was not created: %s", raw)
	}
	if sess.GetCwd() != proj.GetPath() {
		t.Fatalf("session cwd = %q, want %q", sess.GetCwd(), proj.GetPath())
	}
	if sess.GetProjectId() != proj.GetId() {
		t.Fatalf("session project = %q, want %q", sess.GetProjectId(), proj.GetId())
	}
}

// A scheme URL must name the project after the repo, not the scheme —
// "https://github.com/o/r" once registered a project literally called
// "https". file:// clones locally, so this exercises the scheme-cut path
// without touching the network.
func TestFromRepoNamesProjectFromSchemeURL(t *testing.T) {
	app := repoTaskApp(t)
	origin := repoTaskOriginRepo(t)

	code, raw := repoTaskPost(t, app, `{"url":`+jsonStr("file://"+origin)+`}`)
	if code != 200 {
		t.Fatalf("from-repo: %d %s", code, raw)
	}
	proj, _ := decodeRepoTask(t, raw)

	if want := filepath.Base(origin); proj.GetName() != want {
		t.Fatalf("project name = %q, want %q", proj.GetName(), want)
	}
}

// Tapping the same repo twice must reuse the registered project, not clone a
// second copy (which would refuse with target-not-empty, failing the tap).
func TestFromRepoReusesRegisteredProject(t *testing.T) {
	app := repoTaskApp(t)
	origin := repoTaskOriginRepo(t)
	body := `{"url":` + jsonStr(origin) + `}`

	code, raw := repoTaskPost(t, app, body)
	if code != 200 {
		t.Fatalf("first call: %d %s", code, raw)
	}
	_, first := decodeRepoTask(t, raw)

	code, raw = repoTaskPost(t, app, body)
	if code != 200 {
		t.Fatalf("second call: %d %s", code, raw)
	}
	_, second := decodeRepoTask(t, raw)

	// Same task's project, new task's session.
	if second.GetProjectId() != first.GetProjectId() {
		t.Fatalf("second call made a new project: %s vs %s", second.GetProjectId(), first.GetProjectId())
	}
	if second.GetId() == first.GetId() {
		t.Fatalf("second call reused the session; each tap is a new task")
	}
}

// The GitHub-not-connected contract is the same one the clone dialog uses:
// 409 with text that points at Accounts.
func TestFromRepoUnconnectedGitHubIsConflict(t *testing.T) {
	app := repoTaskApp(t)
	t.Setenv("GITHUB_CREDENTIALS_PATH", filepath.Join(t.TempDir(), "github-creds.json"))

	code, raw := repoTaskPost(t, app, `{"url":"https://github.com/octocat/private.git"}`)
	if code != 409 {
		t.Fatalf("want 409, got %d %s", code, raw)
	}
	if !strings.Contains(raw, "Accounts") {
		t.Fatalf("error must name Accounts as the fix: %s", raw)
	}
}

func TestFromRepoValidatesBody(t *testing.T) {
	app := repoTaskApp(t)

	for name, body := range map[string]string{
		"missing url":       `{}`,
		"invalid url":       `{"url":"nonsense"}`,
		"traversal as name": `{"url":` + jsonStr(repoTaskOriginRepo(t)) + `,"name":"../escape"}`,
	} {
		code, raw := repoTaskPost(t, app, body)
		if code != 400 {
			t.Fatalf("%s: want 400, got %d %s", name, code, raw)
		}
	}
}
