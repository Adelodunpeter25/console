// Provider + model catalog. Port of the catalog slice of
// apps/server/agent/src/commands/provider-registry.ts: static seeds,
// provider listing, and dynamic model refresh with favorites-first
// sorting. Only implemented providers are listed (codex first).
package providers

import (
	"context"
	"sort"
	"strings"
	"time"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/providers/codex"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/types"
)

// DefaultCodexModels mirrors DEFAULT_CODEX_MODELS: offline seed refreshed
// from the live list after login.
func DefaultCodexModels() []types.Model {
	ids := []string{"gpt-5.6-terra", "gpt-5.6-luna", "gpt-5.5", "gpt-5.4-mini"}
	models := make([]types.Model, 0, len(ids))
	for _, id := range ids {
		models = append(models, types.Model{
			ID: id, Provider: "codex", ContextWindow: 272_000, SupportsImages: true,
			ThinkingLevels: codex.CodexThinkingLevels, DefaultThinking: "low",
		})
	}
	return models
}

// ListProviders returns catalog entries for implemented providers.
func ListProviders() []types.ProviderEntry {
	return []types.ProviderEntry{{
		Name: "codex", DisplayName: "OpenAI Codex",
		Description: "ChatGPT subscription models through the Codex Responses API",
		Models:      DefaultCodexModels(), AuthMethod: "oauth",
	}}
}

// CodexModels returns live Codex models when logged in, else the static
// seed. Mirrors fetchModelsForProvider's discover-or-fallback rule.
func CodexModels(ctx context.Context) []types.Model {
	cred, err := codex.LoadCredential()
	if err != nil {
		return DefaultCodexModels()
	}
	if refreshed, err := codex.RefreshIfNeeded(nil, cred); err == nil {
		cred = refreshed
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	if discovered, err := codex.FetchModels(ctx, nil, "", cred); err == nil && len(discovered) > 0 {
		return discovered
	}
	return DefaultCodexModels()
}

// SortModelsByFavorites moves favorited models first (stable), mirroring
// ProviderService's sorting in the TS server.
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
// mirroring findModelInProvider.
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

// IsCatalogProvider reports whether id is a known catalog provider id
// (implemented or not).
func IsCatalogProvider(id string) bool {
	switch id {
	case "antigravity", "codex", "claude":
		return true
	default:
		return false
	}
}
