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

// Zen publishes no reasoning metadata: GET /zen/v1/models returns only id,
// object, created and owned_by for every model. Thinking support therefore has
// to be a curated table rather than discovery-driven, mirroring
// codex.CodexThinkingLevels and claude.ClaudeThinkingLevels.
//
// Level sets are the effort vocabularies the upstream models accept per
// models.dev. Only free models are cataloged here, so the table covers the
// Zen free lineup only; unknown ids deliberately resolve to no levels and keep
// the existing "provider default" behaviour.
var (
	// museThinkingLevels covers Muse Spark 1.2 and 1.3, which share the same
	// five-step effort vocabulary.
	museThinkingLevels = []string{"minimal", "low", "medium", "high", "xhigh"}
	// deepSeekThinkingLevels covers the DeepSeek V4 free lane, which offers
	// effort levels on top of its native hybrid thinking toggle.
	deepSeekThinkingLevels = []string{"low", "medium", "high", "xhigh"}
	// spaceBunnyThinkingLevels is the Stealth model's own vocabulary. It runs
	// up to "max" and has no "minimal" step, so it cannot share the Muse Spark
	// set even though both models expose five levels.
	spaceBunnyThinkingLevels = []string{"low", "medium", "high", "xhigh", "max"}
)

// defaultThinkingLevel is the effort used when a level-capable model receives
// no explicit level. "medium" is the midpoint of every declared vocabulary, so
// it is always a member of the level set it defaults for.
const defaultThinkingLevel = "medium"

// ThinkingLevelsFor reports the effort levels an OpenCode model accepts. It
// returns nil for models with no tunable reasoning, which callers treat as
// "unsupported" and fall back to the provider default.
func ThinkingLevelsFor(modelID string) []string {
	switch {
	case strings.HasPrefix(modelID, "muse-"):
		return museThinkingLevels
	case strings.HasPrefix(modelID, "deepseek-"):
		return deepSeekThinkingLevels
	case modelID == "space-bunny-free":
		return spaceBunnyThinkingLevels
	}
	return nil
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
