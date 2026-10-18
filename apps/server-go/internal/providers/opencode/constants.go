// OpenCode Zen provider constants and wire policy.
package opencode

import (
	"errors"
	"net/http"
	"os"
	"strings"
	"sync"

	"github.com/google/uuid"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/providers/apikeys"
)

const (
	// DefaultBaseURL is the OpenCode Go inference root. Go is a subscription
	// plan served from a different path than the free Zen catalogue, and the
	// two are not interchangeable: a key issued for one is not accepted by the
	// other.
	DefaultBaseURL = "https://opencode.ai/inference/go/v1"
	// DefaultUserAgent matches the OpenCode client identity required by Go.
	DefaultUserAgent = "opencode/latest/2.0.15/cli"
	// DefaultContextWindow is a conservative window for models models.dev does
	// not cover yet. It is a floor, not a claim: it keeps an unknown model
	// usable rather than guessing high, and it is only reached for the handful
	// of models upstream ships before the catalogue catches up.
	DefaultContextWindow = 200_000
	// ProviderID is this provider's catalog id, and the key under which its
	// credential is stored. Kept as a constant because the store and the
	// catalog must agree on it.
	ProviderID = "opencode"
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

// IsResponsesModel reports whether modelID belongs to the Responses model
// family.
func IsResponsesModel(modelID string) bool {
	return strings.HasPrefix(modelID, "muse-") ||
		strings.HasPrefix(modelID, "gpt-5") ||
		strings.HasPrefix(modelID, "grok-")
}

// ErrNoAPIKey is returned when no credential is configured. Go has no anonymous
// tier to fall back to, so this is a hard stop rather than a degraded mode: the
// alternative — inventing an identity — would silently send the user's traffic
// to a plan they did not choose.
var ErrNoAPIKey = errors.New("OpenCode Go needs an API key. Add one in Settings > Providers.")

// ThinkingLevelsFor reports the effort levels an OpenCode model accepts.
//
// Reads the models.dev index rather than a curated table. See metadata.go for why
// the table went away. Returns nil for models with no tunable reasoning — and
// for models the index does not cover — which callers treat as "unsupported"
// and fall back to the provider default.
func ThinkingLevelsFor(modelID string) []string {
	md, ok := MetadataFor(modelID)
	if !ok {
		return nil
	}
	return md.ThinkingLevels
}

// APIKey resolves the credential to send upstream, in priority order:
//
//  1. OPENCODE_API_KEY — an environment override, so headless and CI setups
//     work with no stored credential at all. Env wins over stored, matching
//     the BaseURL/UserAgent convention in this file.
//  2. A key pasted through PUT /api/providers/opencode/credential. It lives on
//     the server, which is the only party that talks to Go, so a key pasted on
//     any client reaches the same store.
//
// There is deliberately no third case. The provider previously ended in
// "public", Zen's anonymous shared identity, so an unconfigured server quietly
// ran on the free tier — indistinguishable, to the user, from a paid one.
func APIKey() (string, error) {
	if key := strings.TrimSpace(os.Getenv("OPENCODE_API_KEY")); key != "" {
		return key, nil
	}
	if key, ok := apikeys.Get(ProviderID); ok {
		return key, nil
	}
	return "", ErrNoAPIKey
}

// SetCommonHeaders applies the Go identity used by model discovery and
// inference. The compatibility gate is isolated here so it can be updated
// without changing the agent loop.
//
// Returns ErrNoAPIKey rather than sending an unauthenticated request, so an
// unconfigured server fails at the call site with an actionable message instead
// of producing an opaque 401 from upstream.
func SetCommonHeaders(req *http.Request, accept string) error {
	key, err := APIKey()
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("User-Agent", UserAgent())
	req.Header.Set("x-opencode-client", "cli")
	req.Header.Set("x-opencode-session", SessionID())
	if accept != "" {
		req.Header.Set("Accept", accept)
	}
	return nil
}
