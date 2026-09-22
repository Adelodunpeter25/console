// Claude (Anthropic subscription) backend constants.
// Port of apps/server/providers/src/claude/constants.ts.
package claude

import (
	"os"
	"strings"
)

// BaseURL is read dynamically so tests can override CLAUDE_BASE_URL per-case.
func BaseURL() string {
	if v := os.Getenv("CLAUDE_BASE_URL"); v != "" {
		return v
	}
	return "https://api.anthropic.com"
}

// ClientID is the Claude Code CLI OAuth client ID.
const ClientID = "9d1c250a-e61b-44d9-88ed-5944d1962f5e"

const AuthorizeURL = "https://claude.ai/oauth/authorize"
const TokenURL = "https://platform.claude.com/v1/oauth/token"
const CallbackPort = 53692
const CallbackPath = "/callback"
const Scope = "org:create_api_key user:profile user:inference user:sessions:claude_code user:mcp_servers user:file_upload"

// RefreshSkewMs mirrors CLAUDE_REFRESH_SKEW_MS: refresh when expiry is near.
const RefreshSkewMs = 5 * 60_000

// CodeVersion is the Claude Code CLI version on the Anthropic wire.
const CodeVersion = "2.1.280"

// SDKVersion is the @anthropic-ai/sdk version bundled by Claude Code.
const SDKVersion = "0.112.1"

// UserAgent mirrors Claude Code's CLI inference entrypoint.
const UserAgent = "claude-cli/" + CodeVersion + " (external, cli)"

// OAuthBetas are sent on subscription requests, mirroring oh-my-pi.
var OAuthBetas = []string{
	"claude-code-20250219",
	"oauth-2025-04-20",
	"interleaved-thinking-2025-05-14",
}

// MaxOutputTokens is the per-request output ceiling, mirroring Claude Code.
const MaxOutputTokens = 32_000

// ThinkingBudgetTokens is the always-on extended-thinking budget (medium).
// Must stay below MaxOutputTokens.
const ThinkingBudgetTokens = 8192

// ClaudeThinkingLevels mirrors CLAUDE_THINKING_LEVELS in TS: Claude
// supports 5 levels (low..max) — no "none" or "minimal".
var ClaudeThinkingLevels = []string{"low", "medium", "high", "xhigh", "max"}

func RedirectURI() string {
	return "http://localhost:53692/callback"
}

func MessagesURL(baseURL string) string {
	if baseURL == "" {
		baseURL = BaseURL()
	}
	normalized := strings.TrimRight(baseURL, "/")
	// ?beta=true marks Claude Code OAuth traffic, mirroring oh-my-pi.
	return normalized + "/v1/messages?beta=true"
}

func ModelsURL(baseURL string) string {
	if baseURL == "" {
		baseURL = BaseURL()
	}
	return strings.TrimRight(baseURL, "/") + "/v1/models"
}
