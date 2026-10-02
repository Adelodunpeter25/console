// Provider catalog coverage: static seeds, live-or-seed refresh, and
// favorites-first sorting.
package tests

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/db"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/providers"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/services"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/types"
)

func TestCatalogListsCodex(t *testing.T) {
	entries := providers.ListProviders()
	if len(entries) != 4 {
		t.Fatalf("entries: %+v", entries)
	}
	var codexEntry *types.ProviderEntry
	for i := range entries {
		if entries[i].Name == "codex" {
			codexEntry = &entries[i]
		}
	}
	if codexEntry == nil {
		t.Fatalf("no codex entry: %+v", entries)
	}
	if codexEntry.AuthMethod != "oauth" {
		t.Fatalf("codex entry: %+v", codexEntry)
	}
	// No static seed: without discovery the list is empty (logged out).
	if len(codexEntry.Models) != 0 {
		t.Fatalf("codex models without discovery: %+v", codexEntry.Models)
	}
}

func TestCatalogListsOpenCode(t *testing.T) {
	entries := providers.ListProviders()
	var entry *types.ProviderEntry
	for i := range entries {
		if entries[i].Name == "opencode" {
			entry = &entries[i]
		}
	}
	if entry == nil {
		t.Fatalf("no opencode entry: %+v", entries)
	}
	if entry.AuthMethod != "none" || len(entry.Models) != 1 || entry.Models[0].ID != "space-bunny-free" {
		t.Fatalf("opencode fallback: %+v", entry)
	}
	if !providers.IsCatalogProvider("opencode") {
		t.Fatal("opencode must be a catalog provider")
	}
	for _, model := range entry.Models {
		if model.Provider != "opencode" || model.ContextWindow != 1_000_000 || !model.SupportsImages {
			t.Fatalf("opencode model: %+v", model)
		}
	}
}

func TestCatalogListsAntigravity(t *testing.T) {
	entries := providers.ListProviders()
	var entry *types.ProviderEntry
	for i := range entries {
		if entries[i].Name == "antigravity" {
			entry = &entries[i]
		}
	}
	if entry == nil {
		t.Fatalf("no antigravity entry: %+v", entries)
	}
	if entry.AuthMethod != "oauth" {
		t.Fatalf("antigravity entry: %+v", entry)
	}
	// No static seed: without discovery the list is empty (logged out).
	if len(entry.Models) != 0 {
		t.Fatalf("antigravity models without discovery: %+v", entry.Models)
	}
}

func TestCatalogModelsFallbackLoggedOut(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CODEX_CREDENTIALS_PATH", filepath.Join(dir, "missing.json"))
	t.Setenv("OPENAI_CODEX_OAUTH_TOKEN", "")
	models := providers.CodexModels(context.Background())
	if len(models) != 0 {
		t.Fatalf("logged-out models: %+v", models)
	}
}

func codexFavFixture() []types.Model {
	return []types.Model{
		{ID: "gpt-5.6-terra", Provider: "codex"},
		{ID: "gpt-5.6-luna", Provider: "codex"},
		{ID: "gpt-5.5", Provider: "codex"},
		{ID: "gpt-5.4-mini", Provider: "codex"},
	}
}

func TestSortModelsByFavorites(t *testing.T) {
	models := codexFavFixture()
	sorted := providers.SortModelsByFavorites(models, nil)
	if len(sorted) != 4 || sorted[0].ID != "gpt-5.6-terra" {
		t.Fatalf("no-fav order: %+v", sorted)
	}
	favs := []types.ModelFavorite{{Provider: "codex", ModelID: "gpt-5.5"}}
	sorted = providers.SortModelsByFavorites(models, favs)
	if sorted[0].ID != "gpt-5.5" {
		t.Fatalf("fav first: %+v", sorted)
	}
	// Input slice must not be mutated.
	if models[0].ID != "gpt-5.6-terra" {
		t.Fatalf("input mutated: %+v", models)
	}
}

func TestCatalogFavoritesEndToEnd(t *testing.T) {
	manager, err := db.Open(db.OpenOptions{Path: ":memory:"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(manager.Close)
	favorites := services.NewFavoriteService(manager)
	if err := favorites.Set(types.ModelFavorite{Provider: "codex", ModelID: "gpt-5.4-mini"}, true); err != nil {
		t.Fatal(err)
	}
	list, err := favorites.List()
	if err != nil {
		t.Fatal(err)
	}
	sorted := providers.SortModelsByFavorites(codexFavFixture(), list)
	if sorted[0].ID != "gpt-5.4-mini" {
		t.Fatalf("stored fav first: %+v", sorted)
	}
}
