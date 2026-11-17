// Devin CLI OAuth: PKCE authorize URL, code exchange, and credential
// load/save. The flow mirrors the Devin CLI contract (see oh-my-pi
// compat/rules/auth/devin.kdl): authorize on app.devin.ai, callback on the
// fixed loopback port, JSON token exchange on api.devin.ai. The resulting
// credential is a long-lived session JWT — there is no refresh grant, so an
// expired token surfaces as a 401 and the user logs in again.
package devin

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"time"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/accounts"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/providers/shared"
)

// OAuthCredential is the stored Devin credential file shape.
type OAuthCredential struct {
	// Token is the CLI session token (a JWT; may or may not carry the
	// devin-session-token$ prefix — normalizeSessionToken adds it on use).
	Token string `json:"token"`
	Email string `json:"email,omitempty"`
	// ExpiresAt is unix millis from the token's JWT exp claim; 0 means the
	// expiry is unknown (treated as non-expiring — the API is the judge).
	ExpiresAt int64 `json:"expiresAt,omitempty"`
}

// ParsedCredential is the validated in-memory credential.
type ParsedCredential struct {
	Token       string
	Email       string
	ExpiresAtMs int64
}

// CredentialPath returns the DEVIN_CREDENTIALS_PATH override if set,
// otherwise ~/.console/devin-creds.json.
func CredentialPath() string {
	return shared.CredentialPath("DEVIN_CREDENTIALS_PATH", "devin-creds.json")
}

// envToken returns the DEVIN_API_KEY override, which keeps precedence over
// every stored credential.
func envToken() string {
	return os.Getenv("DEVIN_API_KEY")
}

// EnvToken exposes env-token presence for the status API.
func EnvToken() string { return envToken() }

// ClearLegacyCredentialFile removes the pre-accounts single-credential
// file, dropping a stale login that nothing else references.
func ClearLegacyCredentialFile() error {
	if err := os.Remove(CredentialPath()); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// activeCredentialPath resolves where the credential runs use lives: the
// active account's file when the store holds accounts, the legacy
// single-credential file otherwise. An unconfigured store (tests, tools)
// is today's behavior exactly; a store with rows but no active account is
// the documented "needs a selection" state and errors instead of
// resurrecting the leftover legacy file.
func activeCredentialPath() (string, error) {
	id, err := accounts.EnabledID("devin")
	if err == nil && id != "" {
		return accounts.CredentialPath("devin", id), nil
	}
	if any, herr := accounts.HasAny("devin"); herr == nil && any {
		return "", fmt.Errorf("Devin has stored accounts but none is active; select one in settings.")
	}
	return CredentialPath(), nil
}

// GeneratePKCE creates the PKCE verifier/challenge pair for the CLI flow.
func GeneratePKCE() (verifier, challenge string, err error) {
	return shared.GeneratePKCE()
}

// AuthorizationURL builds the Devin CLI authorize URL + redirect URI. The
// redirect URI is fixed by the provider (127.0.0.1:59653/callback); clients
// bind that port to catch the code.
func AuthorizationURL(state, verifierChallenge string) (authURL, redirectURI string) {
	redirectURI = fmt.Sprintf("http://127.0.0.1:%d%s", OAuthCallbackPort, OAuthCallbackPath)
	q := url.Values{}
	q.Set("response_type", "code")
	q.Set("redirect_uri", redirectURI)
	q.Set("code_challenge", verifierChallenge)
	q.Set("code_challenge_method", "S256")
	q.Set("state", state)
	q.Set("prompt", "select_account")
	return AuthorizeURL + "?" + q.Encode(), redirectURI
}

// tokenResponse is the api.devin.ai/auth/cli/token JSON body.
type tokenResponse struct {
	Token string `json:"token"`
}

// ExchangeCode exchanges an authorization code for a session token. The
// endpoint is non-standard: a bare JSON {code, code_verifier} POST with no
// grant_type or redirect_uri (the code is already bound to the fixed
// callback).
func ExchangeCode(client *http.Client, code, verifier string) (OAuthCredential, error) {
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second}
	}
	body, err := json.Marshal(map[string]string{
		"code":          code,
		"code_verifier": verifier,
	})
	if err != nil {
		return OAuthCredential{}, err
	}
	req, err := http.NewRequest("POST", TokenURL, bytes.NewReader(body))
	if err != nil {
		return OAuthCredential{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return OAuthCredential{}, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return OAuthCredential{}, shared.TokenError("Devin", resp.StatusCode, string(raw))
	}
	var data tokenResponse
	if err := json.Unmarshal(raw, &data); err != nil {
		return OAuthCredential{}, fmt.Errorf("decode Devin token response: %w", err)
	}
	if data.Token == "" {
		return OAuthCredential{}, fmt.Errorf("Devin token response carried no token.")
	}
	return OAuthCredential{
		Token:     data.Token,
		ExpiresAt: tokenExpiryMS(data.Token),
	}, nil
}

