// Codex live discovery cache: the endpoint is the only source of truth
// (no static seed), refreshed at most hourly.
package tests

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/providers"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/providers/codex"
)

func useCodexDiscovery(t *testing.T, srv *httptest.Server) {
	t.Helper()
	codex.InvalidateModelCache()
	t.Cleanup(codex.InvalidateModelCache)
	dir := t.TempDir()
	credPath := filepath.Join(dir, "codex-creds.json")
	if err := os.WriteFile(credPath, []byte(`{"access_token":"tok","refresh_token":"r","expiresAt":9999999999999,"accountId":"acc-1"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CODEX_CREDENTIALS_PATH", credPath)
	t.Setenv("OPENAI_CODEX_OAUTH_TOKEN", "")
	prev := codex.DiscoveryBaseURL
	codex.DiscoveryBaseURL = srv.URL
	t.Cleanup(func() { codex.DiscoveryBaseURL = prev })
}

func TestCodexDiscoveryMapsWindows(t *testing.T) {
	var hits atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		if !strings.HasPrefix(r.URL.Path, "/codex/models") {
			t.Errorf("path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"models":[
			{"slug":"gpt-5.6-luna","context_window":400000,"input_modalities":["text","image"]},
			{"slug":"gpt-5.5","input_modalities":["text"]},
			{"id":"legacy-model","context_window":100000},
			{"slug":"","id":""}
		]}`)
	}))
	defer srv.Close()
	useCodexDiscovery(t, srv)

	models := providers.CodexModels(context.Background())
	byID := map[string]int{}
	for i, m := range models {
		byID[m.ID] = i
	}
	if len(models) != 3 {
		t.Fatalf("models: %+v", models)
	}
	luna := models[byID["gpt-5.6-luna"]]
	if luna.ContextWindow != 400000 || !luna.SupportsImages || luna.Provider != "codex" {
		t.Fatalf("luna: %+v", luna)
	}
	// Missing context_window falls back to the 272k default; image support
	// comes from input modalities only.
	five := models[byID["gpt-5.5"]]
	if five.ContextWindow != 272000 || five.SupportsImages {
		t.Fatalf("gpt-5.5: %+v", five)
	}
	if legacy := models[byID["legacy-model"]]; legacy.ContextWindow != 100000 {
		t.Fatalf("id fallback: %+v", legacy)
	}
	if hits.Load() != 1 {
		t.Fatalf("hits = %d; want 1", hits.Load())
	}

	// Second call serves the cache: no second fetch.
	again := providers.CodexModels(context.Background())
	if len(again) != len(models) || hits.Load() != 1 {
		t.Fatalf("cache miss: %d models, %d hits", len(again), hits.Load())
	}

	// Case-insensitive hit with the discovered window.
	found, ok := codex.ResolveModel(context.Background(), "GPT-5.6-LUNA")
	if !ok || found.ContextWindow != 400000 {
		t.Fatalf("resolve: %+v %v", found, ok)
	}
	if _, ok := codex.ResolveModel(context.Background(), "nope"); ok {
		t.Fatal("unknown id must miss")
	}

	// Seed path agrees once the snapshot is warm.
	if found, ok := providers.FindModel("codex", "gpt-5.6-luna"); !ok || found.ContextWindow != 400000 {
		t.Fatalf("find: %+v %v", found, ok)
	}
}

func TestCodexLoggedOutIsEmpty(t *testing.T) {
	codex.InvalidateModelCache()
	t.Cleanup(codex.InvalidateModelCache)
	t.Setenv("CODEX_CREDENTIALS_PATH", filepath.Join(t.TempDir(), "missing.json"))
	t.Setenv("OPENAI_CODEX_OAUTH_TOKEN", "")
	if models := providers.CodexModels(context.Background()); len(models) != 0 {
		t.Fatalf("logged out: %+v", models)
	}
}
