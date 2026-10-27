// Codex OAuth login flow state: pending PKCE verifiers keyed by state
// token (10-minute TTL), login URL generation, and the code-exchange
// callback. Other providers return "not supported" until
// their Go ports land.
//
// This lives outside services because the provider layer (codex → loop →
// services) must not be imported back by services.
package auth

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/accounts"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/providers/antigravity"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/providers/claude"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/providers/codex"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/providers/github"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/providers/usage"
)

// pendingTTL is the 10-minute OAuth state expiry.
const pendingTTL = 10 * time.Minute

// ErrUnknownProvider rejects a provider id outside codex|claude|antigravity.
var ErrUnknownProvider = errors.New("Invalid OAuth provider.")

// ErrEnvAccount rejects mutations of the synthetic env account. It is
// locked while the environment variable owns the provider; nothing this
// API does can change which credential runs would use.
var ErrEnvAccount = errors.New("The environment-token account cannot be removed or deselected while the env variable is set.")

type pendingLogin struct {
	verifier    string
	redirectURI string
	expiresAt   time.Time
}

// AccountStatus is one stored login as reported in the status API. Secret
// material stays on the server (see ProviderAccount in the proto for the
// wire contract).
type AccountStatus struct {
	ID        string `json:"id"`
	Email     string `json:"email,omitempty"`
	Label     string `json:"label,omitempty"`
	Active    bool   `json:"active"`
	Env       bool   `json:"env,omitempty"`
	CreatedAt int64  `json:"createdAt"`
}

// ProviderAuthStatus is the per-provider login state. loggedIn/email
// describe the ACTIVE credential exactly as they described the sole
// credential before accounts; Accounts carries every stored login (nil
// when the account store is unavailable, so a degraded boot still reports
// old-style status).
type ProviderAuthStatus struct {
	LoggedIn bool            `json:"loggedIn"`
	Email    string          `json:"email,omitempty"`
	Accounts []AccountStatus `json:"accounts,omitempty"`
}

// GitHubAuthStatus reports GitHub git-credential state. Unlike the LLM
// providers this is not a session: it exists so `git` can reach private
// repositories. Username and Scopes are the values cached in the credential
// file when the token was accepted, so reading this never touches the
// network and a GitHub outage cannot stall GET /api/auth/status.
//
// A revoked token still reports Connected until the next re-login or
// disconnect; the symptom is a git auth failure, not a status change.
type GitHubAuthStatus struct {
	Connected bool     `json:"connected"`
	Username  string   `json:"username,omitempty"`
	Scopes    []string `json:"scopes,omitempty"`
}

// AuthStatus mirrors AuthStatusResponse. Non-codex providers report
// loggedIn:false until their Go ports land.
type AuthStatus struct {
	Antigravity ProviderAuthStatus `json:"antigravity"`
	Codex       ProviderAuthStatus `json:"codex"`
	Devin       ProviderAuthStatus `json:"devin"`
	Claude      ProviderAuthStatus `json:"claude"`
	GitHub      GitHubAuthStatus   `json:"github"`
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
	// AccountID is the (existing or newly registered) account the login
	// landed in; AccountActive tells whether it now serves runs. A second
	// account never activates automatically — the client highlights the
	// new row as needing an explicit "Use".
	AccountID     string `json:"accountId,omitempty"`
	AccountActive bool   `json:"accountActive"`
}