// tokenExpiryMS reads the JWT exp claim (seconds) into unix millis; 0 when
// the token is not a JWT or carries no exp.
func tokenExpiryMS(token string) int64 {
	claims := shared.DecodeJWTPayload(token)
	if claims == nil {
		return 0
	}
	switch exp := claims["exp"].(type) {
	case float64:
		return int64(exp) * 1000
	case json.Number:
		if v, err := exp.Int64(); err == nil {
			return v * 1000
		}
	}
	return 0
}

// tokenIdentity picks the account identity from the token's JWT claims:
// email, then subject, then a stable prefix of the token itself.
func tokenIdentity(token string) (identity, email string) {
	claims := shared.DecodeJWTPayload(token)
	if claims != nil {
		if e, ok := claims["email"].(string); ok && e != "" {
			email = e
			return e, e
		}
		for _, key := range []string{"sub", "user_id", "uid"} {
			if v, ok := claims[key].(string); ok && v != "" {
				return v, email
			}
		}
	}
	if len(token) > 16 {
		return "token-" + token[:16], ""
	}
	return "token-" + token, ""
}

// TokenIdentity exposes the account identity for the login callback.
func TokenIdentity(token string) (identity, email string) {
	return tokenIdentity(token)
}

// ParseCredential validates a stored credential.
func ParseCredential(raw OAuthCredential) (ParsedCredential, error) {
	if raw.Token == "" {
		return ParsedCredential{}, fmt.Errorf("Invalid Devin credential. Please login again.")
	}
	return ParsedCredential{
		Token:       raw.Token,
		Email:       raw.Email,
		ExpiresAtMs: raw.ExpiresAt,
	}, nil
}

// LoadCredential reads DEVIN_API_KEY or the credential runs use: the active
// account's file with accounts, the legacy file otherwise.
func LoadCredential() (ParsedCredential, error) {
	if tok := envToken(); tok != "" {
		_, email := tokenIdentity(tok)
		return ParsedCredential{
			Token:       tok,
			Email:       email,
			ExpiresAtMs: tokenExpiryMS(tok),
		}, nil
	}
	path, err := activeCredentialPath()
	if err != nil {
		return ParsedCredential{}, err
	}
	return readCredentialFile(path)
}

// LoadCredentialFor reads one specific account's credential. A pinned id
// never substitutes the env token: env is its own account and silently
// substituting it for a named account is the fallback the run design rules
// out.
func LoadCredentialFor(accountID string) (ParsedCredential, error) {
	return readCredentialFile(accounts.CredentialPath("devin", accountID))
}

// readCredentialFile parses one credential file.
func readCredentialFile(path string) (ParsedCredential, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return ParsedCredential{}, err
	}
	var cred OAuthCredential
	if err := json.Unmarshal(raw, &cred); err != nil {
		return ParsedCredential{}, fmt.Errorf("parse Devin credential: %w", err)
	}
	return ParseCredential(cred)
}

// SaveCredential writes the legacy single-credential file (0600 dir 0700).
func SaveCredential(cred OAuthCredential) error {
	return shared.SaveCredentialFile(CredentialPath(), cred)
}

// SaveCredentialFor writes one account's credential file; empty id means
// the legacy single-credential file.
func SaveCredentialFor(accountID string, cred OAuthCredential) error {
	if accountID == "" {
		return SaveCredential(cred)
	}
	return shared.SaveCredentialFile(accounts.CredentialPath("devin", accountID), cred)
}

// CredentialExists reports whether a login exists (env token or any stored
// credential).
func CredentialExists() bool {
	if envToken() != "" {
		return true
	}
	if shared.CredentialFileExists(CredentialPath()) {
		return true
	}
	if id, err := accounts.EnabledID("devin"); err == nil && id != "" {
		return shared.CredentialFileExists(accounts.CredentialPath("devin", id))
	}
	return false
}
