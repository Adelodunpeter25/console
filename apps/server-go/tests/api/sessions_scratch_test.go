// PATCH /api/sessions/:id project/cwd semantics. The regression: an
// explicit `"projectId": null` (how the desktop clears a project) was
// indistinguishable from an omitted key, so "No Folder" never dropped the
// project link and the scratch dir it points at was never created — every
// terminal for that session then failed PtyManager.Spawn's cwd check.
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

	"github.com/Adelodunpeter25/console/apps/server-go/internal/db"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/routes"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/run"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/services"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/types"
)

// scratchRouteApp wires the session routes over a file-backed store so the
// scratch root on disk matches what Create derives.
func scratchRouteApp(t *testing.T) (*fiber.App, *services.SessionService, *services.ProjectService, string) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("CONSOLE_STORAGE_DIR", dir)
	manager, err := db.Open(db.OpenOptions{Path: filepath.Join(dir, "console-global.db")})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(manager.Close)
	sessions := services.NewSessionService(manager)
	app := fiber.New()
	routes.RegisterSessionRoutes(app, sessions, run.NewService(sessions))
	return app, sessions, services.NewProjectService(manager), dir
}

func patchSession(t *testing.T, app *fiber.App, id, body string) types.SessionHeader {
	t.Helper()
	req := httptest.NewRequest("PATCH", "/api/sessions/"+id, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req, 10000)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("PATCH %s = %d: %s", body, resp.StatusCode, raw)
	}
	var envelope struct {
		Success bool                `json:"success"`
		Data    types.SessionHeader `json:"data"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil || !envelope.Success {
		t.Fatalf("envelope: %s", raw)
	}
	return envelope.Data
}

func TestPatchSessionExplicitNullClearsProject(t *testing.T) {
	app, sessions, projects, storageDir := scratchRouteApp(t)

	projectDir := t.TempDir()
	project, err := projects.Create(services.CreateProjectOptions{Name: "P", Dir: projectDir})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	header, err := sessions.Create(types.CreateSessionOptions{Cwd: projectDir, ProjectID: &project.ID})
	if err != nil {
		t.Fatalf("create session: %v", err)
	}

	target := filepath.Join(storageDir, "scratch", header.ID)
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Fatalf("scratch dir should not exist yet: %v", err)
	}

	// Exactly what the desktop sends for "No Folder": cwd plus explicit null.
	updated := patchSession(t, app, header.ID,
		`{"cwd":`+jsonString(target)+`,"projectId":null}`)
	if updated.ProjectID != nil {
		t.Fatalf("projectId = %q; want nil", *updated.ProjectID)
	}
	if updated.Cwd != target {
		t.Fatalf("cwd = %q; want %q", updated.Cwd, target)
	}
	if st, err := os.Stat(target); err != nil || !st.IsDir() {
		t.Fatalf("scratch dir not created: %v", err)
	}
}

// An omitted projectId must not silently clear a real project link.
func TestPatchSessionOmittedProjectKeepsLink(t *testing.T) {
	app, sessions, projects, _ := scratchRouteApp(t)

	projectDir := t.TempDir()
	project, err := projects.Create(services.CreateProjectOptions{Name: "P", Dir: projectDir})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	header, err := sessions.Create(types.CreateSessionOptions{Cwd: projectDir, ProjectID: &project.ID})
	if err != nil {
		t.Fatalf("create session: %v", err)
	}

	// A cwd under a registered project re-infers the same link.
	updated := patchSession(t, app, header.ID, `{"cwd":`+jsonString(projectDir)+`}`)
	if updated.ProjectID == nil || *updated.ProjectID != project.ID {
		t.Fatalf("projectId = %v; want %q", updated.ProjectID, project.ID)
	}
}

// projectId alone (no cwd key) still clears the link and makes the dir the
// client asked for via its own cwd — here the stored one.
func TestPatchSessionNullWithoutCwdKeyClearsProject(t *testing.T) {
	app, sessions, projects, storageDir := scratchRouteApp(t)

	projectDir := t.TempDir()
	project, err := projects.Create(services.CreateProjectOptions{Name: "P", Dir: projectDir})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	header, err := sessions.Create(types.CreateSessionOptions{Cwd: projectDir, ProjectID: &project.ID})
	if err != nil {
		t.Fatalf("create session: %v", err)
	}

	// Move to the scratch dir first, then clear with only the null.
	target := filepath.Join(storageDir, "scratch", header.ID)
	patchSession(t, app, header.ID, `{"cwd":`+jsonString(target)+`,"projectId":null}`)
	updated := patchSession(t, app, header.ID, `{"projectId":null}`)
	if updated.ProjectID != nil {
		t.Fatalf("projectId = %q; want nil", *updated.ProjectID)
	}
	if updated.Cwd != target {
		t.Fatalf("cwd = %q; want %q", updated.Cwd, target)
	}
}

func jsonString(s string) string {
	raw, _ := json.Marshal(s)
	return string(raw)
}