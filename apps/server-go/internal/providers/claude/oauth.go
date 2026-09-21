// Claude OAuth: PKCE authorization URL, JSON token exchange, credential
// storage. Port of apps/server/providers/src/claude/oauth.ts.
package claude

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"time"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/providers/shared"
)

// OAuthCredential is the stored credential file shape.
type OAuthCredential struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresAt    int64  `json:"expiresAt"`
	Email        string `json:"email,omitempty"`
}

// ParsedCredential is the validated in-memory credential.
type ParsedCredential struct {
	AccessToken  string
	RefreshToken string
	ExpiresAtMs  int64
	Email        string
}

type tokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    *int64 `json:"expires_in"`
}

// CredentialPath mirrors oauth.ts: CLAUDE_CREDENTIALS_PATH override,
// otherwise ~/.console/claude-creds.json.
func CredentialPath() string {
	return shared.CredentialPath("CLAUDE_CREDENTIALS_PATH", "claude-creds.json")
}

func GeneratePKCE() (verifier, challenge string, err error) {
	return shared.GeneratePKCE()
}

// AuthorizationURL builds the Claude Pro/Max PKCE authorize URL.
func AuthorizationURL(state, verifierChallenge string) (authURL, redirectURI string) {
	redirectURI = RedirectURI()
	u, _ := url.Parse(AuthorizeURL)
	q := url.Values{
		"code":                  {"true"},
		"client_id":             {ClientID},
		"response_type":         {"code"},
		"redirect_uri":          {redirectURI},
		"scope":                 {Scope},
		"code_challenge":        {verifierChallenge},
		"code_challenge_method": {"S256"},
		"state":                 {state},
	}
	u.RawQuery = q.Encode()
	return u.String(), redirectURI
}

func postToken(client *http.Client, body map[string]string) (tokenResponse, error) {
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second}
	}
	rawBody, err := json.Marshal(body)
	if err != nil {
		return tokenResponse{}, err
	}
	req, err := http.NewRequest("POST", TokenURL, bytes.NewReader(rawBody))
	if err != nil {
		return tokenResponse{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return tokenResponse{}, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return tokenResponse{}, shared.TokenError("Claude", resp.StatusCode, string(raw))
	}
	var data tokenResponse
	if err := json.Unmarshal(raw, &data); err != nil {
		return tokenResponse{}, fmt.Errorf("decode Claude token response: %w", err)
	}
	return data, nil
}