// AuthService holds OAuth login state. The zero value is usable except
// for Exchange/ExchangeClaude, which default to the provider exchangers
// when nil (overridable in tests). Usage, when set, has its per-provider
// cache invalidated on account mutations.
type AuthService struct {
	mu      sync.Mutex
	pending map[string]pendingLogin
	// Exchange swaps an authorization code for Codex credentials.
	Exchange func(code, verifier, redirectURI string) (codex.OAuthCredential, error)
	// ExchangeClaude swaps an authorization code for Claude credentials.
	ExchangeClaude func(code, state, verifier, redirectURI string) (claude.OAuthCredential, error)
	// ValidatePAT verifies a pasted GitHub token (overridable in tests).
	ValidatePAT func(ctx context.Context, client *http.Client, token string) (string, []string, error)
	// Usage is the aggregated usage cache; nil skips its invalidation.
	Usage *usage.Service
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

func (s *AuthService) exchangeClaude(code, state, verifier, redirectURI string) (claude.OAuthCredential, error) {
	if s.ExchangeClaude != nil {
		return s.ExchangeClaude(code, state, verifier, redirectURI)
	}
	return claude.ExchangeCode(nil, code, state, verifier, redirectURI)
}

// GetStatus reports login state from credential files/env tokens, plus
// every provider's stored account list.
func (s *AuthService) GetStatus() AuthStatus {
	status := AuthStatus{
		Antigravity: s.providerStatus("antigravity"),
		Codex:       s.providerStatus("codex"),
		Claude:      s.providerStatus("claude"),
		GitHub:      GitHubStatus(),
	}
	return status
}

// providerStatus builds one provider's login state: the active credential
// (env token, active account file, or legacy file — all resolved inside
// LoadCredential) and the account list.
func (s *AuthService) providerStatus(provider string) ProviderAuthStatus {
	out := ProviderAuthStatus{}
	switch provider {
	case "codex":
		if cred, err := codex.LoadCredential(); err == nil && cred.AccessToken != "" {
			out = ProviderAuthStatus{LoggedIn: true, Email: cred.Email}
		} else if codex.CredentialExists() {
			out = ProviderAuthStatus{LoggedIn: true}
		}
	case "claude":
		if cred, err := claude.LoadCredential(); err == nil && cred.AccessToken != "" {
			out = ProviderAuthStatus{LoggedIn: true, Email: cred.Email}
		} else if claude.CredentialExists() {
			out = ProviderAuthStatus{LoggedIn: true}
		}
	case "antigravity":
		if cred, err := antigravity.LoadCredential(); err == nil && cred.AccessToken != "" {
			out = ProviderAuthStatus{LoggedIn: true, Email: cred.Email}
		} else if antigravity.CredentialExists() {
			out = ProviderAuthStatus{LoggedIn: true}
		}
	}
	all, err := accounts.All()
	if err != nil {
		// Account store unavailable (unconfigured tooling, degraded boot):
		// report old-style status rather than failing the whole read.
		return out
	}
	out.Accounts = s.accountList(provider, all[provider])
	return out
}

// accountList merges the synthetic env account with the stored accounts.
// While an env token owns the provider it keeps precedence at load time,
// so file accounts are reported inactive regardless of their store flag —
// "harmless and present, but not what runs use" (plan §4.4).
func (s *AuthService) accountList(provider string, rows []accounts.Account) []AccountStatus {
	envOwned := envOwned(provider)
	out := []AccountStatus{}
	if envOwned {
		out = append(out, AccountStatus{ID: "env", Active: true, Env: true})
	}
	for _, r := range rows {
		a := AccountStatus{ID: r.ID, Email: r.Email, Label: r.Label, CreatedAt: r.CreatedAt}
		if !envOwned {
			a.Active = r.Active
		}
		out = append(out, a)
	}
	return out
}

// envOwned reports whether an environment token currently owns the
// provider's credential resolution.
func envOwned(provider string) bool {
	switch provider {
	case "claude":
		return claude.EnvToken() != ""
	case "codex":
		return codex.EnvToken() != ""
	default:
		return false
	}
}

// GitHubStatus reports the cached GitHub credential. Deliberately local-only:
// this is called from GetStatus, which every client polls, so it must never
// block on api.github.com.
func GitHubStatus() GitHubAuthStatus {
	cred, err := github.LoadCredential()
	if err != nil || strings.TrimSpace(cred.Token) == "" {
		return GitHubAuthStatus{}
	}
	return GitHubAuthStatus{
		Connected: true,
		Username:  cred.Username,
		Scopes:    cred.Scopes,
	}
}

// ValidateGitHubPAT swaps a pasted personal access token for stored
// credentials. The token is verified against api.github.com/user before it is
// written, so a bad token never replaces a working one. Overridable in tests.
func (s *AuthService) ValidateGitHubPAT(ctx context.Context, token string) (GitHubAuthStatus, error) {
	validate := s.ValidatePAT
	if validate == nil {
		validate = github.ValidatePAT
	}
	login, scopes, err := validate(ctx, nil, token)
	if err != nil {
		return GitHubAuthStatus{}, err
	}
	if err := github.SaveCredential(github.Credential{
		Token:    token,
		Username: login,
		Scopes:   scopes,
	}); err != nil {
		return GitHubAuthStatus{}, err
	}
	return GitHubStatus(), nil
}

// DisconnectGitHub removes the stored GitHub credential. Idempotent.
func (s *AuthService) DisconnectGitHub() error {
	return github.ClearCredential()
}

// GetLoginURL starts a Codex PKCE login, storing the verifier by state.
func (s *AuthService) GetLoginURL() (LoginURL, error) {
	return s.GetLoginURLFor("codex")
}

// GetLoginURLFor starts a PKCE login for a provider.
func (s *AuthService) GetLoginURLFor(provider string) (LoginURL, error) {
	if provider == "antigravity" {
		state, err := newAntigravityStateToken()
		if err != nil {
			return LoginURL{}, err
		}
		authURL, redirectURI := antigravity.AuthorizationURL(state)
		s.mu.Lock()
		if s.pending == nil {
			s.pending = map[string]pendingLogin{}
		}
		s.sweepLocked()
		s.pending[state] = pendingLogin{redirectURI: redirectURI, expiresAt: time.Now().Add(pendingTTL)}
		s.mu.Unlock()
		return LoginURL{Provider: "antigravity", AuthURL: authURL, State: state, RedirectURI: redirectURI}, nil
	}
	state, err := newStateToken()
	if err != nil {
		return LoginURL{}, err
	}
	var authURL, redirectURI string
	switch provider {
	case "claude":
		var verifier, challenge string
		verifier, challenge, err = claude.GeneratePKCE()
		if err != nil {
			return LoginURL{}, err
		}
		authURL, redirectURI = claude.AuthorizationURL(state, challenge)
		s.mu.Lock()
		if s.pending == nil {
			s.pending = map[string]pendingLogin{}
		}
		s.sweepLocked()
		s.pending[state] = pendingLogin{verifier: verifier, redirectURI: redirectURI, expiresAt: time.Now().Add(pendingTTL)}
		s.mu.Unlock()
		return LoginURL{Provider: "claude", AuthURL: authURL, State: state, RedirectURI: redirectURI}, nil
	default:
		var verifier, challenge string
		verifier, challenge, err = codex.GeneratePKCE()
		if err != nil {
			return LoginURL{}, err
		}
		authURL, redirectURI = codex.AuthorizationURL(state, challenge)
		s.mu.Lock()
		if s.pending == nil {
			s.pending = map[string]pendingLogin{}
		}
		s.sweepLocked()
		s.pending[state] = pendingLogin{verifier: verifier, redirectURI: redirectURI, expiresAt: time.Now().Add(pendingTTL)}
		s.mu.Unlock()
		return LoginURL{Provider: "codex", AuthURL: authURL, State: state, RedirectURI: redirectURI}, nil
	}
}

// HandleCallback exchanges the authorization code, consuming the pending
// state (single-use).
func (s *AuthService) HandleCallback(code, state string) (CallbackResult, error) {
	return s.HandleCallbackFor("codex", code, state)
}

// HandleCallbackFor exchanges the authorization code for a provider,
// registering (or updating in place) the account it belongs to and saving
// the credential into that account's file (plan §5.4).
func (s *AuthService) HandleCallbackFor(provider, code, state string) (CallbackResult, error) {
	if state == "" {
		return CallbackResult{}, fmt.Errorf("OAuth callback is missing state.")
	}
	s.mu.Lock()
	s.sweepLocked()
	pending, ok := s.pending[state]
	delete(s.pending, state)
	s.mu.Unlock()
	if !ok {
		return CallbackResult{}, fmt.Errorf("OAuth state is invalid or expired.")
	}
	switch provider {
	case "antigravity":
		configuredProjectID := antigravity.GetConfiguredProjectID()
		credential, err := antigravity.CompleteAuthFlowWithCode(nil, code, configuredProjectID)
		if err != nil {
			return CallbackResult{}, err
		}
		// Identity: email (the project id is per-account but not a stable
		// cross-restart key). No email → a fresh, unmerged account.
		target, err := loginAccount(provider, credential.Email, credential.Email)
		if err != nil {
			return CallbackResult{}, err
		}
		save := func(cred antigravity.OAuthCredential) error {
			return saveLogin(cred, target, antigravity.SaveCredentialFor, antigravity.SaveCredential)
		}
		if err := save(credential); err != nil {
			return CallbackResult{}, err
		}
		return renderLogin(provider, credential.Email, target), nil
	case "claude":
		credential, err := s.exchangeClaude(code, state, pending.verifier, pending.redirectURI)
		if err != nil {
			return CallbackResult{}, err
		}
		// Identity: the bootstrap email.
		target, err := loginAccount(provider, credential.Email, credential.Email)
		if err != nil {
			return CallbackResult{}, err
		}
		if err := saveLogin(credential, target, claude.SaveCredentialFor, claude.SaveCredential); err != nil {
			return CallbackResult{}, err
		}
		return renderLogin(provider, credential.Email, target), nil
	}
	credential, err := s.exchange(code, pending.verifier, pending.redirectURI)
	if err != nil {
		return CallbackResult{}, err
	}
	// Identity: the ChatGPT account id, falling back to email.
	identity := credential.AccountID
	if identity == "" {
		identity = credential.Email
	}
	target, err := loginAccount(provider, identity, credential.Email)
	if err != nil {
		return CallbackResult{}, err
	}
	if err := saveLogin(credential, target, codex.SaveCredentialFor, codex.SaveCredential); err != nil {
		return CallbackResult{}, err
	}
	return renderLogin(provider, credential.Email, target), nil
}

// loginTarget is where a fresh credential must be saved.
type loginTarget struct {
	// Account is the registered account (SaveCredentialFor target).
	Account accounts.Account
	// Legacy means the account store is unavailable: the credential is
	// saved to the legacy single-credential path, exactly the pre-accounts
	// behavior, so a degraded boot — or standalone tooling/tests — can
	// still log in.
	Legacy bool
}

// loginAccount registers (or dedups) the account for a fresh credential
// and returns where to save it, falling back to the legacy path when the
// account store is unavailable.
func loginAccount(provider, identity, email string) (loginTarget, error) {
	account, _, err := accounts.Register(provider, identity, email, "")
	if err == nil {
		return loginTarget{Account: account}, nil
	}
	if errors.Is(err, accounts.ErrNotConfigured) {
		return loginTarget{Legacy: true}, nil
	}
	return loginTarget{}, err
}

// saveLogin routes the credential write to the destination loginAccount chose.
func saveLogin[T any](cred T, target loginTarget, saveFor func(string, T) error, saveLegacy func(T) error) error {
	if target.Legacy {
		return saveLegacy(cred)
	}
	return saveFor(target.Account.ID, cred)
}

// renderLogin renders the callback response for a saved credential. The
// legacy path keeps the old shape: no account id at all.
func renderLogin(provider, email string, target loginTarget) CallbackResult {
	out := CallbackResult{Provider: provider, UserEmail: email}
	if !target.Legacy {
		out.AccountID = target.Account.ID
		out.AccountActive = target.Account.Active
	}
	return out
}

// GetProjectID returns the configured project ID for a provider.
func (s *AuthService) GetProjectID(provider string) (string, error) {
	if provider != "antigravity" {
		return "", fmt.Errorf("Invalid provider.")
	}
	return antigravity.GetConfiguredProjectID(), nil
}

// SetProjectID persists the configured project ID for a provider.
func (s *AuthService) SetProjectID(provider, projectID string) error {
	if provider != "antigravity" {
		return fmt.Errorf("Invalid provider.")
	}
	return antigravity.SetConfiguredProjectID(projectID)
}

// validateProvider accepts only the OAuth providers with account support.
func validateProvider(provider string) error {
	switch provider {
	case "codex", "claude", "antigravity":
		return nil
	default:
		return ErrUnknownProvider
	}
}

// PatchAccount activates or deselects one account and/or renames it, and
// invalidates the provider's model and usage caches (model availability
// can differ per account plan). Returns the provider's post-mutation
// status so the client updates from one response.
func (s *AuthService) PatchAccount(provider, id string, active *bool, label *string) (ProviderAuthStatus, error) {
	if err := validateProvider(provider); err != nil {
		return ProviderAuthStatus{}, err
	}
	if id == "env" {
		// The synthetic env account exists only while the variable is set.
		if !envOwned(provider) {
			return ProviderAuthStatus{}, accounts.ErrNotFound
		}
		return ProviderAuthStatus{}, ErrEnvAccount
	}
	if active != nil {
		if err := accounts.Select(provider, id, *active); err != nil {
			return ProviderAuthStatus{}, err
		}
	}
	if label != nil {
		if err := accounts.SetLabel(provider, id, *label); err != nil {
			return ProviderAuthStatus{}, err
		}
	}
	s.invalidateProviderCaches(provider)
	return s.providerStatus(provider), nil
}

// DeleteAccount removes one stored login. Idempotent; deleting the active
// account promotes the next one, and the store's last remaining login for
// a provider also drops that provider's legacy credential file so a final
// delete cannot resurrect a stale login. Returns the post-mutation status.
func (s *AuthService) DeleteAccount(provider, id string) (ProviderAuthStatus, error) {
	if err := validateProvider(provider); err != nil {
		return ProviderAuthStatus{}, err
	}
	if id == "env" {
		if !envOwned(provider) {
			return ProviderAuthStatus{}, accounts.ErrNotFound
		}
		return ProviderAuthStatus{}, ErrEnvAccount
	}
	if err := accounts.Delete(provider, id); err != nil {
		return ProviderAuthStatus{}, err
	}
	// With the store empty for this provider, the legacy file is the only
	// thing left referencing a login; keeping it would bring a deleted
	// account back to life as "the provider's credential".
	if any, err := accounts.HasAny(provider); err == nil && !any {
		switch provider {
		case "claude":
			_ = claude.ClearLegacyCredentialFile()
		case "codex":
			_ = codex.ClearLegacyCredentialFile()
		case "antigravity":
			_ = antigravity.ClearLegacyCredentialFile()
		}
	}
	s.invalidateProviderCaches(provider)
	return s.providerStatus(provider), nil
}

// invalidateProviderCaches drops everything downstream that assumed a
// single account: model discovery and the provider's usage report.
func (s *AuthService) invalidateProviderCaches(provider string) {
	switch provider {
	case "claude":
		claude.InvalidateModelCache()
	case "codex":
		codex.InvalidateModelCache()
	case "antigravity":
		antigravity.InvalidateModelCache()
	}
	if s.Usage != nil {
		s.Usage.Invalidate(provider)
	}
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

// newAntigravityStateToken returns 16 random bytes, hex-encoded.
func newAntigravityStateToken() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
