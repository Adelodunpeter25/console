// Codex ChatGPT OAuth: PKCE, authorize URL, code exchange, token refresh,
// credential load/save. Port of apps/server/providers/src/codex/oauth.ts.
// Generic JWT/PKCE/file helpers live in providers/shared.
package codex

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/providers/shared"
)

// OAuthCredential is the stored Codex credential file shape.
type OAuthCredential struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresAt    int64  `json:"expiresAt"`
	AccountID    string `json:"accountId"`
	Email        string `json:"email,omitempty"`
	PlanType     string `json:"planType,omitempty"`
}

// ParsedCredential is the validated in-memory credential.
type ParsedCredential struct {
	AccessToken  string
	RefreshToken string
	ExpiresAtMs  int64
	AccountID    string
	Email        string
}

type tokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	IDToken      string `json:"id_token"`
	ExpiresIn    *int64 `json:"expires_in"`
}

// CredentialPath mirrors oauth.ts: CODEX_CREDENTIALS_PATH override,
// otherwise ~/.console/codex-creds.json.
func CredentialPath() string {
	return shared.CredentialPath("CODEX_CREDENTIALS_PATH", "codex-creds.json")
}

// DecodeJWTPayload decodes the middle JWT segment without verification.
func DecodeJWTPayload(token string) map[string]any {
	return shared.DecodeJWTPayload(token)
}

// TokenProfile extracts account id / email / plan from access + id tokens.
func TokenProfile(accessToken, idToken string) (accountID, email, planType string) {
	payload := shared.DecodeJWTPayload(accessToken)
	var idPayload map[string]any
	if idToken != "" {
		idPayload = shared.DecodeJWTPayload(idToken)
	}
	if auth, ok := payload[AccountClaim].(map[string]any); ok {
		if v, ok := auth["chatgpt_account_id"].(string); ok {
			accountID = v
		}
		if v, ok := auth["chatgpt_plan_type"].(string); ok {
			planType = strings.TrimSpace(strings.ToLower(v))
		}
	}
	if planType == "" {
		if auth, ok := idPayload[AccountClaim].(map[string]any); ok {
			if v, ok := auth["chatgpt_plan_type"].(string); ok {
				planType = strings.TrimSpace(strings.ToLower(v))
			}
		}
	}
	if profile, ok := payload[ProfileClaim].(map[string]any); ok {
		if v, ok := profile["email"].(string); ok {
			email = strings.TrimSpace(strings.ToLower(v))
		}
	}
	return accountID, email, planType
}

// GeneratePKCE creates a verifier/challenge pair (S256).
func GeneratePKCE() (verifier, challenge string, err error) {
	return shared.GeneratePKCE()
}

// AuthorizationURL builds the ChatGPT OAuth authorize URL + redirect URI.
func AuthorizationURL(state, verifierChallenge string) (authURL, redirectURI string) {
	redirectURI = fmt.Sprintf("http://localhost:%d%s", CallbackPort, CallbackPath)
	u, _ := url.Parse(AuthorizeURL)
	q := url.Values{
		"response_type":              {"code"},
		"client_id":                  {ClientID},
		"redirect_uri":               {redirectURI},
		"scope":                      {Scope},
		"code_challenge":             {verifierChallenge},
		"code_challenge_method":      {"S256"},
		"state":                      {state},
		"id_token_add_organizations": {"true"},
		"codex_cli_simplified_flow":  {"true"},
		"originator":                 {"pi"},
	}
	u.RawQuery = q.Encode()
	return u.String(), redirectURI
}

func postToken(client *http.Client, body url.Values) (tokenResponse, error) {
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second}
	}
	req, err := http.NewRequest("POST", TokenURL, strings.NewReader(body.Encode()))
	if err != nil {
		return tokenResponse{}, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := client.Do(req)
	if err != nil {
		return tokenResponse{}, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return tokenResponse{}, shared.TokenError("Codex", resp.StatusCode, string(raw))
	}
	var data tokenResponse
	if err := json.Unmarshal(raw, &data); err != nil {
		return tokenResponse{}, fmt.Errorf("decode Codex token response: %w", err)
	}
	return data, nil
}

