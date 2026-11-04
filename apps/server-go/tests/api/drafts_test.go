// Chat draft sync: last write wins by updatedAt, whole-draft replace so
// deletions propagate, and a cleared draft stays as an empty tombstone.
package tests

import (
	"encoding/json"
	"io"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/db"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/routes"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/services"
)

type draftEnvelope struct {
	Success bool `json:"success"`
	Data    struct {
		Applied bool `json:"applied"`
		Draft   *struct {
			Text      string `json:"text"`
			UpdatedAt string `json:"updatedAt"`
			Comments  []struct {
				ID   string `json:"id"`
				Text string `json:"text"`
			} `json:"comments"`
		} `json:"draft"`
	} `json:"data"`
}

func draftApp(t *testing.T) *fiber.App {
	t.Helper()
	manager, err := db.Open(db.OpenOptions{Path: ":memory:"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(manager.Close)
	app := fiber.New()
	routes.RegisterDraftRoutes(app, services.NewDraftService(manager))
	return app
}

func draftCall(t *testing.T, app *fiber.App, method, body string) draftEnvelope {
	t.Helper()
	req := httptest.NewRequest(method, "/api/drafts/s1", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req, 10000)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := io.ReadAll(resp.Body)
	var env draftEnvelope
	if err := json.Unmarshal(raw, &env); err != nil || !env.Success {
		t.Fatalf("envelope: %s", raw)
	}
	return env
}

func TestDraftsLastWriteWinsAndClear(t *testing.T) {
	app := draftApp(t)

	if env := draftCall(t, app, "GET", ""); env.Data.Draft != nil {
		t.Fatalf("expected no draft, got %+v", env.Data.Draft)
	}

	first := `{"text":"from desktop","updatedAt":"100","comments":[{"id":"c1","path":"a.rs","startLine":1,"endLine":2,"text":"fix","snippet":"fn a()"}]}`
	if env := draftCall(t, app, "PUT", first); !env.Data.Applied {
		t.Fatal("first write should apply")
	}

	// An older write from a slower device is rejected and returns the winner.
	stale := draftCall(t, app, "PUT", `{"text":"stale","updatedAt":"50"}`)
	if stale.Data.Applied || stale.Data.Draft.Text != "from desktop" {
		t.Fatalf("stale write must lose: %+v", stale.Data)
	}

	// A newer whole-draft replace drops the comment, so deletes propagate.
	newer := draftCall(t, app, "PUT", `{"text":"from phone","updatedAt":"200"}`)
	if !newer.Data.Applied {
		t.Fatal("newer write should apply")
	}
	got := draftCall(t, app, "GET", "").Data.Draft
	if got == nil || got.Text != "from phone" || len(got.Comments) != 0 {
		t.Fatalf("unexpected stored draft: %+v", got)
	}

	// Clearing keeps an empty draft as a tombstone, so a device that still
	// holds the old text cannot push it back over the clear.
	draftCall(t, app, "PUT", `{"text":"","updatedAt":"300"}`)
	cleared := draftCall(t, app, "GET", "").Data.Draft
	if cleared == nil || cleared.Text != "" || cleared.UpdatedAt != "300" {
		t.Fatalf("cleared draft should be an empty tombstone: %+v", cleared)
	}
	late := draftCall(t, app, "PUT", `{"text":"from phone","updatedAt":"250"}`)
	if late.Data.Applied {
		t.Fatal("a write older than the clear must lose")
	}
}
