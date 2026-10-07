// Antigravity interactive OAuth handshake: code exchange, user email lookup,
// loadCodeAssist, onboardUser, and completing the auth flow.
package antigravity

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/providers/shared"
)

// Tier is the {id, isDefault} tier shape.
type Tier struct {
	ID        string `json:"id,omitempty"`
	IsDefault bool   `json:"isDefault,omitempty"`
}

// TierFree and TierLegacy are the tier IDs used during onboarding.
const (
	TierFree   = "free-tier"
	TierLegacy = "legacy-tier"
)

// TokenResult is the token endpoint response for an authorization code.
type TokenResult struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int64  `json:"expires_in"`
}

func tokenURL() string {
	if v := os.Getenv("ANTIGRAVITY_TOKEN_URL"); v != "" {
		return v
	}
	return OAuthTokenURL
}

func userInfoURL() string {
	if v := os.Getenv("ANTIGRAVITY_USERINFO_URL"); v != "" {
		return v
	}
	return UserInfoURL
}

func codeAssistBase() string {
	if v := os.Getenv("ANTIGRAVITY_BASE_URL"); v != "" {
		return strings.TrimRight(v, "/")
	}
	return BaseURL
}

// RedirectURI mirrors ANTIGRAVITY_OAUTH_CONFIG port + callbackPath.
func RedirectURI() string {
	return fmt.Sprintf("http://localhost:%d%s", OAuthPort, OAuthCallbackPath)
}

// AuthorizationURL builds the installed-app authorize URL + redirect URI.
// Param order: client_id, response_type, scope,
// redirect_uri, state, access_type, prompt. Antigravity does not use PKCE.
func AuthorizationURL(state string) (authURL, redirectURI string) {
	redirectURI = RedirectURI()
	q := "client_id=" + url.QueryEscape(ClientID()) +
		"&response_type=code" +
		"&scope=" + url.QueryEscape(strings.Join(Scopes(), " ")) +
		"&redirect_uri=" + url.QueryEscape(redirectURI) +
		"&state=" + url.QueryEscape(state) +
		"&access_type=offline" +
		"&prompt=consent"
	return OAuthAuthorizeURL + "?" + q, redirectURI
}

// ExchangeCode swaps an authorization code for tokens via form POST.
func ExchangeCode(client *http.Client, code, redirectURI string) (TokenResult, error) {
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second}
	}
	body := url.Values{
		"client_id":     {ClientID()},
		"client_secret": {ClientSecret()},
		"code":          {code},
		"redirect_uri":  {redirectURI},
		"grant_type":    {"authorization_code"},
	}
	req, err := http.NewRequest("POST", tokenURL(), strings.NewReader(body.Encode()))
	if err != nil {
		return TokenResult{}, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := client.Do(req)
	if err != nil {
		return TokenResult{}, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return TokenResult{}, shared.TokenError("Antigravity", resp.StatusCode, string(raw))
	}
	var data TokenResult
	if err := json.Unmarshal(raw, &data); err != nil {
		return TokenResult{}, fmt.Errorf("decode Antigravity token response: %w", err)
	}
	return data, nil
}

