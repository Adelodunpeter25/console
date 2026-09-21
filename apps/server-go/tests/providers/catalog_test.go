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
	if len(entries) != 3 {
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
	if len(codexEntry.Models) != 4 || codexEntry.AuthMethod != "oauth" {
		t.Fatalf("codex entry: %+v", codexEntry)
	}
	for _, m := range codexEntry.Models {
		if m.ContextWindow != 272_000 || !m.SupportsImages || m.DefaultThinking != "low" {
			t.Fatalf("seed model: %+v", m)
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
	if entry.AuthMethod != "oauth" || len(entry.Models) != 8 {
		t.Fatalf("antigravity entry: %+v", entry)
	}
}

func TestCatalogModelsFallbackLoggedOut(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CODEX_CREDENTIALS_PATH", filepath.Join(dir, "missing.json"))
	t.Setenv("OPENAI_CODEX_OAUTH_TOKEN", "")
	models := providers.CodexModels(context.Background())
	if len(models) != 4 {
		t.Fatalf("logged-out models: %+v", models)
	}
}

func TestSortModelsByFavorites(t *testing.T) {
	models := providers.DefaultCodexModels()
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
	sorted := providers.SortModelsByFavorites(providers.DefaultCodexModels(), list)
	if sorted[0].ID != "gpt-5.4-mini" {
		t.Fatalf("stored fav first: %+v", sorted)
	}
}
