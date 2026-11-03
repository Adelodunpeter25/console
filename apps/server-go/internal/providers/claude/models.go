// Claude model discovery: static seed list plus live list.
package claude

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/types"
)

// DiscoveredModel is the provider-registry discovered shape (alias of the
// shared catalog model type).
type DiscoveredModel = types.Model

// modelCacheTTL: model releases are infrequent, so an hour keeps the
// picker, run resolution, and context math fresh without per-request
// fetches. There is intentionally no static seed — the live endpoint is
// the only source of truth.
const modelCacheTTL = time.Hour

var (
	modelCacheMu sync.Mutex
	modelCacheAt time.Time
	modelCache   []DiscoveredModel
	// Last known list, served while a background refresh re-discovers
	// models for a freshly activated account. Cleared by the hard
	// InvalidateModelCache (logout, tests).
	staleModels []DiscoveredModel
	// Single-flight guard for background re-discovery.
	refreshing bool
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

// copyModels duplicates the cache contents, never returning nil — a nil
// slice would serialize as `models: null` and fail the desktop's whole
// catalog decode, blanking every provider list.
func copyModels(in []DiscoveredModel) []DiscoveredModel {
	out := make([]DiscoveredModel, 0, len(in))
	return append(out, in...)
}

// Snapshot returns the last discovered list without fetching. Empty (never
// nil) until the first successful discovery — logged-out callers see no
// models.
func Snapshot() []DiscoveredModel {
	modelCacheMu.Lock()
	defer modelCacheMu.Unlock()
	return copyModels(modelCache)
}

// CachedModels returns the cached discovery, refreshing when stale or
// missing. With a stale list on hand (recent account switch) it returns
// immediately and re-discovers in the background; only a cold cache blocks.
// Empty (never nil) when logged out or when the fetch fails with nothing
// cached.
func CachedModels(ctx context.Context) []DiscoveredModel {
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
func discover() []DiscoveredModel {
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
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	discovered, err := FetchModels(ctx, nil, DiscoveryBaseURL, cred)
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
func ResolveModel(ctx context.Context, id string) (DiscoveredModel, bool) {
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
	return DiscoveredModel{}, false
}

// FetchModels lists Claude models with the subscription OAuth token.
// Returns nil slice on non-OK status so callers fall back to the seed.
func FetchModels(ctx context.Context, client *http.Client, baseURL string, cred ParsedCredential) ([]DiscoveredModel, error) {
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	req, err := http.NewRequestWithContext(ctx, "GET", ModelsURL(baseURL), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+cred.AccessToken)
	req.Header.Set("anthropic-version", "2023-06-01")
	req.Header.Set("anthropic-beta", "oauth-2025-04-20")
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("Claude models request failed (%d %s)", resp.StatusCode, resp.Status)
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return nil, err
	}
	var payload struct {
		Data []struct {
			ID             string `json:"id"`
			MaxInputTokens *int   `json:"max_input_tokens"`
			Capabilities   *struct {
				ImageInput *struct {
					Supported *bool `json:"supported"`
				} `json:"image_input"`
			} `json:"capabilities"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, fmt.Errorf("decode Claude models: %w", err)
	}
	var out []DiscoveredModel
	for _, entry := range payload.Data {
		if entry.ID == "" {
			continue
		}
		model := DiscoveredModel{
			ID: entry.ID, Provider: "claude",
			ThinkingLevels: ClaudeThinkingLevels, DefaultThinking: "low",
		}
		if entry.MaxInputTokens != nil && *entry.MaxInputTokens > 0 {
			model.ContextWindow = *entry.MaxInputTokens
		}
		if model.ContextWindow <= 0 {
			// Floor at the smallest known Claude window so an entry
			// without metadata never resolves to a zero window.
			model.ContextWindow = 200_000
		}
		if entry.Capabilities != nil && entry.Capabilities.ImageInput != nil &&
			entry.Capabilities.ImageInput.Supported != nil {
			model.SupportsImages = *entry.Capabilities.ImageInput.Supported
		}
		out = append(out, model)
	}
	return out, nil
}
