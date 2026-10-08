package tests

import (
	"testing"
	"time"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/services/mcp"
)

func TestAutoConnectOnlyServersWithSavedCredential(t *testing.T) {
	ts := startHTTPMCP(t, requireBearer("Bearer s3cret"))
	m := newManager(t)
	saveStatic(t, m, ts.URL, "Bearer s3cret")

	// An OAuth server with no stored token must stay disconnected (no browser).
	if _, err := m.Config.Save(mcp.ServerConfig{
		ID: "needslogin", Label: "Needs login", Transport: mcp.TransportHTTP, URL: ts.URL, Enabled: true,
		Auth: &mcp.AuthConfig{Type: mcp.AuthOAuth2, TokenRef: "needslogin"},
	}); err != nil {
		t.Fatal(err)
	}

	started := m.AutoConnect()
	if len(started) != 1 || started[0] != "echo" {
		t.Fatalf("started: %v", started)
	}
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		st, _ := m.Status()
		if st[0].ID == "echo" && st[0].Status == mcp.StatusConnected {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	st, _ := m.Status()
	for _, row := range st {
		switch row.ID {
		case "echo":
			if row.Status != mcp.StatusConnected {
				t.Fatalf("echo: %+v", row)
			}
		case "needslogin":
			if row.Status != mcp.StatusDisconnected {
				t.Fatalf("needslogin: %+v", row)
			}
		}
	}
}
