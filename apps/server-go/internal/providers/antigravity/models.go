// Antigravity model discovery: static seed list plus live list.
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
	// Last known list, served while a background refresh re-discovers
	// models for a freshly activated account. Cleared by the hard
	// InvalidateModelCache (logout, tests).
	staleModels []types.Model
	// Single-flight guard for background re-discovery.
	refreshing bool
)

// DiscoveryBaseURL overrides the models endpoint (tests).
var DiscoveryBaseURL = ""

// copyModels duplicates the cache contents, never returning nil — a nil
// slice would serialize as `models: null` and fail the desktop's whole
// catalog decode, blanking every provider list.
func copyModels(in []types.Model) []types.Model {
	out := make([]types.Model, 0, len(in))
	return append(out, in...)
}

// InvalidateModelCache drops the discovery cache (tests; called on logout
// paths that rotate credentials).
func InvalidateModelCache() {
	modelCacheMu.Lock()
	defer modelCacheMu.Unlock()
	modelCacheAt = time.Time{}
	modelCache = nil
	staleModels = nil
	refreshing = false
}

// MarkModelCacheStale keeps the current list available while revalidating:
// an account switch may change model availability, but serving the old list
// for a moment beats blocking every catalog read on a token refresh plus a
// discovery round-trip. A background fetch replaces it (see CachedModels).
func MarkModelCacheStale() {
	modelCacheMu.Lock()
	defer modelCacheMu.Unlock()
	modelCacheAt = time.Time{}
	if len(modelCache) > 0 {
		staleModels = modelCache
	}
	modelCache = nil
}

// Snapshot returns the last discovered list without fetching. Empty (never
// nil) until the first successful discovery — logged-out callers see no
// models.
func Snapshot() []types.Model {
	modelCacheMu.Lock()
	defer modelCacheMu.Unlock()
	return copyModels(modelCache)
}

// CachedModels returns the cached discovery, refreshing when stale or
// missing. With a stale list on hand (recent account switch) it returns
// immediately and re-discovers in the background; only a cold cache blocks.
// Empty (never nil) when logged out or when the fetch fails with nothing
// cached.
func CachedModels(ctx context.Context) []types.Model {
	modelCacheMu.Lock()
	fresh := len(modelCache) > 0 && time.Since(modelCacheAt) < modelCacheTTL
	stale := copyModels(staleModels)
	busy := refreshing
	modelCacheMu.Unlock()
	if fresh {
		return Snapshot()
	}
	if len(stale) > 0 {
		if !busy {
			go discover()
		}
		return stale
	}
	return discover()
}

// discover fetches models with the currently active credential and stores
// them on success. Single-flight: concurrent callers either join a running
// fetch (served via Snapshot) or skip. Empty on failure — the previous list
// keeps being served as stale.
func discover() []types.Model {
	modelCacheMu.Lock()
	if refreshing {
		modelCacheMu.Unlock()
		return Snapshot()
	}
	refreshing = true
	modelCacheMu.Unlock()
	defer func() {
		modelCacheMu.Lock()
		refreshing = false
		modelCacheMu.Unlock()
	}()

	cred, err := LoadCredential()
	if err != nil {
		return nil
	}
	if refreshed, err := RefreshIfNeeded(nil, cred); err == nil {
		cred = refreshed
	}
	base := DiscoveryBaseURL
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	discovered, err := FetchModels(ctx, nil, base, cred.AccessToken)
	if err != nil || len(discovered) == 0 {
		return nil
	}
	modelCacheMu.Lock()
	modelCache, modelCacheAt = discovered, time.Now()
	staleModels = discovered
	out := copyModels(modelCache)
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
// Returns nil slice on failure; callers fall back to the (possibly empty)
// cache.
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