// GetUserEmail fetches the Google userinfo email for an access token.
func GetUserEmail(client *http.Client, accessToken string) (string, error) {
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second}
	}
	req, err := http.NewRequest("GET", userInfoURL(), nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("Failed to fetch user info (%d): %s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}
	var data struct {
		Email string `json:"email"`
	}
	if err := json.Unmarshal(raw, &data); err != nil {
		return "", fmt.Errorf("decode Antigravity userinfo response: %w", err)
	}
	return data.Email, nil
}

// GetDefaultTier returns legacy-tier when allowedTiers is empty or has no
// default.
func GetDefaultTier(allowedTiers []Tier) Tier {
	if len(allowedTiers) == 0 {
		return Tier{ID: TierLegacy}
	}
	for _, t := range allowedTiers {
		if t.IsDefault {
			return t
		}
	}
	return Tier{ID: TierLegacy}
}

// ReadProjectId extracts the project ID: cloudaicompanionProject may be a
// bare string or {id: string}.
func ReadProjectId(value any) string {
	switch v := value.(type) {
	case string:
		if v != "" {
			return v
		}
		return ""
	case map[string]any:
		if id, ok := v["id"].(string); ok && id != "" {
			return id
		}
		return ""
	case map[string]string:
		if id, ok := v["id"]; ok && id != "" {
			return id
		}
		return ""
	default:
		return ""
	}
}

type loadCodeAssistResponse struct {
	CloudaicompanionProject any    `json:"cloudaicompanionProject"`
	CurrentTier             *Tier  `json:"currentTier"`
	AllowedTiers            []Tier `json:"allowedTiers"`
}

type onboardOperation struct {
	Name     string `json:"name"`
	Done     bool   `json:"done"`
	Response *struct {
		CloudaicompanionProject any `json:"cloudaicompanionProject"`
	} `json:"response"`
}

func codeAssistHeaders(accessToken string) http.Header {
	h := http.Header{}
	h.Set("Authorization", "Bearer "+accessToken)
	h.Set("Content-Type", "application/json")
	h.Set("User-Agent", UserAgent())
	return h
}

func codeAssistMetadataBody(extra map[string]any) []byte {
	body := map[string]any{
		"metadata": map[string]any{
			"ideType":    "ANTIGRAVITY",
			"platform":   "PLATFORM_UNSPECIFIED",
			"pluginType": "GEMINI",
		},
	}
	for k, v := range extra {
		body[k] = v
	}
	raw, _ := json.Marshal(body)
	return raw
}

// LoadCodeAssist discovers the Cloud Code Assist project ID, onboarding the
// user if needed.
func LoadCodeAssist(client *http.Client, accessToken, explicitProjectID string) (string, error) {
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second}
	}
	envProjectID := explicitProjectID
	if envProjectID == "" {
		envProjectID = os.Getenv("GOOGLE_CLOUD_PROJECT")
	}
	if envProjectID == "" {
		envProjectID = os.Getenv("GOOGLE_CLOUD_PROJECT_ID")
	}
	endpoint := codeAssistBase() + "/v1internal:loadCodeAssist"
	req, err := http.NewRequest("POST", endpoint, bytes.NewReader(codeAssistMetadataBody(nil)))
	if err != nil {
		return "", err
	}
	req.Header = codeAssistHeaders(accessToken)
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("loadCodeAssist failed (%d): %s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}
	var data loadCodeAssistResponse
	if err := json.Unmarshal(raw, &data); err != nil {
		return "", fmt.Errorf("decode Antigravity loadCodeAssist response: %w", err)
	}
	if project := ReadProjectId(data.CloudaicompanionProject); project != "" {
		return project, nil
	}
	tierID := GetDefaultTier(data.AllowedTiers).ID
	if tierID == "" {
		tierID = TierFree
	}
	if data.CurrentTier != nil {
		currentTierID := data.CurrentTier.ID
		if currentTierID != "" && currentTierID != TierFree && currentTierID != TierLegacy && envProjectID == "" {
			return "", fmt.Errorf("This account requires setting the GOOGLE_CLOUD_PROJECT or GOOGLE_CLOUD_PROJECT_ID environment variable.")
		}
	}
	return OnboardUser(client, accessToken, tierID, envProjectID)
}

