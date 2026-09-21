// Antigravity (Google Cloud Code Assist) constants. Port of
// apps/server/providers/src/constants.ts (antigravity slice).
package antigravity

import (
	"encoding/base64"
	"os"
	"runtime"
)

// BaseURL is the daily Cloud Code Assist endpoint.
const BaseURL = "https://daily-cloudcode-pa.googleapis.com"

const DefaultVersion = "2.1.4"

const OAuthAuthorizeURL = "https://accounts.google.com/o/oauth2/v2/auth"
const OAuthTokenURL = "https://oauth2.googleapis.com/token"
const UserInfoURL = "https://www.googleapis.com/oauth2/v1/userinfo"

// RefreshSkewMs mirrors REFRESH_SKEW_MS: refresh when expiry is near.
const RefreshSkewMs = 60_000

var oauthScopes = []string{
	"https://www.googleapis.com/auth/cloud-platform",
	"https://www.googleapis.com/auth/userinfo.email",
	"https://www.googleapis.com/auth/userinfo.profile",
	"https://www.googleapis.com/auth/cclog",
	"https://www.googleapis.com/auth/experimentsandconfigs",
}

func mustDecode(b64 string) string {
	raw, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return ""
	}
	return string(raw)
}

// ClientID/ClientSecret mirror the Antigravity CLI's public OAuth client,
// overridable via env for testing.
func ClientID() string {
	if v := os.Getenv("ANTIGRAVITY_CLIENT_ID"); v != "" {
		return v
	}
	return mustDecode("MTA3MTAwNjA2MDU5MS10bWhzc2luMmgyMWxjcmUyMzV2dG9sb2poNGc0MDNlcC5hcHBzLmdvb2dsZXVzZXJjb250ZW50LmNvbQ==")
}

func ClientSecret() string {
	if v := os.Getenv("ANTIGRAVITY_CLIENT_SECRET"); v != "" {
		return v
	}
	return mustDecode("R09DU1BYLUs1OEZXUjQ4NkxkTEoxbUxCOHNYQzR6NnFEQWY=")
}

const OAuthPort = 51121
const OAuthCallbackPath = "/oauth-callback"

func Scopes() []string { return oauthScopes }

// UserAgent mirrors getAntigravityUserAgent().
func UserAgent() string {
	version := os.Getenv("ANTIGRAVITY_VERSION")
	if version == "" {
		version = DefaultVersion
	}
	osName := runtime.GOOS
	if osName == "windows" {
		osName = "windows"
	}
	arch := runtime.GOARCH
	switch arch {
	case "amd64":
		arch = "amd64"
	case "386":
		arch = "386"
	}
	return "antigravity/hub/" + version + " " + osName + "/" + arch
}

// GeminiThinkingLevels mirrors GEMINI_THINKING_LEVELS.
var GeminiThinkingLevels = []string{"minimal", "low", "medium", "high"}
