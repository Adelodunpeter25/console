// Codex OAuth login flow state. Port of the codex branch in
// apps/server/api/src/services/auth.service.ts: pending PKCE verifiers
// keyed by state token (10-minute TTL), login URL generation, and the
// code-exchange callback. Other providers return "not supported" until
// their Go ports land.
//
// This lives outside services because the provider layer (codex → loop →
// services) must not be imported back by services.
package auth

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sync"
	"time"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/providers/codex"
)

// pendingTTL mirrors the TS 10-minute OAuth state expiry.
const pendingTTL = 10 * time.Minute

type pendingLogin struct {
	verifier    string
	redirectURI string
	expiresAt   time.Time
}

// CodexAuthStatus mirrors ProviderAuthStatus for the codex key.
type CodexAuthStatus struct {
	LoggedIn bool   `json:"loggedIn"`
	Email    string `json:"email,omitempty"`
}

// AuthStatus mirrors AuthStatusResponse. Non-codex providers report
// loggedIn:false until their Go ports land.
type AuthStatus struct {
	Antigravity CodexAuthStatus `json:"antigravity"`
	Codex       CodexAuthStatus `json:"codex"`
	Devin       CodexAuthStatus `json:"devin"`
	Claude      CodexAuthStatus `json:"claude"`
}

// LoginURL is the response for POST /api/auth/login/url.
type LoginURL struct {
	Provider    string `json:"provider"`
	AuthURL     string `json:"authUrl"`
	State       string `json:"state"`
	RedirectURI string `json:"redirectUri"`
}

// CallbackResult is the response for POST /api/auth/login/callback.
type CallbackResult struct {
	Provider  string `json:"provider"`
	UserEmail string `json:"userEmail,omitempty"`
}

// AuthService holds OAuth login state. The zero value is usable except
// for Exchange, which defaults to codex.ExchangeCode when nil (overridable
// in tests).
type AuthService struct {
	mu      sync.Mutex
	pending map[string]pendingLogin
	// Exchange swaps an authorization code for credentials.
	Exchange func(code, verifier, redirectURI string) (codex.OAuthCredential, error)
}

func NewAuthService() *AuthService {
	return &AuthService{pending: map[string]pendingLogin{}}
}

func (s *AuthService) exchange(code, verifier, redirectURI string) (codex.OAuthCredential, error) {
	if s.Exchange != nil {
		return s.Exchange(code, verifier, redirectURI)
	}
	return codex.ExchangeCode(nil, code, verifier, redirectURI)
}

// GetStatus reports Codex login state from the credential file/env token.
func (s *AuthService) GetStatus() AuthStatus {
	status := AuthStatus{}
	if cred, err := codex.LoadCredential(); err == nil && cred.AccessToken != "" {
		status.Codex = CodexAuthStatus{LoggedIn: true, Email: cred.Email}
	} else if codex.CredentialExists() {
		status.Codex = CodexAuthStatus{LoggedIn: true}
	}
	return status
}

// GetLoginURL starts a Codex PKCE login, storing the verifier by state.
func (s *AuthService) GetLoginURL() (LoginURL, error) {
	state, err := newStateToken()
	if err != nil {
		return LoginURL{}, err
	}
	verifier, challenge, err := codex.GeneratePKCE()
	if err != nil {
		return LoginURL{}, err
	}
	authURL, redirectURI := codex.AuthorizationURL(state, challenge)
	s.mu.Lock()
	if s.pending == nil {
		s.pending = map[string]pendingLogin{}
	}
	s.sweepLocked()
	s.pending[state] = pendingLogin{verifier: verifier, redirectURI: redirectURI, expiresAt: time.Now().Add(pendingTTL)}
	s.mu.Unlock()
	return LoginURL{Provider: "codex", AuthURL: authURL, State: state, RedirectURI: redirectURI}, nil
}

// HandleCallback exchanges the authorization code, consuming the pending
// state (single-use, like the TS service).
func (s *AuthService) HandleCallback(code, state string) (CallbackResult, error) {
	if state == "" {
		return CallbackResult{}, fmt.Errorf("Codex OAuth callback is missing state.")
	}
	s.mu.Lock()
	s.sweepLocked()
	pending, ok := s.pending[state]
	delete(s.pending, state)
	s.mu.Unlock()
	if !ok {
		return CallbackResult{}, fmt.Errorf("Codex OAuth state is invalid or expired.")
	}
	credential, err := s.exchange(code, pending.verifier, pending.redirectURI)
	if err != nil {
		return CallbackResult{}, err
	}
	if err := codex.SaveCredential(credential); err != nil {
		return CallbackResult{}, err
	}
	return CallbackResult{Provider: "codex", UserEmail: credential.Email}, nil
}

// sweepLocked drops expired states. Caller must hold s.mu.
func (s *AuthService) sweepLocked() {
	now := time.Now()
	for state, p := range s.pending {
		if !p.expiresAt.After(now) {
			delete(s.pending, state)
		}
	}
}

func newStateToken() (string, error) {
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
