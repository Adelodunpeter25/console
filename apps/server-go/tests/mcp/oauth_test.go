// OAuth 2.1 flow against a fake authorization server: discovery, dynamic
// client registration, PKCE, loopback redirect, persisted tokens, reuse.
package tests

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/auth"
	"github.com/modelcontextprotocol/go-sdk/oauthex"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/services/mcp"
)

// fakeAuthServer is one httptest server acting as both the MCP resource and
// its authorization server.
type fakeAuthServer struct {
	ts        *httptest.Server
	mu        sync.Mutex
	challenge string
	code      string
	registers atomic.Int32
	exchanges atomic.Int32
	refreshes atomic.Int32
	// tokenTTL controls expires_in so refresh can be exercised.
	tokenTTL int
}

func newFakeAuthServer(t *testing.T) *fakeAuthServer {
	f := &fakeAuthServer{tokenTTL: 3600}
	mux := http.NewServeMux()
	base := func() string { return f.ts.URL }

	mux.HandleFunc("/.well-known/oauth-protected-resource", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"resource": base(), "authorization_servers": []string{base()},
		})
	})
	mux.HandleFunc("/.well-known/oauth-authorization-server", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"issuer": base(), "authorization_endpoint": base() + "/authorize",
			"token_endpoint": base() + "/token", "registration_endpoint": base() + "/register",
			"code_challenge_methods_supported":      []string{"S256"},
			"grant_types_supported":                 []string{"authorization_code", "refresh_token"},
			"token_endpoint_auth_methods_supported": []string{"none"},
		})
	})
	mux.HandleFunc("/register", func(w http.ResponseWriter, r *http.Request) {
		f.registers.Add(1)
		var meta oauthex.ClientRegistrationMetadata
		json.NewDecoder(r.Body).Decode(&meta)
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]any{
			"client_id": "client-123", "redirect_uris": meta.RedirectURIs, "token_endpoint_auth_method": "none",
		})
	})
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		switch r.Form.Get("grant_type") {
		case "authorization_code":
			f.exchanges.Add(1)
			f.mu.Lock()
			challenge := f.challenge
			f.mu.Unlock()
			sum := sha256.Sum256([]byte(r.Form.Get("code_verifier")))
			if base64.RawURLEncoding.EncodeToString(sum[:]) != challenge || r.Form.Get("code") != "good-code" {
				http.Error(w, `{"error":"invalid_grant"}`, http.StatusBadRequest)
				return
			}
			json.NewEncoder(w).Encode(map[string]any{
				"access_token": "access-1", "refresh_token": "refresh-1", "token_type": "Bearer", "expires_in": f.tokenTTL,
			})
		case "refresh_token":
			f.refreshes.Add(1)
			if r.Form.Get("refresh_token") != "refresh-1" {
				http.Error(w, `{"error":"invalid_grant"}`, http.StatusBadRequest)
				return
			}
			json.NewEncoder(w).Encode(map[string]any{
				"access_token": "access-2", "refresh_token": "refresh-1", "token_type": "Bearer", "expires_in": 3600,
			})
		}
	})

	mcpHandler := startHTTPMCP(t, nil).Config.Handler // reuse the echo MCP handler
	mux.HandleFunc("/mcp", func(w http.ResponseWriter, r *http.Request) {
		h := r.Header.Get("Authorization")
		if h != "Bearer access-1" && h != "Bearer access-2" {
			w.Header().Set("WWW-Authenticate", `Bearer resource_metadata="`+base()+`/.well-known/oauth-protected-resource"`)
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		mcpHandler.ServeHTTP(w, r)
	})
	f.ts = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/mcp" {
			w.Header().Set("Content-Type", "application/json")
		}
		mux.ServeHTTP(w, r)
	}))
	t.Cleanup(f.ts.Close)
	return f
}

// browserThatApproves plays the user: "opens" the authorization URL by
// recording the PKCE challenge and hitting the redirect URI with a code.
func (f *fakeAuthServer) browserThatApproves(t *testing.T, deny bool) func(string) error {
	return func(authURL string) error {
		u, err := url.Parse(authURL)
		if err != nil {
			return err
		}
		q := u.Query()
		if q.Get("code_challenge_method") != "S256" || q.Get("resource") == "" || q.Get("client_id") != "client-123" {
			t.Errorf("bad authorize params: %v", q)
		}
		f.mu.Lock()
		f.challenge = q.Get("code_challenge")
		f.mu.Unlock()
		cb, _ := url.Parse(q.Get("redirect_uri"))
		cq := cb.Query()
		cq.Set("state", q.Get("state"))
		if deny {
			cq.Set("error", "access_denied")
		} else {
			cq.Set("code", "good-code")
		}
		cb.RawQuery = cq.Encode()
		go http.Get(cb.String())
		return nil
	}
}

func oauthManager(t *testing.T, f *fakeAuthServer, open func(string) error) *mcp.Manager {
	dir := t.TempDir()
	oc := mcp.NewOAuthCoordinator()
	oc.OpenBrowser = open
	m := mcp.NewManager(mcp.NewConfigStore(dir), mcp.NewCredentialStore(dir), oc)
	t.Cleanup(m.CloseAll)
	if _, err := m.Config.Save(mcp.ServerConfig{
		ID: "atl", Label: "Atlassian", Transport: mcp.TransportHTTP, URL: f.ts.URL + "/mcp", Enabled: true,
		Auth: &mcp.AuthConfig{Type: mcp.AuthOAuth2, TokenRef: "atl"},
	}); err != nil {
		t.Fatal(err)
	}
	return m
}

var _ = auth.AuthorizationResult{}
var _ = time.Second

