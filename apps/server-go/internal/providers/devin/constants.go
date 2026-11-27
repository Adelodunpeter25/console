// Devin (Codeium Cascade) constants: Connect-RPC endpoints, the pinned
// client identity the backend gates behavior on, and credential paths.
// Wire quirks mirror oh-my-pi's providers/devin.ts — the server checks the
// ideType/ideVersion tuple, so keep these in sync with the released Devin CLI.
package devin

// BaseURL is the Codeium/Windsurf Cascade host (Connect protocol over
// HTTP/1.1). GetUserJwt can return a customApiServerUrl that overrides it
// per account (enterprise deployments).
const BaseURL = "https://server.codeium.com"

// AuthBaseURL hosts the CLI OAuth endpoints (authorize page + token exchange).
const AuthBaseURL = "https://api.devin.ai"

const AuthorizeURL = "https://app.devin.ai/auth/cli/continue"
const TokenURL = AuthBaseURL + "/auth/cli/token"

// OAuthCallbackPort/Path are fixed by the Devin CLI contract: the authorize
// page only redirects to http://127.0.0.1:59653/callback.
const OAuthCallbackPort = 59653
const OAuthCallbackPath = "/callback"

const authPath = "/exa.auth_pb.AuthService/GetUserJwt"
const chatPath = "/exa.api_server_pb.ApiServerService/GetChatMessage"
const assignModelPath = "/exa.api_server_pb.ApiServerService/AssignModel"
const cliModelConfigsPath = "/exa.api_server_pb.ApiServerService/GetCliModelConfigs"

// Connect protocol envelope flags (Connect streaming spec).
const connectCompressedFlag = 0x01
const connectEndStreamFlag = 0x02

// maxConnectFramePayload caps one response envelope payload.
const maxConnectFramePayload = 16 * 1024 * 1024

// largeHistoryRecoveryBytes is the oh-my-pi threshold above which an
// invalid_argument "internal error" trailer is treated as context overflow
// so the agent loop's emergency compaction retries with shrunk history.
const largeHistoryRecoveryBytes = 512 * 1024

// sessionTokenPrefix is the wire prefix the Cascade API requires on the
// session token carried in Metadata.apiKey.
const sessionTokenPrefix = "devin-session-token$"

// normalizeSessionToken adds the scheme prefix unless already present.
// Legacy Windsurf keys are sent unprefixed — the auth call retries without
// the prefix on 401 (see requestUserJWT).
func normalizeSessionToken(apiKey string) string {
	if apiKey == "" || (len(apiKey) >= len(sessionTokenPrefix) && apiKey[:len(sessionTokenPrefix)] == sessionTokenPrefix) {
		return apiKey
	}
	return sessionTokenPrefix + apiKey
}

// imageBlindUIDs advertise supports_images but silently drop
// ChatMessagePrompt.images (verified upstream, oh-my-pi #6072). Clients must
// see them as text-only so attachments take the fallback path.
var imageBlindUIDs = map[string]bool{
	"swe-1-6":      true,
	"swe-1-6-fast": true,
}

// defaultContextWindow/defaultMaxTokens mirror oh-my-pi's fallbacks when a
// config ships no window data.
const (
	defaultContextWindow = 200_000
	defaultMaxTokens     = 64_000
)
