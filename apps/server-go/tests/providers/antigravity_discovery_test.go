// Antigravity live discovery cache: the endpoint is the only source of
// truth (no static seed), refreshed at most hourly.
package tests

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/providers"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/providers/antigravity"
)

func antigravityDiscoveryServer(t *testing.T, hits *atomic.Int64) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		if r.URL.Path != "/v1internal:fetchAvailableModels" {
			t.Errorf("path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"models":{
			"gemini-2.5-pro": {"modelProvider":"MODEL_PROVIDER_GOOGLE","maxTokens":1048576,"supportsImages":true},
			"gemini-3.8-flash-tiered": {"modelProvider":"MODEL_PROVIDER_GOOGLE","maxTokens":1048576},
			"claude-opus-4-6": {"modelProvider":"MODEL_PROVIDER_ANTHROPIC","maxTokens":250000},
			"mystery-model": {"modelProvider":"MODEL_PROVIDER_GOOGLE"},
			"chat_20706": {"modelProvider":"MODEL_PROVIDER_GOOGLE","maxTokens":999},
			"tab_flash_lite_preview": {"modelProvider":"MODEL_PROVIDER_GOOGLE","maxTokens":999,"isInternal":true}
		}}`)
	}))
}

func useAntigravityDiscovery(t *testing.T, srv *httptest.Server) {
	t.Helper()
	antigravity.InvalidateModelCache()
	t.Cleanup(antigravity.InvalidateModelCache)
	dir := t.TempDir()
	credPath := filepath.Join(dir, "ag-creds.json")
	if err := os.WriteFile(credPath, []byte(`{"token":"tok","projectId":"p"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("ANTIGRAVITY_CREDENTIALS_PATH", credPath)
	prev := antigravity.DiscoveryBaseURL
	antigravity.DiscoveryBaseURL = srv.URL
	t.Cleanup(func() { antigravity.DiscoveryBaseURL = prev })
}

func TestAntigravityDiscoveryMapsWindows(t *testing.T) {
	var hits atomic.Int64
	srv := antigravityDiscoveryServer(t, &hits)
	defer srv.Close()
	useAntigravityDiscovery(t, srv)

	models := providers.AntigravityModels(context.Background())
	byID := map[string]int{}
	for i, m := range models {
		byID[m.ID] = i
	}
	// maxTokens maps through; missing maxTokens falls back to 200k;
	// denylisted and internal models are excluded.
	for id, want := range map[string]int{
		"gemini-2.5-pro": 1048576, "gemini-3.8-flash-tiered": 1048576,
		"claude-opus-4-6": 250000, "mystery-model": 200000,
	} {
		i, ok := byID[id]
		if !ok {
			t.Fatalf("missing %s: %+v", id, models)
		}
		if models[i].ContextWindow != want || models[i].Provider != "antigravity" {
			t.Fatalf("%s: %+v", id, models[i])
		}
	}
	for _, id := range []string{"chat_20706", "tab_flash_lite_preview"} {
		if _, ok := byID[id]; ok {
			t.Fatalf("excluded model present: %s", id)
		}
	}
	if !models[byID["gemini-2.5-pro"]].SupportsImages {
		t.Fatalf("images not mapped: %+v", models[byID["gemini-2.5-pro"]])
	}
	if hits.Load() != 1 {
		t.Fatalf("hits = %d; want 1", hits.Load())
	}

	// Second call serves the cache: no second fetch.
	again := providers.AntigravityModels(context.Background())
	if len(again) != len(models) || hits.Load() != 1 {
		t.Fatalf("cache miss: %d models, %d hits", len(again), hits.Load())
	}
}

func TestAntigravityResolveModel(t *testing.T) {
	var hits atomic.Int64
	srv := antigravityDiscoveryServer(t, &hits)
	defer srv.Close()
	useAntigravityDiscovery(t, srv)

	// Case-insensitive hit with the discovered window — the tiered id that
	// used to fall back to the 128k synthetic default.
	found, ok := antigravity.ResolveModel(context.Background(), "GEMINI-3.8-FLASH-TIERED")
	if !ok || found.ContextWindow != 1048576 || found.Provider != "antigravity" {
		t.Fatalf("resolve: %+v %v", found, ok)
	}
	// Seed path agrees once the snapshot is warm.
	if found, ok := providers.FindModel("antigravity", "gemini-3.8-flash-tiered"); !ok || found.ContextWindow != 1048576 {
		t.Fatalf("find: %+v %v", found, ok)
	}
	if _, ok := antigravity.ResolveModel(context.Background(), "nope"); ok {
		t.Fatal("unknown id must miss")
	}
}

func TestAntigravityLoggedOutIsEmpty(t *testing.T) {
	antigravity.InvalidateModelCache()
	t.Cleanup(antigravity.InvalidateModelCache)
	t.Setenv("ANTIGRAVITY_CREDENTIALS_PATH", filepath.Join(t.TempDir(), "missing.json"))
	if models := providers.AntigravityModels(context.Background()); len(models) != 0 {
		t.Fatalf("logged out: %+v", models)
	}
}

// An empty cache must serialize as [] not null: the desktop decodes the
// catalog's models as a plain list, and null used to fail the entire
// response and blank every provider list.
func TestAntigravitySnapshotSerializesEmpty(t *testing.T) {
	antigravity.InvalidateModelCache()
	t.Cleanup(antigravity.InvalidateModelCache)
	raw, err := json.Marshal(antigravity.Snapshot())
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != "[]" {
		t.Fatalf("snapshot serializes as %s; want []", raw)
	}
}
