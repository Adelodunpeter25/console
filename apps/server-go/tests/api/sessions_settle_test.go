// The working→done settle fixup on read must not touch updated_at. A GET that
// bumped it would reorder the sidebar, which keys on last activity for stale
// rows.
package tests

import (
	"encoding/json"
	"io"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/routes"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/run"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/types"
	"github.com/Adelodunpeter25/console/apps/server-go/tests/helpers"
)

func TestSettleOnReadKeepsUpdatedAt(t *testing.T) {
	sessions := helpers.NewRunSessions(t)
	app := fiber.New()
	routes.RegisterSessionRoutes(app, sessions, run.NewService(sessions))

	created, err := sessions.Create(types.CreateSessionOptions{
		Title: "Stale",
		Cwd:   t.TempDir(),
	})
	if err != nil {
		t.Fatalf("create session: %v", err)
	}

	// Mark it working with no live run, the stale state the settle targets.
	if err := sessions.UpdateStatus(created.ID, "working"); err != nil {
		t.Fatalf("set working: %v", err)
	}
	before, err := sessions.Header(created.ID)
	if err != nil {
		t.Fatalf("header before: %v", err)
	}

	req := httptest.NewRequest("GET", "/api/sessions/"+created.ID, nil)
	resp, err := app.Test(req, 10000)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := io.ReadAll(resp.Body)
	var detail struct {
		Success bool `json:"success"`
		Data    struct {
			Header map[string]any `json:"header"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &detail); err != nil || !detail.Success {
		t.Fatalf("detail envelope: %s", raw)
	}
	if got := detail.Data.Header["status"]; got != "done" {
		t.Fatalf("GET status = %v, want done", got)
	}

	after, err := sessions.Header(created.ID)
	if err != nil {
		t.Fatalf("header after: %v", err)
	}
	if after.Status != "done" {
		t.Fatalf("persisted status = %q, want done", after.Status)
	}
	if after.UpdatedAt != before.UpdatedAt {
		t.Fatalf("updated_at changed on read: %d -> %d", before.UpdatedAt, after.UpdatedAt)
	}
}
