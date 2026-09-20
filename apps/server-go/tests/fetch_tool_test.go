// Coverage for the fetch tool (direct-fetch path; Firecrawl is skipped by
// using an /api/ URL so tests stay hermetic and fast).
package tests

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/tools"
)

func TestFetchDirectJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	// /api/ path forces the direct-fetch path (skips Firecrawl).
	args, _ := json.Marshal(map[string]any{"url": srv.URL + "/api/ping"})
	out, err := tools.Fetch.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	s, ok := out.(string)
	if !ok || !strings.Contains(s, `"ok": true`) || !strings.Contains(s, "Status: 200") {
		t.Fatalf("output: %v", out)
	}
}

func TestFetchDirectHTML(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte("<html><body><script>evil()</script><p>Hello world</p></body></html>"))
	}))
	defer srv.Close()

	args, _ := json.Marshal(map[string]any{"url": srv.URL + "/api/page"})
	out, err := tools.Fetch.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	s, ok := out.(string)
	if !ok || !strings.Contains(s, "Hello world") || strings.Contains(s, "evil()") {
		t.Fatalf("output: %v", out)
	}
}

func TestFetchErrorStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte("nope"))
	}))
	defer srv.Close()

	args, _ := json.Marshal(map[string]any{"url": srv.URL + "/api/missing"})
	if _, err := tools.Fetch.Execute(context.Background(), args); err == nil {
		t.Fatal("expected tool error for 404 status")
	}
}

func TestFetchPost(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		body, _ := io.ReadAll(r.Body)
		w.Write([]byte("received: " + string(body)))
	}))
	defer srv.Close()

	args, _ := json.Marshal(map[string]any{"url": srv.URL, "method": "POST", "body": "hello"})
	out, err := tools.Fetch.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	s, ok := out.(string)
	if !ok || !strings.Contains(s, "received: hello") {
		t.Fatalf("output: %v", out)
	}
}

func TestFetchMissingURL(t *testing.T) {
	args, _ := json.Marshal(map[string]any{"url": ""})
	if _, err := tools.Fetch.Execute(context.Background(), args); err == nil {
		t.Fatal("expected error for missing url")
	}
}
