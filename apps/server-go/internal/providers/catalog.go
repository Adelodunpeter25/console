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
			Name: "opencode", DisplayName: "OpenCode Go",
			Description: "Subscription models through the OpenCode Go API",
			// "api-key", not "none": both clients hide providers whose
			// auth_method is "none", which would make the field unreachable.
			//
			// Models is deliberately empty. Go's lineup is account-scoped and
			// changes often, so a static list here would drift immediately; the
			// pickers read /api/providers/opencode/models instead, and
			// ResolveLiveModel covers individual lookups. Note the key is now
			// required rather than optional — Go has no anonymous tier.
			Models: nil, AuthMethod: "api-key",
		},
	}
}

// SupportsAPIKey reports whether a catalog provider accepts a pasted key, and
// so may be targeted by PUT/DELETE /api/providers/:id/credential.
//
// Derived from the catalog rather than a hand-kept list, so a provider cannot
// be added in one place and forgotten in the other.
func SupportsAPIKey(provider string) bool {
	for _, entry := range ListProviders() {
		if entry.Name == provider && entry.AuthMethod == AuthMethodAPIKey {
			return true
		}
	}
	return false
}

// AuthMethodAPIKey marks a provider whose credential is a key the owner pastes
// once, rather than an OAuth session this server renews on its own.
const AuthMethodAPIKey = "api-key"

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

// OpenCodeModels returns the live Go model list.
//
// Unlike the other providers there is no offline fallback: the previous one was
// a hardcoded free-tier model, and since Go has no free tier a fallback could
// only be a lie. An unconfigured or unreachable provider now surfaces its own
// error instead of quietly advertising a model that cannot be called.
func OpenCodeModels(ctx context.Context) ([]types.Model, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	return opencode.FetchModels(ctx, nil, "")
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
	case "opencode":
		// Without this, any Go model not in the (now empty) static catalog
		// would fall through to the 128k synthetic entry in
		// run.resolveModel — understating a 1M window by 8x and offering no
		// effort levels.
		return opencode.ResolveModel(ctx, modelID)
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
