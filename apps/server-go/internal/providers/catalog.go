// Provider + model catalog: static seeds, provider listing, and dynamic
// model refresh with favorites-first sorting. Only implemented providers are listed.
package providers

import (
	"context"
	"sort"
	"strings"
	"time"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/providers/antigravity"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/providers/claude"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/providers/codex"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/providers/opencode"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/types"
)

// ListProviders returns catalog entries for implemented providers.
func ListProviders() []types.ProviderEntry {
	return []types.ProviderEntry{
		{
			Name: "antigravity", DisplayName: "Google Antigravity",
			Description: "Daily Cloud Code Assist endpoint with Antigravity session envelope",
			Models:      antigravity.Snapshot(), AuthMethod: "oauth",
		},
		{
			Name: "codex", DisplayName: "Codex",
			Description: "ChatGPT subscription models through the Codex Responses API",
			Models:      codex.Snapshot(), AuthMethod: "oauth",
		},
		{
			Name: "claude", DisplayName: "Claude",
			Description: "Anthropic subscription models through the Messages API",
			Models:      claude.Snapshot(), AuthMethod: "oauth",
		},
		{
			Name: "opencode", DisplayName: "OpenCode Zen",
			Description: "Free-tier models through the direct OpenCode Zen API",
			Models:      DefaultOpenCodeModels(), AuthMethod: "none",
		},
	}
}

// AntigravityModels returns live Antigravity models from the hourly
// discovery cache, or nil when logged out / undiscovered. There is no
// static seed: the endpoint is the only source of truth.
func AntigravityModels(ctx context.Context) []types.Model {
	return antigravity.CachedModels(ctx)
}

// CodexModels returns live Codex models from the hourly discovery cache,
// or empty when logged out / undiscovered. There is no static seed: the
// endpoint is the only source of truth.
func CodexModels(ctx context.Context) []types.Model {
	return codex.CachedModels(ctx)
}

// ClaudeModels returns live Claude models from the hourly discovery cache,
// or empty when logged out / undiscovered. There is no static seed: the
// endpoint is the only source of truth.
func ClaudeModels(ctx context.Context) []types.Model {
	return claude.CachedModels(ctx)
}

// DefaultOpenCodeModels returns the single offline fallback model.
func DefaultOpenCodeModels() []types.Model {
	return opencode.DefaultModels()
}

// OpenCodeModels returns the live Zen model list, falling back only to the
// stealth model when discovery is unavailable.
func OpenCodeModels(ctx context.Context) []types.Model {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	if discovered, err := opencode.FetchModels(ctx, nil, ""); err == nil && len(discovered) > 0 {
		return discovered
	}
	return DefaultOpenCodeModels()
}

// SortModelsByFavorites moves favorited models first (stable).
func SortModelsByFavorites(models []types.Model, favs []types.ModelFavorite) []types.Model {
	if len(favs) == 0 {
		return models
	}
	favSet := make(map[string]bool, len(favs))
	for _, f := range favs {
		favSet[f.Provider+":"+f.ModelID] = true
	}
	out := append([]types.Model(nil), models...)
	sort.SliceStable(out, func(i, j int) bool {
		aFav := favSet[out[i].Provider+":"+out[i].ID]
		bFav := favSet[out[j].Provider+":"+out[j].ID]
		return aFav && !bFav
	})
	return out
}

// FindModel looks up a model by provider and id (case-insensitive),
// mirroring findModelInProvider. For antigravity this covers the
// live-discovery snapshot served via ListProviders (no fetch — callers
// needing a cold lookup use antigravity.ResolveModel with a context).
func FindModel(providerID, modelID string) (types.Model, bool) {
	for _, entry := range ListProviders() {
		if entry.Name != providerID {
			continue
		}
		for _, m := range entry.Models {
			if strings.EqualFold(m.ID, modelID) {
				return m, true
			}
		}
	}
	return types.Model{}, false
}

// ResolveLiveModel finds id in a provider's live-discovery cache, fetching
// once on a cold cache. Used by run resolution so the compaction threshold
// and context math track real windows instead of the 128k synthetic
// fallback. Providers without live discovery miss.
func ResolveLiveModel(ctx context.Context, providerID, modelID string) (types.Model, bool) {
	switch providerID {
	case "antigravity":
		return antigravity.ResolveModel(ctx, modelID)
	case "claude":
		return claude.ResolveModel(ctx, modelID)
	case "codex":
		return codex.ResolveModel(ctx, modelID)
	default:
		return types.Model{}, false
	}
}

// IsCatalogProvider reports whether id is a known catalog provider id
// (implemented or not).
func IsCatalogProvider(id string) bool {
	switch id {
	case "antigravity", "codex", "claude", "opencode":
		return true
	default:
		return false
	}
}
