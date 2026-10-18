// Model capability metadata sourced from models.dev.
//
// This is the replacement for a hand-written effort table that covered 3 of
// Go's 37 models and had the vocabulary wrong for one of those 3.
package tests

import (
	"net/http"
	"net/http/httptest"
	"slices"
	"sync/atomic"
	"testing"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/providers/opencode"
)

// modelsDevStub serves a trimmed catalogue with the same shape as the real
// document: several providers, of which only one is ours.
const modelsDevStub = `{
  "some-other-provider": {
    "models": {
      "decoy-model": {"id":"decoy-model","limit":{"context":9999999},"modalities":{"input":["image"]}}
    }
  },
  "opencode-go": {
    "models": {
      "kimi-k3": {
        "id": "kimi-k3",
        "name": "Kimi K3",
        "attachment": true,
        "reasoning": true,
        "reasoning_options": [{"type":"effort","values":["low","high","max"]}],
        "modalities": {"input":["text","image"], "output":["text"]},
        "limit": {"context": 262144, "output": 65536}
      },
      "glm-5.3": {
        "id": "glm-5.3",
        "name": "GLM 5.3",
        "reasoning": false,
        "modalities": {"input":["text"]},
        "limit": {"context": 204800}
      },
      "toggle-only": {
        "id": "toggle-only",
        "reasoning": true,
        "reasoning_options": [{"type":"enabled","values":[true]}],
        "limit": {"context": 128000}
      },
      "no-limit": {"id":"no-limit","reasoning": false}
    }
  },
  "opencode": {
    "models": {
      "free-zen-model": {"id":"free-zen-model","limit":{"context":128000}}
    }
  }
}`

// useModelsDevStub points the catalogue lookup at a stub for one test and
// restores it afterwards.
func useModelsDevStub(t *testing.T, handler http.HandlerFunc) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(handler)
	previous := opencode.ModelsDevEndpoint
	opencode.ModelsDevEndpoint = server.URL
	t.Cleanup(func() {
		opencode.ModelsDevEndpoint = previous
		// Drop whatever the test warmed, so ordering cannot leak between tests.
		opencode.UseMetadata(nil)
	})
	return server
}

func TestMetadataMapsUpstreamCapabilities(t *testing.T) {
	useModelsDevStub(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(modelsDevStub))
	})

	opencode.EnsureMetadata(t.Context(), nil)

	kimi, ok := opencode.MetadataFor("kimi-k3")
	if !ok {
		t.Fatal("kimi-k3 should be catalogued")
	}
	if kimi.ContextWindow != 262144 {
		t.Fatalf("context: %d", kimi.ContextWindow)
	}
	if !kimi.SupportsImages {
		t.Fatal("kimi-k3 accepts image input")
	}
	if !slices.Equal(kimi.ThinkingLevels, []string{"low", "high", "max"}) {
		t.Fatalf("levels: %#v", kimi.ThinkingLevels)
	}
	if kimi.DisplayName != "Kimi K3" {
		t.Fatalf("name: %q", kimi.DisplayName)
	}

	glm, ok := opencode.MetadataFor("glm-5.3")
	if !ok {
		t.Fatal("glm-5.3 should be catalogued")
	}
	if glm.SupportsImages {
		t.Fatal("glm-5.3 is text-only in the stub")
	}
	// reasoning:false must yield no picker, not the default vocabulary.
	if glm.ThinkingLevels != nil {
		t.Fatalf("non-reasoning model must offer no levels: %#v", glm.ThinkingLevels)
	}

	// Reasoning that is a boolean toggle, not an effort scale, gets no picker
	// either: offering levels there would invent a control that does not exist.
	if toggle, _ := opencode.MetadataFor("toggle-only"); toggle.ThinkingLevels != nil {
		t.Fatalf("toggle-only must offer no levels: %#v", toggle.ThinkingLevels)
	}

	// Only the opencode-go entry is read. The Zen provider and the decoy must
	// not leak in — they are different catalogues with different models.
	for _, id := range []string{"free-zen-model", "decoy-model"} {
		if _, ok := opencode.MetadataFor(id); ok {
			t.Fatalf("%s is not a Go model and must not be indexed", id)
		}
	}
}

func TestMetadataSurvivesAnEntryWithNoLimit(t *testing.T) {
	useModelsDevStub(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(modelsDevStub))
	})
	opencode.EnsureMetadata(t.Context(), nil)

	entry, ok := opencode.MetadataFor("no-limit")
	if !ok {
		t.Fatal("no-limit should still be indexed")
	}
	// A zero here would propagate into NewModel, where it is guarded — but the
	// index itself should still be truthy about what it read.
	if entry.ContextWindow != 0 {
		t.Fatalf("expected an unpublished limit to read as zero, got %d", entry.ContextWindow)
	}

	model := opencode.NewModel("no-limit")
	if model.ContextWindow != opencode.DefaultContextWindow {
		t.Fatalf("unpublished limit must fall back, got %d", model.ContextWindow)
	}
}

func TestMetadataRequestNeverCarriesTheProviderKey(t *testing.T) {
	// Security: models.dev is a third party. The provider's API key is sent to
	// opencode.ai and nowhere else.
	t.Setenv("OPENCODE_API_KEY", "oc_sk_super_secret_value")

	var sawAuth, sawSession atomic.Bool
	useModelsDevStub(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "" {
			sawAuth.Store(true)
		}
		// The session header identifies a user's conversation; it is just as
		// inappropriate to forward to a third party.
		if r.Header.Get("x-opencode-session") != "" {
			sawSession.Store(true)
		}
		_, _ = w.Write([]byte(modelsDevStub))
	})

	opencode.EnsureMetadata(t.Context(), nil)

	if sawAuth.Load() {
		t.Fatal("the catalogue request must not carry an Authorization header")
	}
	if sawSession.Load() {
		t.Fatal("the catalogue request must not carry the provider session header")
	}
}

func TestMetadataIsFetchedOncePerTTL(t *testing.T) {
	var calls atomic.Int32
	useModelsDevStub(t, func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		_, _ = w.Write([]byte(modelsDevStub))
	})

	opencode.EnsureMetadata(t.Context(), nil)
	opencode.EnsureMetadata(t.Context(), nil)
	opencode.EnsureMetadata(t.Context(), nil)

	// The model list is fetched on every settings page load; without the TTL a
	// cold 6 MB download would fire per request.
	if got := calls.Load(); got != 1 {
		t.Fatalf("expected 1 catalogue fetch across 3 calls, got %d", got)
	}
}

func TestMetadataFailureDegradesInsteadOfFailing(t *testing.T) {
	useModelsDevStub(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})

	// Must not panic, must not poison the index, and must leave the provider
	// usable: Go's own model list still works without this enrichment.
	opencode.EnsureMetadata(t.Context(), nil)

	if _, ok := opencode.MetadataFor("kimi-k3"); ok {
		t.Fatal("a failed fetch must not fabricate entries")
	}
	model := opencode.NewModel("kimi-k3")
	if model.ContextWindow != opencode.DefaultContextWindow || len(model.ThinkingLevels) != 0 {
		t.Fatalf("expected conservative defaults, got %+v", model)
	}
}

func TestMetadataIgnoresAServerWithoutOurProvider(t *testing.T) {
	// A proxy or a future catalogue rename must not be mistaken for "no
	// metadata", silently degrading every model.
	useModelsDevStub(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"some-other-provider":{"models":{}}}`))
	})

	opencode.EnsureMetadata(t.Context(), nil)

	if _, ok := opencode.MetadataFor("kimi-k3"); ok {
		t.Fatal("nothing should be indexed")
	}
}
