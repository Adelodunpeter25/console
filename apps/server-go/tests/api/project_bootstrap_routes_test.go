// HTTP coverage for the two project-bootstrap routes: the happy paths
// register a real project, refusals map onto the right status code, and the
// 409 for "not connected" is what lets the dialog point at Accounts instead
// of blaming the URL field.
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
	"github.com/Adelodunpeter25/console/apps/server-go/internal/providers/github"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/routes"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/services"
)

// bootstrapRouteApp builds an app with only the project routes, backed by an
// in-memory DB.
func bootstrapRouteApp(t *testing.T) *fiber.App {
	t.Helper()
	manager, err := db.Open(db.OpenOptions{Path: ":memory:"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(manager.Close)
	app := fiber.New()
	routes.RegisterProjectRoutes(app, services.NewProjectService(manager), services.NewProjectBootstrapService())
	return app
}

func doPost(t *testing.T, app *fiber.App, path, body string) (int, string) {
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

// decodeProject pulls the ProjectInfo out of the {success, data} envelope.
// Returns a pointer: a generated message embeds a mutex, so returning it by
// value trips `go vet`'s copylocks check.
func decodeProject(t *testing.T, raw string) *consolev1.ProjectInfo {
	t.Helper()
	var envelope struct {
		Success bool            `json:"success"`
		Data    json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal([]byte(raw), &envelope); err != nil || !envelope.Success {
		t.Fatalf("envelope: %s", raw)
	}
	info := &consolev1.ProjectInfo{}
	if err := (protojson.UnmarshalOptions{DiscardUnknown: true}).Unmarshal(envelope.Data, info); err != nil {
		t.Fatalf("data: %s", envelope.Data)
	}
	return info
}

func TestInitRouteCreatesAndRegistersProject(t *testing.T) {
	app := bootstrapRouteApp(t)
	parent := t.TempDir()

	code, raw := doPost(t, app, "/api/projects/init",
		`{"name":"blank-one","parentDir":`+jsonStr(parent)+`}`)
	if code != 200 {
		t.Fatalf("init: %d %s", code, raw)
	}

	info := decodeProject(t, raw)
	if info.GetName() != "blank-one" {
		t.Fatalf("name = %q", info.GetName())
	}
	if info.GetPath() != filepath.Join(parent, "blank-one") {
		t.Fatalf("path = %q", info.GetPath())
	}
	// The directory really exists and really is a repo.
	if !bootstrapGitOK(t, info.GetPath(), "rev-parse", "--is-inside-work-tree") {
		t.Fatalf("created project is not a git repo")
	}
}

func TestInitRouteValidatesBody(t *testing.T) {
	app := bootstrapRouteApp(t)

	for name, body := range map[string]string{
		"missing name":      `{"parentDir":"/tmp"}`,
		"missing parentDir": `{"name":"x"}`,
		"empty name":        `{"name":"  ","parentDir":"/tmp"}`,
		"missing parent dir on disk": `{"name":"x","parentDir":"/no/such/dir"}`,
	} {
		code, raw := doPost(t, app, "/api/projects/init", body)
		if code != 400 {
			t.Fatalf("%s: want 400, got %d %s", name, code, raw)
		}
	}
}

func TestInitRouteRefusesNonEmptyTarget(t *testing.T) {
	app := bootstrapRouteApp(t)
	parent := t.TempDir()

	existing := filepath.Join(parent, "taken")
	if err := os.MkdirAll(existing, 0o755); err != nil {
		t.Fatal(err)
	}
	keep := filepath.Join(existing, "keep.txt")
	if err := os.WriteFile(keep, []byte("mine"), 0o644); err != nil {
		t.Fatal(err)
	}

	code, raw := doPost(t, app, "/api/projects/init",
		`{"name":"taken","parentDir":`+jsonStr(parent)+`}`)
	if code != 400 {
		t.Fatalf("want 400, got %d %s", code, raw)
	}
	if !strings.Contains(raw, "already exists") {
		t.Fatalf("error must be displayable: %s", raw)
	}
	if _, err := os.Stat(keep); err != nil {
		t.Fatalf("refusal disturbed existing content: %v", err)
	}
}

func TestCloneRouteClonesAndRegisters(t *testing.T) {
	app := bootstrapRouteApp(t)
	parent := t.TempDir()
	origin := routeOriginRepo(t)

	code, raw := doPost(t, app, "/api/projects/clone",
		`{"url":`+jsonStr(origin)+`,"parentDir":`+jsonStr(parent)+`}`)
	if code != 200 {
		t.Fatalf("clone: %d %s", code, raw)
	}

	info := decodeProject(t, raw)
	// Name defaults from the source folder when the client sends none.
	if info.GetName() != filepath.Base(origin) {
		t.Fatalf("default name = %q, want %q", info.GetName(), filepath.Base(origin))
	}
	if !bootstrapGitOK(t, info.GetPath(), "rev-parse", "--is-inside-work-tree") {
		t.Fatalf("cloned project is not a git repo")
	}
}

func TestCloneRouteHonorsExplicitName(t *testing.T) {
	app := bootstrapRouteApp(t)
	parent := t.TempDir()
	origin := routeOriginRepo(t)

	code, raw := doPost(t, app, "/api/projects/clone",
		`{"url":`+jsonStr(origin)+`,"parentDir":`+jsonStr(parent)+`,"name":"renamed"}`)
	if code != 200 {
		t.Fatalf("clone: %d %s", code, raw)
	}
	info := decodeProject(t, raw)
	if info.GetName() != "renamed" || filepath.Base(info.GetPath()) != "renamed" {
		t.Fatalf("explicit name ignored: name=%q path=%q", info.GetName(), info.GetPath())
	}
}

// The 409 is the contract with the dialog: it is what makes the client offer
// "connect in Accounts" rather than treating this as a bad URL.
func TestCloneRouteUnconnectedGitHubIsConflict(t *testing.T) {
	app := bootstrapRouteApp(t)
	parent := t.TempDir()
	t.Setenv("GITHUB_CREDENTIALS_PATH", filepath.Join(t.TempDir(), "github-creds.json"))

	code, raw := doPost(t, app, "/api/projects/clone",
		`{"url":"https://github.com/octocat/private.git","parentDir":`+jsonStr(parent)+`}`)
	if code != 409 {
		t.Fatalf("want 409, got %d %s", code, raw)
	}
	if !strings.Contains(raw, "Accounts") {
		t.Fatalf("error must name Accounts as the fix: %s", raw)
	}
	// Nothing may be left on disk by a refused clone.
	entries, err := os.ReadDir(parent)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("refused clone left %d entries behind", len(entries))
	}
}

func TestCloneRouteValidatesBody(t *testing.T) {
	app := bootstrapRouteApp(t)

	for name, body := range map[string]string{
		"missing url":        `{"parentDir":"/tmp"}`,
		"missing parentDir":  `{"url":"https://github.com/a/b.git"}`,
		"invalid url":        `{"url":"nonsense","parentDir":"/tmp"}`,
		"missing parent dir": `{"url":"https://github.com/a/b.git","parentDir":"/no/such/dir"}`,
	} {
		code, raw := doPost(t, app, "/api/projects/clone", body)
		if code != 400 {
			t.Fatalf("%s: want 400, got %d %s", name, code, raw)
		}
	}
}

// A stored token must clear the 409 guard. The clone itself may still fail
// without network, but never with "not connected".
func TestCloneRouteStoredTokenIsNotBlocked(t *testing.T) {
	app := bootstrapRouteApp(t)
	parent := t.TempDir()
	t.Setenv("GITHUB_CREDENTIALS_PATH", filepath.Join(t.TempDir(), "github-creds.json"))
	if err := github.SaveCredential(github.Credential{Token: "ghp_route_test"}); err != nil {
		t.Fatal(err)
	}

	code, raw := doPost(t, app, "/api/projects/clone",
		`{"url":"https://github.com/octocat/hello.git","parentDir":`+jsonStr(parent)+`}`)
	if code == 409 {
		t.Fatalf("a stored token must not produce 409: %s", raw)
	}
}

// routeOriginRepo builds a local repo to clone from. A local path needs no
// credentials, so these tests never touch the network.
func routeOriginRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	runRouteGit(t, dir, "init", "-b", "main")
	runRouteGit(t, dir, "-c", "user.name=t", "-c", "user.email=t@t", "commit", "--allow-empty", "-m", "init")
	return dir
}

func runRouteGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_CONFIG_NOSYSTEM=1", "HOME="+t.TempDir())
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func bootstrapGitOK(t *testing.T, dir string, args ...string) bool {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_CONFIG_NOSYSTEM=1", "HOME="+t.TempDir())
	return cmd.Run() == nil
}

// jsonStr renders a path as a JSON string value, quotes included. TempDir
// paths can contain characters that break naive concatenation.
func jsonStr(s string) string {
	raw, _ := json.Marshal(s)
	return string(raw)
}