// fetchBootstrapEmail is a best-effort account identity lookup via the
// Claude Code bootstrap endpoint. It never throws (mirrors oauth.ts).
func fetchBootstrapEmail(client *http.Client, accessToken string) string {
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, "GET",
		"https://api.anthropic.com/api/claude_cli/bootstrap?entrypoint=cli&model=claude-sonnet-4-6", nil)
	if err != nil {
		return ""
	}
	req.Header.Set("Accept", "application/json, text/plain, */*")
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("anthropic-beta", "oauth-2025-04-20")
	resp, err := client.Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return ""
	}
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	var payload struct {
		OAuthAccount *struct {
			AccountEmail string `json:"account_email"`
		} `json:"oauth_account"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return ""
	}
	if payload.OAuthAccount == nil || payload.OAuthAccount.AccountEmail == "" {
		return ""
	}
	return payload.OAuthAccount.AccountEmail
}

// ExchangeCode exchanges an authorization code for stored credentials.
func ExchangeCode(client *http.Client, code, state, verifier, redirectURI string) (OAuthCredential, error) {
	data, err := postToken(client, map[string]string{
		"grant_type":    "authorization_code",
		"client_id":     ClientID,
		"code":          code,
		"state":         state,
		"redirect_uri":  redirectURI,
		"code_verifier": verifier,
	})
	if err != nil {
		return OAuthCredential{}, err
	}
	if data.AccessToken == "" || data.RefreshToken == "" || data.ExpiresIn == nil {
		return OAuthCredential{}, fmt.Errorf("Claude OAuth response is missing required token fields.")
	}
	return OAuthCredential{
		AccessToken:  data.AccessToken,
		RefreshToken: data.RefreshToken,
		ExpiresAt:    time.Now().UnixMilli() + *data.ExpiresIn*1000,
		Email:        fetchBootstrapEmail(client, data.AccessToken),
	}, nil
}

func refreshToken(client *http.Client, cred ParsedCredential) (ParsedCredential, error) {
	data, err := postToken(client, map[string]string{
		"grant_type":    "refresh_token",
		"client_id":     ClientID,
		"refresh_token": cred.RefreshToken,
	})
	if err != nil {
		return ParsedCredential{}, err
	}
	if data.AccessToken == "" || data.RefreshToken == "" || data.ExpiresIn == nil {
		return ParsedCredential{}, fmt.Errorf("Claude token refresh response is missing required fields.")
	}
	email := cred.Email
	if email == "" {
		email = fetchBootstrapEmail(client, data.AccessToken)
	}
	updated := OAuthCredential{
		AccessToken:  data.AccessToken,
		RefreshToken: data.RefreshToken,
		ExpiresAt:    time.Now().UnixMilli() + *data.ExpiresIn*1000,
		Email:        email,
	}
	if err := SaveCredential(updated); err != nil {
		return ParsedCredential{}, err
	}
	return ParseCredential(updated)
}

// ParseCredential validates a stored credential.
func ParseCredential(raw OAuthCredential) (ParsedCredential, error) {
	if raw.AccessToken == "" || raw.RefreshToken == "" {
		return ParsedCredential{}, fmt.Errorf("Invalid Claude credential. Please login again.")
	}
	return ParsedCredential{
		AccessToken:  raw.AccessToken,
		RefreshToken: raw.RefreshToken,
		ExpiresAtMs:  raw.ExpiresAt,
		Email:        raw.Email,
	}, nil
}

// LoadCredential reads CLAUDE_OAUTH_TOKEN/ANTHROPIC_OAUTH_TOKEN or the
// credential file. Env tokens never expire (parity with TS MAX_SAFE_INTEGER).
func LoadCredential() (ParsedCredential, error) {
	envToken := os.Getenv("CLAUDE_OAUTH_TOKEN")
	if envToken == "" {
		envToken = os.Getenv("ANTHROPIC_OAUTH_TOKEN")
	}
	if envToken != "" {
		return ParsedCredential{
			AccessToken:  envToken,
			RefreshToken: "",
			ExpiresAtMs:  1<<62 - 1,
			Email:        "",
		}, nil
	}
	raw, err := os.ReadFile(CredentialPath())
	if err != nil {
		return ParsedCredential{}, err
	}
	dec := json.NewDecoder(bytes.NewReader(bytes.TrimSpace(raw)))
	var cred OAuthCredential
	if err := dec.Decode(&cred); err != nil {
		return ParsedCredential{}, err
	}
	return ParseCredential(cred)
}

// SaveCredential writes the credential file (0600 dir 0700).
func SaveCredential(cred OAuthCredential) error {
	return shared.SaveCredentialFile(CredentialPath(), cred)
}

// CredentialExists reports whether a login exists (env token or file).
func CredentialExists() bool {
	if os.Getenv("CLAUDE_OAUTH_TOKEN") != "" || os.Getenv("ANTHROPIC_OAUTH_TOKEN") != "" {
		return true
	}
	return shared.CredentialFileExists(CredentialPath())
}

// RefreshIfNeeded refreshes an expiring credential (5-minute skew).
func RefreshIfNeeded(client *http.Client, cred ParsedCredential) (ParsedCredential, error) {
	if cred.RefreshToken == "" || time.Now().UnixMilli()+RefreshSkewMs < cred.ExpiresAtMs {
		return cred, nil
	}
	return refreshToken(client, cred)
}
