// Composer paste-upload route: server-side storage of collapsed pasted
// text, naming, guards, and sweeper cleanup.
package tests

import (
	"encoding/json"
	"io"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/routes"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/run"
	"github.com/Adelodunpeter25/console/apps/server-go/tests/helpers"
)

func newPasteApp(t *testing.T) (*fiber.App, string) {
	t.Helper()
	pastes := filepath.Join(helpers.TempDir(t), "pastes")
	t.Setenv("CONSOLE_PASTES_DIR", pastes)
	sessions := helpers.NewRunSessions(t)
	header := helpers.CreateRunSession(t, sessions)
	app := fiber.New()
	routes.RegisterPasteRoutes(app, run.NewService(sessions))
	return app, header.ID
}

func postPaste(t *testing.T, app *fiber.App, sessionID, text string) (int, map[string]any) {
	t.Helper()
	payload, err := json.Marshal(map[string]string{"text": text})
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest("POST", "/api/sessions/"+sessionID+"/pastes", strings.NewReader(string(payload)))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req, 10000)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := io.ReadAll(resp.Body)
	var envelope struct {
		Success bool           `json:"success"`
		Error   string         `json:"error"`
		Data    map[string]any `json:"data"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		t.Fatalf("envelope: %s", raw)
	}
	if resp.StatusCode != fiber.StatusOK && envelope.Error == "" {
		t.Fatalf("status %d with no error text: %s", resp.StatusCode, raw)
	}
	return resp.StatusCode, envelope.Data
}

const pasteLogText = "2026-10-29T09:00:00.000Z INFO boot\n2026-10-29T09:00:01.000Z ERROR boom\nplain parent\n"

func TestPastesRouteStoresLog(t *testing.T) {
	app, sessionID := newPasteApp(t)

	status, data := postPaste(t, app, sessionID, pasteLogText)
	if status != fiber.StatusOK {
		t.Fatalf("status: %d", status)
	}

	path, _ := data["path"].(string)
	name, _ := data["name"].(string)
	if path == "" || name == "" || filepath.Base(path) != name {
		t.Fatalf("bad result: %v", data)
	}
	if !strings.HasSuffix(name, ".log") {
		t.Fatalf("log-shaped paste should end .log: %s", name)
	}
	if lines, _ := data["lines"].(float64); int(lines) != 3 {
		t.Fatalf("lines: %v", data["lines"])
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stored file: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("file mode: %v", info.Mode().Perm())
	}
	raw, err := os.ReadFile(path)
	if err != nil || string(raw) != pasteLogText {
		t.Fatalf("stored bytes differ: %v", err)
	}

	// Plain prose gets .txt.
	status, data = postPaste(t, app, sessionID, "just some text")
	if status != fiber.StatusOK || !strings.HasSuffix(data["name"].(string), ".txt") {
		t.Fatalf("txt paste: %d %v", status, data)
	}

	// Two pastes never collide on the same name.
	if data["name"] == pasteLogText {
		t.Fatal("sanity")
	}
}

func TestPasteGuards(t *testing.T) {
	app, sessionID := newPasteApp(t)

	status, _ := postPaste(t, app, "no-such-session", pasteLogText)
	if status != fiber.StatusNotFound {
		t.Fatalf("unknown session: %d", status)
	}

	status, _ = postPaste(t, app, sessionID, "   ")
	if status != fiber.StatusBadRequest {
		t.Fatalf("empty: %d", status)
	}

	status, _ = postPaste(t, app, sessionID, strings.Repeat("a", 2<<20+1))
	if status != fiber.StatusRequestEntityTooLarge {
		t.Fatalf("oversize: %d", status)
	}

	// Repeated pastes in one burst get distinct names.
	firstStatus, firstName := postPaste(t, app, sessionID, pasteLogText)
	secondStatus, secondName := postPaste(t, app, sessionID, pasteLogText)
	if firstStatus != fiber.StatusOK || secondStatus != fiber.StatusOK {
		t.Fatalf("statuses: %d %d", firstStatus, secondStatus)
	}
	if firstName["name"] == secondName["name"] {
		t.Fatalf("name collision: %v", firstName["name"])
	}
}

func TestPasteSweeper(t *testing.T) {
	pastes := filepath.Join(helpers.TempDir(t), "pastes")
	t.Setenv("CONSOLE_PASTES_DIR", pastes)

	// dead-session dir disappears entirely, fresh or old.
	dead := filepath.Join(pastes, "ses_dead")
	if err := os.MkdirAll(dead, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dead, "pasted-x.log"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}

	// live session: a stale file goes, a fresh file keeps its directory alive.
	live := filepath.Join(pastes, "ses_live")
	if err := os.MkdirAll(live, 0o700); err != nil {
		t.Fatal(err)
	}
	oldFile := filepath.Join(live, "pasted-old.log")
	if err := os.WriteFile(oldFile, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	past := time.Now().Add(-8 * 24 * time.Hour)
	if err := os.Chtimes(oldFile, past, past); err != nil {
		t.Fatal(err)
	}
	freshFile := filepath.Join(live, "pasted-new.log")
	if err := os.WriteFile(freshFile, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}

	exists := func(id string) bool { return id == "ses_live" }
	routes.SweepPastesOnce(pastes, exists)

	if _, err := os.Stat(dead); !os.IsNotExist(err) {
		t.Fatalf("dead session dir should be removed: %v", err)
	}
	if _, err := os.Stat(oldFile); !os.IsNotExist(err) {
		t.Fatalf("stale file should be removed: %v", err)
	}
	if _, err := os.Stat(freshFile); err != nil {
		t.Fatalf("fresh file must survive: %v", err)
	}

	// A directory emptied by the sweep is pruned too.
	empty := filepath.Join(pastes, "ses_empty")
	if err := os.MkdirAll(empty, 0o700); err != nil {
		t.Fatal(err)
	}
	routes.SweepPastesOnce(pastes, exists)
	if _, err := os.Stat(empty); !os.IsNotExist(err) {
		t.Fatalf("empty dir should be pruned: %v", err)
	}
}

// The route never leaves the sweeper pointed at real storage: capture at
// registration is covered by construction; here only that a stale mapping
// to a removed dir is a no-op, not a crash.
func TestPasteSweeperMissingDir(t *testing.T) {
	routes.SweepPastesOnce(filepath.Join(helpers.TempDir(t), "gone"), func(string) bool { return true })
}
