// Claude (Anthropic subscription) backend constants.
// Port of apps/server/providers/src/claude/constants.ts.
package claude

import (
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
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

// defaultCodeVersion is the pinned fallback Claude Code CLI version on the
// Anthropic wire, used until (if ever) a server rejection names a newer
// required one — see AdoptRequiredClaudeCodeVersion.
const defaultCodeVersion = "2.1.280"

// SDKVersion is the @anthropic-ai/sdk version bundled by Claude Code.
const SDKVersion = "0.112.1"

var (
	versionMu      sync.Mutex
	adoptedVersion string
)

var requiredVersionPattern = regexp.MustCompile(`version (\d+\.\d+\.\d+) or newer is required`)

// CodeVersion is the Claude Code CLI version represented on the wire:
// CONSOLE_CLAUDE_CODE_VERSION env override, then any version adopted at
// runtime (AdoptRequiredClaudeCodeVersion), then the pinned default.
func CodeVersion() string {
	if v := os.Getenv("CONSOLE_CLAUDE_CODE_VERSION"); v != "" {
		return v
	}
	versionMu.Lock()
	defer versionMu.Unlock()
	if adoptedVersion != "" {
		return adoptedVersion
	}
	return defaultCodeVersion
}

// UserAgent mirrors Claude Code's CLI inference entrypoint, using whatever
// wire version CodeVersion currently resolves to.
func UserAgent() string {
	return "claude-cli/" + CodeVersion() + " (external, cli)"
}

func compareSemver(a, b string) int {
	pa := strings.SplitN(a, ".", 3)
	pb := strings.SplitN(b, ".", 3)
	for i := 0; i < 3; i++ {
		var na, nb int
		if i < len(pa) {
			na, _ = strconv.Atoi(pa[i])
		}
		if i < len(pb) {
			nb, _ = strconv.Atoi(pb[i])
		}
		if na != nb {
			return na - nb
		}
	}
	return 0
}

// AdoptRequiredClaudeCodeVersion adopts the minimum version named by an
// Anthropic "claude_code_version_too_old" rejection for the rest of the
// process, so the pinned fallback going stale costs one rejected request
// instead of a hard failure. Returns true only when the wire version
// actually increased — callers should retry once on true; a repeated
// rejection at the same adopted version returns false so a retry can never
// loop. Always false while CONSOLE_CLAUDE_CODE_VERSION pins the version
// explicitly.
func AdoptRequiredClaudeCodeVersion(responseBody string) bool {
	if os.Getenv("CONSOLE_CLAUDE_CODE_VERSION") != "" {
		return false
	}
	if !strings.Contains(responseBody, "claude_code_version_too_old") {
		return false
	}
	match := requiredVersionPattern.FindStringSubmatch(responseBody)
	if match == nil {
		return false
	}
	required := match[1]
	versionMu.Lock()
	defer versionMu.Unlock()
	current := adoptedVersion
	if current == "" {
		current = defaultCodeVersion
	}
	if compareSemver(required, current) <= 0 {
		return false
	}
	adoptedVersion = required
	return true
}

// OAuthBetas are sent on subscription requests, mirroring oh-my-pi.
var OAuthBetas = []string{
	"claude-code-20250219",
	"oauth-2025-04-20",
	"interleaved-thinking-2025-05-14",
}

// MaxOutputTokens is the per-request output ceiling, mirroring Claude
// Code's own CLI (CLAUDE_CODE_MAX_OUTPUT_TOKENS).
const MaxOutputTokens = 64_000

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