// OnboardUser onboards the user, polling the long-running
// operation up to 30 times at 1s.
func OnboardUser(client *http.Client, accessToken, tierID, envProjectID string) (string, error) {
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second}
	}
	endpoint := codeAssistBase() + "/v1internal:onboardUser"
	extra := map[string]any{}
	if tierID != "" {
		extra["tierId"] = tierID
	}
	req, err := http.NewRequest("POST", endpoint, bytes.NewReader(codeAssistMetadataBody(extra)))
	if err != nil {
		return "", err
	}
	req.Header = codeAssistHeaders(accessToken)
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("onboardUser failed (%d): %s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}
	var operation onboardOperation
	if err := json.Unmarshal(raw, &operation); err != nil {
		return "", fmt.Errorf("decode Antigravity onboardUser response: %w", err)
	}
	var projectID string
	if operation.Response != nil {
		projectID = ReadProjectId(operation.Response.CloudaicompanionProject)
	}
	if operation.Done && projectID != "" {
		return projectID, nil
	}
	if !operation.Done && operation.Name != "" {
		for i := 0; i < 30; i++ {
			time.Sleep(1 * time.Second)
			pollEndpoint := codeAssistBase() + "/v1internal/" + operation.Name
			pollReq, err := http.NewRequest("GET", pollEndpoint, nil)
			if err != nil {
				return "", err
			}
			pollReq.Header = codeAssistHeaders(accessToken)
			pollResp, err := client.Do(pollReq)
			if err != nil {
				return "", err
			}
			pollRaw, _ := io.ReadAll(io.LimitReader(pollResp.Body, 1<<20))
			pollResp.Body.Close()
			if pollResp.StatusCode < 200 || pollResp.StatusCode >= 300 {
				return "", fmt.Errorf("onboardUser poll failed (%d): %s", pollResp.StatusCode, strings.TrimSpace(string(pollRaw)))
			}
			var pollData onboardOperation
			if err := json.Unmarshal(pollRaw, &pollData); err != nil {
				return "", fmt.Errorf("decode Antigravity onboardUser poll response: %w", err)
			}
			var pollProjectID string
			if pollData.Response != nil {
				pollProjectID = ReadProjectId(pollData.Response.CloudaicompanionProject)
			}
			if pollData.Done && pollProjectID != "" {
				return pollProjectID, nil
			}
		}
		return "", fmt.Errorf("onboardUser timed out")
	}
	if envProjectID != "" {
		return envProjectID, nil
	}
	return "", fmt.Errorf("Could not discover or provision a Google Cloud project. Try setting the GOOGLE_CLOUD_PROJECT or GOOGLE_CLOUD_PROJECT_ID environment variable.")
}

// CompleteAuthFlowWithCode exchanges the authorization code and resolves the
// user's email and project ID into a credential.
func CompleteAuthFlowWithCode(client *http.Client, code, explicitProjectID string) (OAuthCredential, error) {
	redirectURI := RedirectURI()
	tokens, err := ExchangeCode(client, code, redirectURI)
	if err != nil {
		return OAuthCredential{}, err
	}
	email, err := GetUserEmail(client, tokens.AccessToken)
	if err != nil {
		return OAuthCredential{}, err
	}
	projectID, err := LoadCodeAssist(client, tokens.AccessToken, explicitProjectID)
	if err != nil {
		return OAuthCredential{}, err
	}
	return OAuthCredential{
		Token: tokens.AccessToken, RefreshToken: tokens.RefreshToken,
		ProjectID: projectID, Email: email,
		ExpiresAt: time.Now().UnixMilli() + tokens.ExpiresIn*1000,
	}, nil
}

// ConfigPath returns ~/.console/antigravity-config.json (overridable via
// ANTIGRAVITY_CONFIG_PATH).
func ConfigPath() string {
	return shared.CredentialPath("ANTIGRAVITY_CONFIG_PATH", "antigravity-config.json")
}

// GetConfiguredProjectID returns the user-configured project ID, or "".
func GetConfiguredProjectID() string {
	raw, err := os.ReadFile(ConfigPath())
	if err != nil {
		return ""
	}
	var cfg struct {
		ConfiguredProjectID *string `json:"configuredProjectId"`
	}
	if err := json.Unmarshal(bytes.TrimSpace(raw), &cfg); err != nil {
		return ""
	}
	if cfg.ConfiguredProjectID == nil {
		return ""
	}
	return strings.TrimSpace(*cfg.ConfiguredProjectID)
}

// SetConfiguredProjectID persists the user-configured project ID; empty
// clears it.
func SetConfiguredProjectID(projectID string) error {
	path := ConfigPath()
	var existing map[string]any
	if raw, err := os.ReadFile(path); err == nil {
		_ = json.Unmarshal(raw, &existing)
	}
	if existing == nil {
		existing = map[string]any{}
	}
	trimmed := strings.TrimSpace(projectID)
	if trimmed == "" {
		delete(existing, "configuredProjectId")
	} else {
		existing["configuredProjectId"] = trimmed
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(existing, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o600)
}
