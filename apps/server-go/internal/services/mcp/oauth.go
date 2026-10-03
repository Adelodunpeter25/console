// OAuth 2.1 for HTTP MCP servers: discovery, dynamic client registration and
// PKCE are done by the SDK's AuthorizationCodeHandler; this file supplies the
// loopback redirect listener, browser opening and token persistence.
package mcp

import (
	"context"
	"errors"
	"fmt"
	"html"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/modelcontextprotocol/go-sdk/auth"
	"github.com/modelcontextprotocol/go-sdk/oauthex"
	"golang.org/x/oauth2"
)

const (
	callbackPath = "/callback"
	// authTimeout is how long we wait for the user to finish in the browser.
	authTimeout = 5 * time.Minute
)

type callbackResult struct {
	code, state, iss, errMsg string
}

type waiterEntry struct {
	ch   chan callbackResult
	done bool
}

// OAuthCoordinator owns the loopback callback listener shared by all servers.
type OAuthCoordinator struct {
	// OpenBrowser opens the authorization URL; defaults to OpenBrowser.
	OpenBrowser func(string) error

	mu      sync.Mutex
	ln      net.Listener
	srv     *http.Server
	waiters map[string]*waiterEntry
}

func NewOAuthCoordinator() *OAuthCoordinator {
	return &OAuthCoordinator{OpenBrowser: OpenBrowser, waiters: map[string]*waiterEntry{}}
}

// redirectURI lazily starts the loopback listener and returns its callback URL.
func (o *OAuthCoordinator) redirectURI() (string, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.ln == nil {
		ln, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			return "", fmt.Errorf("start oauth callback listener: %w", err)
		}
		mux := http.NewServeMux()
		mux.HandleFunc(callbackPath, o.handleCallback)
		o.ln = ln
		o.srv = &http.Server{Handler: mux, ReadHeaderTimeout: 10 * time.Second}
		go func() { _ = o.srv.Serve(ln) }()
	}
	return "http://" + o.ln.Addr().String() + callbackPath, nil
}

