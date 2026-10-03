// Client-supplied redirect flow: Connect takes a mobile callback URL, the
// auth URL embeds it, and the forwarded code+state from the mobile loopback
// completes the same pending flow the server listener would.
package tests

import (
	"errors"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/routes"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/services/mcp"
)

func TestOAuthClientRedirectForwardsCallback(t *testing.T) {
	f := newFakeAuthServer(t)
	var authURL string
	m := oauthManager(t, f, func(u string) error {
		// Dead: with a client redirect the browser must NOT be opened on
		// the server — the URL only travels to the client via status.
		authURL = u
		return errors.New("browser open unsupported")
	})

	mobileRedirect := "http://127.0.0.1:9999/callback"
	if err := m.Connect("atl", mobileRedirect); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	var u *url.URL
	for time.Now().Before(deadline) {
		st, _ := m.Status()
		if st[0].Status == mcp.StatusNeedsAuth && st[0].AuthURL != "" {
			var err error
			u, err = url.Parse(st[0].AuthURL)
			if err != nil {
				t.Fatal(err)
			}
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if u == nil {
		t.Fatal("never reached needs_auth")
	}
	if got := u.Query().Get("redirect_uri"); got != mobileRedirect {
		t.Fatalf("redirect_uri = %q; want %q", got, mobileRedirect)
	}
	if authURL != "" {
		t.Fatalf("server opened browser with %q; client redirect must not", authURL)
	}

	// The fake provider sends the user "to" the mobile redirect; the mobile
	// app forwards the result to the server instead. The provider learns
	// the PKCE challenge from the authorize URL itself.
	state := u.Query().Get("state")
	cb := u.Query().Get("redirect_uri")
	if cb == "" || state == "" {
		t.Fatalf("params: %v", u.Query())
	}
	f.mu.Lock()
	f.challenge = u.Query().Get("code_challenge")
	f.mu.Unlock()
	ok := m.OAuth.ResolveWaiter(state, "good-code", "", "")
	if !ok {
		t.Fatal("resolve waiter rejected forwarded state")
	}
	if err := m.Ensure(ctx5(t), "atl"); err != nil {
		t.Fatal(err)
	}
	if f.exchanges.Load() != 1 {
		t.Fatalf("exchanges = %d; want 1", f.exchanges.Load())
	}
	// Stale state must conflict.
	if m.OAuth.ResolveWaiter(state, "good-code", "", "") {
		t.Fatal("stale state re-resolved")
	}
}

func TestOAuthResolveWaiterErrorCompletesFlow(t *testing.T) {
	f := newFakeAuthServer(t)
	m := oauthManager(t, f, func(u string) error { return errors.New("no browser") })
	if err := m.Connect("atl"); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	var state string
	for time.Now().Before(deadline) {
		st, _ := m.Status()
		if st[0].Status == mcp.StatusNeedsAuth && st[0].AuthURL != "" {
			u, _ := url.Parse(st[0].AuthURL)
			state = u.Query().Get("state")
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if state == "" {
		t.Fatal("never reached needs_auth")
	}
	if !m.OAuth.ResolveWaiter(state, "", "access_denied", "") {
		t.Fatal("resolve waiter rejected")
	}
	if err := m.Ensure(ctx5(t), "atl"); err == nil {
		t.Fatal("denied callback must fail the flow")
	}
}

func TestMCPConnectRouteValidatesRedirectURI(t *testing.T) {
	m := newManager(t)
	app := fiber.New()
	routes.RegisterMCPRoutes(app, m)

	code, _ := do(t, app, "POST", "/api/mcp/servers/x/connect", `{"redirectUri":"http://evil.example/cb"}`)
	if code != http.StatusBadRequest {
		t.Fatalf("status = %d; want 400", code)
	}
}