// ExchangeCode exchanges an authorization code for stored credentials.
func ExchangeCode(client *http.Client, code, verifier, redirectURI string) (OAuthCredential, error) {
	data, err := postToken(client, url.Values{
		"grant_type":    {"authorization_code"},
		"client_id":     {ClientID},
		"code":          {code},
		"code_verifier": {verifier},
		"redirect_uri":  {redirectURI},
	})
	if err != nil {
		return OAuthCredential{}, err
	}
	if data.AccessToken == "" || data.RefreshToken == "" || data.ExpiresIn == nil {
		return OAuthCredential{}, fmt.Errorf("Codex OAuth response is missing required token fields.")
	}
	accountID, email, planType := TokenProfile(data.AccessToken, data.IDToken)
	if accountID == "" {
		return OAuthCredential{}, fmt.Errorf("Codex OAuth response did not include a ChatGPT account ID.")
	}
	return OAuthCredential{
		AccessToken:  data.AccessToken,
		RefreshToken: data.RefreshToken,
		ExpiresAt:    time.Now().UnixMilli() + *data.ExpiresIn*1000,
		AccountID:    accountID,
		Email:        email,
		PlanType:     planType,
	}, nil
}

func refreshToken(client *http.Client, cred ParsedCredential) (ParsedCredential, error) {
	data, err := postToken(client, url.Values{
		"grant_type":    {"refresh_token"},
		"client_id":     {ClientID},
		"refresh_token": {cred.RefreshToken},
	})
	if err != nil {
		return ParsedCredential{}, err
	}
	if data.AccessToken == "" || data.RefreshToken == "" || data.ExpiresIn == nil {
		return ParsedCredential{}, fmt.Errorf("Codex token refresh response is missing required fields.")
	}
	accountID, email, _ := TokenProfile(data.AccessToken, "")
	if accountID == "" {
		accountID = cred.AccountID
	}
	if email == "" {
		email = cred.Email
	}
	updated := OAuthCredential{
		AccessToken:  data.AccessToken,
		RefreshToken: data.RefreshToken,
		ExpiresAt:    time.Now().UnixMilli() + *data.ExpiresIn*1000,
		AccountID:    accountID,
		Email:        email,
	}
	if err := SaveCredential(updated); err != nil {
		return ParsedCredential{}, err
	}
	return ParseCredential(updated)
}

// ParseCredential validates a stored credential.
func ParseCredential(raw OAuthCredential) (ParsedCredential, error) {
	if raw.AccessToken == "" || raw.RefreshToken == "" || raw.AccountID == "" {
		return ParsedCredential{}, fmt.Errorf("Invalid Codex credential. Please login again.")
	}
	return ParsedCredential{
		AccessToken:  raw.AccessToken,
		RefreshToken: raw.RefreshToken,
		ExpiresAtMs:  raw.ExpiresAt,
		AccountID:    raw.AccountID,
		Email:        raw.Email,
	}, nil
}

// LoadCredential reads OPENAI_CODEX_OAUTH_TOKEN or the credential file.
func LoadCredential() (ParsedCredential, error) {
	if envToken := os.Getenv("OPENAI_CODEX_OAUTH_TOKEN"); envToken != "" {
		accountID, email, _ := TokenProfile(envToken, "")
		if accountID == "" {
			return ParsedCredential{}, fmt.Errorf("OPENAI_CODEX_OAUTH_TOKEN has no ChatGPT account ID.")
		}
		return ParsedCredential{
			AccessToken:  envToken,
			RefreshToken: "",
			ExpiresAtMs:  1<<62 - 1,
			AccountID:    accountID,
			Email:        email,
		}, nil
	}
	raw, err := os.ReadFile(CredentialPath())
	if err != nil {
		return ParsedCredential{}, err
	}
	// Tolerate extra whitespace/BOM-free JSON only (parity with TS readFile).
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
	if os.Getenv("OPENAI_CODEX_OAUTH_TOKEN") != "" {
		return true
	}
	return shared.CredentialFileExists(CredentialPath())
}

// RefreshIfNeeded refreshes an expiring credential (60s skew).
func RefreshIfNeeded(client *http.Client, cred ParsedCredential) (ParsedCredential, error) {
	if cred.RefreshToken == "" || time.Now().UnixMilli()+RefreshSkewMs < cred.ExpiresAtMs {
		return cred, nil
	}
	return refreshToken(client, cred)
}