func (o *OAuthCoordinator) handleCallback(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	state := q.Get("state")
	o.mu.Lock()
	entry, ok := o.waiters[state]
	o.mu.Unlock()
	if !ok || entry.done {
		http.Error(w, "Unknown or expired authorization request.", http.StatusBadRequest)
		return
	}
	res := callbackResult{code: q.Get("code"), state: state, iss: q.Get("iss")}
	if e := q.Get("error"); e != "" {
		res.errMsg = e
		if d := q.Get("error_description"); d != "" {
			res.errMsg += ": " + d
		}
	} else if res.code == "" {
		res.errMsg = "authorization response had no code"
	}
	o.mu.Lock()
	entry.done = true
	o.mu.Unlock()
	select {
	case entry.ch <- res:
	default:
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if res.errMsg != "" {
		fmt.Fprintf(w, "<html><body style=\"font-family:sans-serif\"><h3>Sign-in failed</h3><p>%s</p></body></html>", html.EscapeString(res.errMsg))
		return
	}
	fmt.Fprint(w, "<html><body style=\"font-family:sans-serif\"><h3>Signed in</h3><p>You can close this tab and return to Console.</p></body></html>")
}

// ResolveWaiter delivers an out-of-band callback result (mobile client
// loopback) to the pending server-initiated flow, exactly as the
// server-side loopback listener would. State keys the waiter; a terminal
// error completes the flow with failure instead of waiting out the timeout.
func (o *OAuthCoordinator) ResolveWaiter(state, code, errMsg, iss string) bool {
	o.mu.Lock()
	entry, ok := o.waiters[state]
	fresh := ok && entry != nil && !entry.done
	if fresh {
		entry.done = true
	}
	o.mu.Unlock()
	if !fresh {
		return false
	}
	select {
	case entry.ch <- callbackResult{code: code, state: state, errMsg: errMsg, iss: iss}:
	default:
	}
	return true
}

// Close stops the callback listener.
func (o *OAuthCoordinator) Close() {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.srv != nil {
		_ = o.srv.Close()
		o.srv, o.ln = nil, nil
	}
}

// fetcher waits for the OAuth callback for one connect attempt, reporting
// the pending URL through onURL. The server box auto-opens the browser
// only when the redirect comes back to it (server-owned loopback); for a
// client-supplied redirect the client opens the URL itself and forwards the
// result via ResolveWaiter.
func (o *OAuthCoordinator) fetcher(onURL func(string)) auth.AuthorizationCodeFetcher {
	return o.fetcherForRedirect(onURL, "")
}

func (o *OAuthCoordinator) fetcherForRedirect(onURL func(string), redirectURL string) auth.AuthorizationCodeFetcher {
	return func(ctx context.Context, args *auth.AuthorizationArgs) (*auth.AuthorizationResult, error) {
		u, err := url.Parse(args.URL)
		if err != nil {
			return nil, err
		}
		state := u.Query().Get("state")
		ch := make(chan callbackResult, 1)
		o.mu.Lock()
		o.waiters[state] = &waiterEntry{ch: ch}
		o.mu.Unlock()
		defer func() {
			o.mu.Lock()
			delete(o.waiters, state)
			o.mu.Unlock()
		}()

		if onURL != nil {
			onURL(args.URL)
		}
		if redirectURL == "" {
			if err := o.OpenBrowser(args.URL); err != nil {
				// Not fatal: the URL is exposed through the server status so the
				// user can open it by hand.
				slog.Warn("mcp oauth: could not open browser", "error", err)
			}
		}
		select {
		case res := <-ch:
			if res.errMsg != "" {
				return nil, errors.New("authorization failed: " + res.errMsg)
			}
			return &auth.AuthorizationResult{Code: res.code, State: res.state, Iss: res.iss}, nil
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(authTimeout):
			return nil, errors.New("timed out waiting for browser sign-in")
		}
	}
}

// NewHandler builds the SDK OAuth handler for one server. Tokens are loaded
// from and saved to the credential store; onURL reports the pending
// authorization URL. redirectURL overrides the callback origin (mobile
// loopback); empty keeps the server-owned loopback.
func (o *OAuthCoordinator) NewHandler(cfg ServerConfig, creds *CredentialStore, onURL func(string), redirectURL string) (auth.OAuthHandler, error) {
	redirect := redirectURL
	if redirect == "" {
		var err error
		redirect, err = o.redirectURI()
		if err != nil {
			return nil, err
		}
	}
	ref := cfg.Auth.TokenRef
	hc := auth.AuthorizationCodeHandlerConfig{
		DynamicClientRegistrationConfig: &auth.DynamicClientRegistrationConfig{
			Metadata: &oauthex.ClientRegistrationMetadata{
				RedirectURIs:            []string{redirect},
				TokenEndpointAuthMethod: "none",
				GrantTypes:              []string{"authorization_code", "refresh_token"},
				ResponseTypes:           []string{"code"},
				ClientName:              "Console",
			},
		},
		RedirectURL:              redirect,
		RequestRefreshToken:      true,
		AuthorizationCodeFetcher: o.fetcherForRedirect(onURL, redirectURL),
		NewTokenSource: func(ctx context.Context, c *oauth2.Config, tok *oauth2.Token) (oauth2.TokenSource, error) {
			persist := &persistingSource{inner: c.TokenSource(ctx, tok), ref: ref, creds: creds,
				clientID: c.ClientID, clientSecret: c.ClientSecret, tokenURL: c.Endpoint.TokenURL}
			persist.save(tok)
			return persist, nil
		},
	}
	if stored, ok, err := creds.Get(ref); err == nil && ok && stored.Kind == CredentialOAuth && stored.TokenURL != "" &&
		(stored.AccessToken != "" || stored.RefreshToken != "") {
		oc := &oauth2.Config{ClientID: stored.ClientID, ClientSecret: stored.ClientSecret,
			Endpoint: oauth2.Endpoint{TokenURL: stored.TokenURL, AuthStyle: oauth2.AuthStyleAutoDetect}}
		tok := &oauth2.Token{AccessToken: stored.AccessToken, RefreshToken: stored.RefreshToken, TokenType: "Bearer"}
		if stored.ExpiresAt > 0 {
			tok.Expiry = time.Unix(stored.ExpiresAt, 0)
		}
		hc.InitialTokenSource = &persistingSource{inner: oc.TokenSource(context.Background(), tok), ref: ref, creds: creds,
			clientID: stored.ClientID, clientSecret: stored.ClientSecret, tokenURL: stored.TokenURL, last: stored.AccessToken}
	}
	return auth.NewAuthorizationCodeHandler(&hc)
}

// persistingSource writes refreshed tokens back to the credential store. A
// failed refresh yields (nil, nil) so the transport sends no header, gets a
// 401 and falls into the interactive flow instead of failing outright.
type persistingSource struct {
	inner                                 oauth2.TokenSource
	ref, clientID, clientSecret, tokenURL string
	creds                                 *CredentialStore
	mu                                    sync.Mutex
	last                                  string
}

func (p *persistingSource) Token() (*oauth2.Token, error) {
	tok, err := p.inner.Token()
	if err != nil {
		slog.Debug("mcp oauth: token unavailable, will re-authorize", "error", err)
		return nil, nil
	}
	p.save(tok)
	return tok, nil
}

func (p *persistingSource) save(tok *oauth2.Token) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if tok == nil || tok.AccessToken == p.last {
		return
	}
	p.last = tok.AccessToken
	c := Credential{Kind: CredentialOAuth, ClientID: p.clientID, ClientSecret: p.clientSecret, TokenURL: p.tokenURL,
		AccessToken: tok.AccessToken, RefreshToken: tok.RefreshToken}
	if !tok.Expiry.IsZero() {
		c.ExpiresAt = tok.Expiry.Unix()
	}
	if err := p.creds.Set(p.ref, c); err != nil {
		slog.Warn("mcp oauth: could not persist token", "error", err)
	}
}
