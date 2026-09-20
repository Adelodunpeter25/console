// OpenAI Codex (ChatGPT subscription) backend constants.
// Port of apps/server/providers/src/codex/constants.ts.
package codex

import (
	"net/url"
	"os"
	"strings"
)

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// BaseURL is read dynamically so tests can override CODEX_BASE_URL per-case.
func BaseURL() string {
	return envOr("CODEX_BASE_URL", "https://chatgpt.com/backend-api")
}

const ClientVersion = "0.144.1"
const ClientID = "app_EMoamEEZ73f0CkXaXp7hrann"
const AuthorizeURL = "https://auth.openai.com/oauth/authorize"
const TokenURL = "https://auth.openai.com/oauth/token"
const CallbackPort = 1455
const CallbackPath = "/auth/callback"
const Scope = "openid profile email offline_access api.connectors.read api.connectors.invoke"
const AccountClaim = "https://api.openai.com/auth"
const ProfileClaim = "https://api.openai.com/profile"

// RefreshSkewMs mirrors CODEX_REFRESH_SKEW_MS: refresh when expiry is near.
const RefreshSkewMs = 60_000

func ResponsesURL(baseURL string) string {
	if baseURL == "" {
		baseURL = BaseURL()
	}
	normalized := strings.TrimRight(baseURL, "/")
	if strings.HasSuffix(normalized, "/codex/responses") {
		return normalized
	}
	return normalized + "/codex/responses"
}

func ModelsURL(baseURL string) string {
	if baseURL == "" {
		baseURL = BaseURL()
	}
	normalized := strings.TrimRight(baseURL, "/")
	return normalized + "/codex/models?client_version=" + url.QueryEscape(ClientVersion)
}