func TestOAuthFullFlowOpensBrowserAndPersistsTokens(t *testing.T) {
	f := newFakeAuthServer(t)
	opened := make(chan string, 1)
	approve := f.browserThatApproves(t, false)
	m := oauthManager(t, f, func(u string) error { opened <- u; return approve(u) })

	if err := m.Ensure(ctx5(t), "atl"); err != nil {
		t.Fatalf("ensure: %v", err)
	}
	select {
	case <-opened:
	default:
		t.Fatal("browser was never opened")
	}
	if f.registers.Load() != 1 || f.exchanges.Load() != 1 {
		t.Fatalf("registers=%d exchanges=%d", f.registers.Load(), f.exchanges.Load())
	}
	if len(m.Tools("atl")) != 2 {
		t.Fatalf("tools: %+v", m.Tools("atl"))
	}
	cred, ok, _ := m.Credentials.Get("atl")
	if !ok || cred.AccessToken != "access-1" || cred.RefreshToken != "refresh-1" || cred.ClientID != "client-123" || cred.TokenURL == "" {
		t.Fatalf("stored credential: %+v", cred)
	}
}

func TestOAuthStoredTokenSkipsBrowserAfterRestart(t *testing.T) {
	f := newFakeAuthServer(t)
	dir := t.TempDir()
	newMgr := func(open func(string) error) *mcp.Manager {
		oc := mcp.NewOAuthCoordinator()
		oc.OpenBrowser = open
		m := mcp.NewManager(mcp.NewConfigStore(dir), mcp.NewCredentialStore(dir), oc)
		t.Cleanup(m.CloseAll)
		return m
	}
	m1 := newMgr(f.browserThatApproves(t, false))
	if _, err := m1.Config.Save(mcp.ServerConfig{ID: "atl", Label: "Atlassian", Transport: mcp.TransportHTTP,
		URL: f.ts.URL + "/mcp", Enabled: true, Auth: &mcp.AuthConfig{Type: mcp.AuthOAuth2, TokenRef: "atl"}}); err != nil {
		t.Fatal(err)
	}
	if err := m1.Ensure(ctx5(t), "atl"); err != nil {
		t.Fatal(err)
	}
	m1.CloseAll()

	// "Restart": fresh manager, same storage; any browser open is a failure.
	m2 := newMgr(func(string) error { t.Error("browser must not open when a valid token is stored"); return nil })
	if err := m2.Ensure(ctx5(t), "atl"); err != nil {
		t.Fatalf("reconnect with stored token: %v", err)
	}
	if f.registers.Load() != 1 {
		t.Fatalf("should not re-register, registers=%d", f.registers.Load())
	}
}

func TestOAuthExpiredTokenRefreshesWithoutBrowser(t *testing.T) {
	f := newFakeAuthServer(t)
	dir := t.TempDir()
	m := mcp.NewManager(mcp.NewConfigStore(dir), mcp.NewCredentialStore(dir), func() *mcp.OAuthCoordinator {
		oc := mcp.NewOAuthCoordinator()
		oc.OpenBrowser = func(string) error { t.Error("browser must not open on refresh"); return nil }
		return oc
	}())
	t.Cleanup(m.CloseAll)
	m.Config.Save(mcp.ServerConfig{ID: "atl", Label: "A", Transport: mcp.TransportHTTP, URL: f.ts.URL + "/mcp",
		Enabled: true, Auth: &mcp.AuthConfig{Type: mcp.AuthOAuth2, TokenRef: "atl"}})
	// Expired access token + valid refresh token, as left by a previous session.
	m.Credentials.Set("atl", mcp.Credential{Kind: mcp.CredentialOAuth, ClientID: "client-123",
		AccessToken: "stale", RefreshToken: "refresh-1", ExpiresAt: time.Now().Add(-time.Hour).Unix(),
		TokenURL: f.ts.URL + "/token"})

	if err := m.Ensure(ctx5(t), "atl"); err != nil {
		t.Fatalf("ensure: %v", err)
	}
	if f.refreshes.Load() == 0 {
		t.Fatal("expected a refresh_token grant")
	}
	cred, _, _ := m.Credentials.Get("atl")
	if cred.AccessToken != "access-2" {
		t.Fatalf("refreshed token not persisted: %+v", cred)
	}
}

func TestOAuthDeniedByUserFailsCleanly(t *testing.T) {
	f := newFakeAuthServer(t)
	m := oauthManager(t, f, f.browserThatApproves(t, true))
	if err := m.Ensure(ctx5(t), "atl"); err == nil {
		t.Fatal("denied sign-in should fail")
	}
	st, _ := m.Status()
	if st[0].Status != mcp.StatusError {
		t.Fatalf("status: %+v", st[0])
	}
}

func TestOAuthStatusExposesAuthURLWhilePending(t *testing.T) {
	f := newFakeAuthServer(t)
	release := make(chan struct{})
	approve := f.browserThatApproves(t, false)
	m := oauthManager(t, f, func(u string) error {
		go func() { <-release; approve(u) }()
		return nil
	})
	if err := m.Connect("atl"); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		st, _ := m.Status()
		if st[0].Status == mcp.StatusNeedsAuth && st[0].AuthURL != "" {
			close(release)
			if err := m.Ensure(ctx5(t), "atl"); err != nil {
				t.Fatal(err)
			}
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("never reached needs_auth with an authUrl")
}

func TestResetAuthForgetsCredential(t *testing.T) {
	f := newFakeAuthServer(t)
	m := oauthManager(t, f, f.browserThatApproves(t, false))
	if err := m.Ensure(ctx5(t), "atl"); err != nil {
		t.Fatal(err)
	}
	if err := m.ResetAuth("atl"); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := m.Credentials.Get("atl"); ok {
		t.Fatal("credential should be gone")
	}
}
