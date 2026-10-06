// Title normalization at the store boundary. Desktop and mobile both render
// the stored title verbatim, so the capitalization rule has to hold for every
// write path: create (client-supplied), LLM generation (via titles), and
// manual rename.
package tests

import (
	"testing"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/db"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/services"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/types"
)

func newTitleStore(t *testing.T) *services.SessionService {
	t.Helper()
	manager, err := db.Open(db.OpenOptions{Path: ":memory:"})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(manager.Close)
	return services.NewSessionService(manager)
}

func TestCreateCapitalizesTitle(t *testing.T) {
	sessions := newTitleStore(t)

	cases := []struct{ in, want string }{
		{"fix login bug", "Fix login bug"},
		{"  spaced out  ", "Spaced out"},
		{"ALL CAPS", "ALL CAPS"},
		{"🔥 fix login", "🔥 Fix login"},
		{"élan vital", "Élan vital"},
		{"v2 api work", "V2 api work"},
		{"", "New Session"},
		{"   ", "New Session"},
	}
	for _, c := range cases {
		header, err := sessions.Create(types.CreateSessionOptions{
			Title: c.in, Cwd: t.TempDir(), ModelID: "m", Provider: "p",
		})
		if err != nil {
			t.Fatalf("create %q: %v", c.in, err)
		}
		if header.Title != c.want {
			t.Errorf("Create(%q).Title = %q; want %q", c.in, header.Title, c.want)
		}
	}
}

// A rename is normalized too, and the change reaches both the index row and
// the per-session meta row.
func TestRenameCapitalizesTitle(t *testing.T) {
	sessions := newTitleStore(t)

	header, err := sessions.Create(types.CreateSessionOptions{
		Title: "original", Cwd: t.TempDir(), ModelID: "m", Provider: "p",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := sessions.UpdateTitle(header.ID, "  rename me later  "); err != nil {
		t.Fatalf("update title: %v", err)
	}

	after, err := sessions.Header(header.ID)
	if err != nil {
		t.Fatalf("header: %v", err)
	}
	if after.Title != "Rename me later" {
		t.Fatalf("index title = %q; want %q", after.Title, "Rename me later")
	}

	loaded, err := sessions.Load(header.ID, 0, 0)
	if err != nil || loaded == nil {
		t.Fatalf("load: %v", err)
	}
	if loaded.Header.Title != "Rename me later" {
		t.Fatalf("meta title = %q; want %q", loaded.Header.Title, "Rename me later")
	}
}
