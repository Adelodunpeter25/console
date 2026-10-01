// Antigravity model discovery. Port of the antigravity branch in
// apps/server/agent/src/commands/provider-registry.ts (seed) and
// apps/server/providers/src/discovery/fetch-models.ts (live list).
package antigravity

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/types"
)

// modelCacheTTL: model releases are infrequent, so an hour keeps the
// picker, run resolution, and context math fresh without per-request
// fetches. There is intentionally no static seed — the live endpoint is
// the only source of truth.
const modelCacheTTL = time.Hour

var (
	modelCacheMu sync.Mutex
	modelCacheAt time.Time
	modelCache   []types.Model
)

// DiscoveryBaseURL overrides the models endpoint (tests).
var DiscoveryBaseURL = ""

// InvalidateModelCache drops the discovery cache (tests; called on logout
// paths that rotate credentials).
func InvalidateModelCache() {
	modelCacheMu.Lock()
	defer modelCacheMu.Unlock()
	modelCacheAt = time.Time{}
	modelCache = nil
}

// Snapshot returns the last discovered list without fetching. Empty until
// the first successful discovery — logged-out callers see no models.
func Snapshot() []types.Model {
	modelCacheMu.Lock()
	defer modelCacheMu.Unlock()
	return append([]types.Model(nil), modelCache...)
}

// CachedModels returns the cached discovery, refreshing when stale or
// missing. Nil when logged out or when the fetch fails with nothing cached.
func CachedModels(ctx context.Context) []types.Model {
	modelCacheMu.Lock()
	fresh := len(modelCache) > 0 && time.Since(modelCacheAt) < modelCacheTTL
	if fresh {
		out := append([]types.Model(nil), modelCache...)
		modelCacheMu.Unlock()
		return out
	}
	modelCacheMu.Unlock()

	cred, err := LoadCredential()
	if err != nil {
		return Snapshot()
	}
	if refreshed, err := RefreshIfNeeded(nil, cred); err == nil {
		cred = refreshed
	}
	base := DiscoveryBaseURL
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	discovered, err := FetchModels(ctx, nil, base, cred.AccessToken)
	if err != nil || len(discovered) == 0 {
		return Snapshot()
	}
	modelCacheMu.Lock()
	modelCache, modelCacheAt = discovered, time.Now()
	out := append([]types.Model(nil), modelCache...)
	modelCacheMu.Unlock()
	return out
}

// ResolveModel finds id in discovery, fetching once on a cold cache.
// Case-insensitive like providers.FindModel.
func ResolveModel(ctx context.Context, id string) (types.Model, bool) {
	for _, m := range Snapshot() {
		if strings.EqualFold(m.ID, id) {
			return m, true
		}
	}
	for _, m := range CachedModels(ctx) {
		if strings.EqualFold(m.ID, id) {
			return m, true
		}
	}
	return types.Model{}, false
}

var discoveryDenylist = map[string]bool{"chat_20706": true, "chat_23310": true}

type discoveredAPIModel struct {
	SupportsImages   *bool `json:"supportsImages"`
	SupportsThinking *bool `json:"supportsThinking"`
	MaxTokens        *int  `json:"maxTokens"`
	IsInternal       *bool `json:"isInternal"`
}

// FetchModels lists Antigravity models via /v1internal:fetchAvailableModels.
// Returns nil slice on failure so callers fall back to the seed.
func FetchModels(ctx context.Context, client *http.Client, baseURL, accessToken string) ([]types.Model, error) {
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	if baseURL == "" {
		baseURL = BaseURL
	}
	url := strings.TrimRight(baseURL, "/") + "/v1internal:fetchAvailableModels"
	req, err := http.NewRequestWithContext(ctx, "POST", url, strings.NewReader("{}"))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", UserAgent())

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, nil
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return nil, err
	}
	var payload struct {
		Models map[string]discoveredAPIModel `json:"models"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil || payload.Models == nil {
		return nil, nil
	}

	var out []types.Model
	for id, meta := range payload.Models {
		if discoveryDenylist[id] {
			continue
		}
		if meta.IsInternal != nil && *meta.IsInternal {
			continue
		}
		contextWindow := 200_000
		if meta.MaxTokens != nil && *meta.MaxTokens > 0 {
			contextWindow = *meta.MaxTokens
		}
		supportsThinking := strings.HasPrefix(id, "gemini-")
		if meta.SupportsThinking != nil {
			supportsThinking = *meta.SupportsThinking
		}
		model := types.Model{ID: id, Provider: "antigravity", ContextWindow: contextWindow}
		if meta.SupportsImages != nil {
			model.SupportsImages = *meta.SupportsImages
		}
		if supportsThinking {
			model.ThinkingLevels = GeminiThinkingLevels
			model.DefaultThinking = "low"
		}
		out = append(out, model)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}
