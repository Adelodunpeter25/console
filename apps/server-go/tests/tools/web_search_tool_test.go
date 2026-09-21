// Coverage for the webSearch tool: Firecrawl-first with DuckDuckGo/Brave
// fallback, exercised against local httptest doubles (Firecrawl's base URL
// and the DuckDuckGo/Brave endpoints are all overridable for tests).
package tests

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/tools"
)

func TestWebSearchFirecrawlSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/search") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"success":true,"data":[{"title":"Fire Title","url":"https://fire.example.com","markdown":"# Fire Markdown","description":"desc"}]}`))
	}))
	defer srv.Close()
	t.Setenv("FIRECRAWL_BASE_URL", srv.URL)

	args, _ := json.Marshal(map[string]any{"query": "fire markdown", "numResults": 1})
	out, err := tools.WebSearch.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	s := resultText(t, out)
	if !strings.Contains(s, "Fire Title") || !strings.Contains(s, "Fire Markdown") {
		t.Fatalf("output: %v", out)
	}
}

func TestWebSearchFallbackOnRetryableError(t *testing.T) {
	fireCalled := false
	fireSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fireCalled = true
		w.WriteHeader(http.StatusTooManyRequests)
		w.Write([]byte("rate limited"))
	}))
	defer fireSrv.Close()
	t.Setenv("FIRECRAWL_BASE_URL", fireSrv.URL)

	ddgSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(`<div class="result"><a class="result__a" href="https://example.com">Example Title</a><a class="result__snippet">Example snippet</a></div></div>`))
	}))
	defer ddgSrv.Close()
	restore := tools.SetDuckDuckGoURLForTest(ddgSrv.URL)
	defer restore()

	args, _ := json.Marshal(map[string]any{"query": "test fallback", "numResults": 1})
	out, err := tools.WebSearch.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !fireCalled {
		t.Fatal("expected Firecrawl to be called first")
	}
	s := resultText(t, out)
	if !strings.Contains(s, "Example Title") {
		t.Fatalf("output: %v", out)
	}
}

func TestWebSearchBraveRequiresKey(t *testing.T) {
	t.Setenv("BRAVE_SEARCH_API_KEY", "")
	args, _ := json.Marshal(map[string]any{"query": "x", "searchEngine": "brave"})
	if _, err := tools.WebSearch.Execute(context.Background(), args); err == nil {
		t.Fatal("expected error without BRAVE_SEARCH_API_KEY")
	}
}

func TestWebSearchBraveSuccess(t *testing.T) {
	braveSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Subscription-Token") != "test-key" {
			t.Errorf("missing subscription token")
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"web":{"results":[{"title":"Brave Title","url":"https://brave.example.com","description":"brave desc"}]}}`))
	}))
	defer braveSrv.Close()
	restore := tools.SetBraveSearchURLForTest(braveSrv.URL)
	defer restore()
	t.Setenv("BRAVE_SEARCH_API_KEY", "test-key")

	args, _ := json.Marshal(map[string]any{"query": "x", "searchEngine": "brave"})
	out, err := tools.WebSearch.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	s := resultText(t, out)
	if !strings.Contains(s, "Brave Title") {
		t.Fatalf("output: %v", out)
	}
}

func TestWebSearchMissingQuery(t *testing.T) {
	args, _ := json.Marshal(map[string]any{"query": ""})
	if _, err := tools.WebSearch.Execute(context.Background(), args); err == nil {
		t.Fatal("expected error for missing query")
	}
}
