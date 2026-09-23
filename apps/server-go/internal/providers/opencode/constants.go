// OpenCode Zen provider constants and wire policy.
package opencode

import (
	"net/http"
	"os"
	"strings"
	"sync"

	"github.com/google/uuid"
)

const (
	// DefaultBaseURL is the public OpenCode Zen API root.
	DefaultBaseURL = "https://opencode.ai/zen/v1"
	// DefaultUserAgent matches the OpenCode client identity required by Zen.
	DefaultUserAgent = "opencode/latest/2.0.15/cli"
	// DefaultContextWindow is used when discovery does not provide one.
	DefaultContextWindow = 200_000
)

var (
	sessionOnce sync.Once
	processID   string
)

func envOr(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

// BaseURL is read dynamically so tests and deployments can override it.
func BaseURL() string {
	return envOr("OPENCODE_BASE_URL", DefaultBaseURL)
}

// UserAgent is read dynamically for a deliberate client-version bump.
func UserAgent() string {
	return envOr("OPENCODE_USER_AGENT", DefaultUserAgent)
}

// SessionID returns a stable ses_ identifier for the provider process.
// OPENCODE_SESSION_ID allows multiple server processes to share affinity.
func SessionID() string {
	if value := os.Getenv("OPENCODE_SESSION_ID"); value != "" {
		return value
	}
	sessionOnce.Do(func() {
		processID = "ses_" + uuid.NewString()
	})
	return processID
}

func endpoint(baseURL, suffix string) string {
	return strings.TrimRight(baseURL, "/") + "/" + strings.TrimLeft(suffix, "/")
}

// ModelsURL returns the Zen model-list endpoint.
func ModelsURL(baseURL string) string {
	if baseURL == "" {
		baseURL = BaseURL()
	}
	return endpoint(baseURL, "models")
}

// ChatCompletionsURL returns the Chat Completions endpoint.
func ChatCompletionsURL(baseURL string) string {
	if baseURL == "" {
		baseURL = BaseURL()
	}
	return endpoint(baseURL, "chat/completions")
}

// ResponsesURL returns the Responses endpoint.
func ResponsesURL(baseURL string) string {
	if baseURL == "" {
		baseURL = BaseURL()
	}
	return endpoint(baseURL, "responses")
}

// IsResponsesModel preserves the TypeScript provider's model-family split.
func IsResponsesModel(modelID string) bool {
	return strings.HasPrefix(modelID, "muse-") ||
		strings.HasPrefix(modelID, "gpt-5") ||
		strings.HasPrefix(modelID, "grok-")
}

// SetCommonHeaders applies the public Zen identity used by model discovery
// and inference. The compatibility gate is isolated here so it can be updated
// without changing the agent loop.
func SetCommonHeaders(req *http.Request, accept string) {
	req.Header.Set("Authorization", "Bearer public")
	req.Header.Set("User-Agent", UserAgent())
	req.Header.Set("x-opencode-client", "cli")
	req.Header.Set("x-opencode-session", SessionID())
	if accept != "" {
		req.Header.Set("Accept", accept)
	}
}
