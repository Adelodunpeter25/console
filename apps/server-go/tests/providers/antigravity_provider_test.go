// Antigravity OAuth handshake coverage: authorization URL params,
// tier/project helpers, token exchange, loadCodeAssist short-circuit and
// paid-tier guard, onboardUser immediate-done and polling.
package tests

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/providers/antigravity"
)

func TestAntigravityAuthorizationURL(t *testing.T) {
	authURL, redirect := antigravity.AuthorizationURL("state-abc")
	if redirect != "http://localhost:51121/oauth-callback" {
		t.Fatalf("redirect = %q", redirect)
	}
	u, err := url.Parse(authURL)
	if err != nil {
		t.Fatal(err)
	}
	q, _ := url.ParseQuery(u.RawQuery)
	if q.Get("client_id") == "" || q.Get("response_type") != "code" || q.Get("state") != "state-abc" {
		t.Fatalf("params = %s", authURL)
	}
	if q.Get("access_type") != "offline" || q.Get("prompt") != "consent" {
		t.Fatalf("params = %s", authURL)
	}
	if !strings.Contains(q.Get("scope"), "cloud-platform") || !strings.Contains(q.Get("redirect_uri"), "51121") {
		t.Fatalf("params = %s", authURL)
	}
	// Expected order: client_id, response_type, scope, redirect_uri, state, access_type, prompt.
	order := []string{"client_id=", "response_type=", "scope=", "redirect_uri=", "state=", "access_type=", "prompt="}
	last := -1
	for _, key := range order {
		idx := strings.Index(u.RawQuery, key)
		if idx < 0 {
			t.Fatalf("missing %s in %s", key, u.RawQuery)
		}
		if idx < last {
			t.Fatalf("param order wrong in %s", u.RawQuery)
		}
		last = idx
	}
}

func TestAntigravityTierAndProjectHelpers(t *testing.T) {
	if got := antigravity.GetDefaultTier(nil).ID; got != "legacy-tier" {
		t.Fatalf("empty tiers = %q", got)
	}
	if got := antigravity.GetDefaultTier([]antigravity.Tier{{ID: "a"}, {ID: "b"}}).ID; got != "legacy-tier" {
		t.Fatalf("no default = %q", got)
	}
	if got := antigravity.GetDefaultTier([]antigravity.Tier{{ID: "a"}, {ID: "b", IsDefault: true}}).ID; got != "b" {
		t.Fatalf("default = %q", got)
	}
	if got := antigravity.ReadProjectId("proj-1"); got != "proj-1" {
		t.Fatalf("string = %q", got)
	}
	if got := antigravity.ReadProjectId(map[string]any{"id": "proj-2"}); got != "proj-2" {
		t.Fatalf("object = %q", got)
	}
	if got := antigravity.ReadProjectId(map[string]any{"id": ""}); got != "" {
		t.Fatalf("empty id = %q", got)
	}
	if got := antigravity.ReadProjectId(nil); got != "" {
		t.Fatalf("nil = %q", got)
	}
}

func TestAntigravityExchangeCode(t *testing.T) {
	var gotForm url.Values
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("method = %s", r.Method)
		}
		if ct := r.Header.Get("Content-Type"); ct != "application/x-www-form-urlencoded" {
			t.Errorf("content-type = %q", ct)
		}
		_ = r.ParseForm()
		gotForm = r.PostForm
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"access_token":"a","refresh_token":"r","expires_in":3600}`)
	}))
	defer srv.Close()
	t.Setenv("ANTIGRAVITY_TOKEN_URL", srv.URL)

	tokens, err := antigravity.ExchangeCode(srv.Client(), "code-123", "http://localhost:51121/oauth-callback")
	if err != nil {
		t.Fatal(err)
	}
	if tokens.AccessToken != "a" || tokens.RefreshToken != "r" || tokens.ExpiresIn != 3600 {
		t.Fatalf("tokens = %+v", tokens)
	}
	if gotForm.Get("code") != "code-123" || gotForm.Get("grant_type") != "authorization_code" {
		t.Fatalf("form = %v", gotForm)
	}
	if gotForm.Get("client_id") == "" || gotForm.Get("client_secret") == "" || gotForm.Get("redirect_uri") == "" {
		t.Fatalf("form = %v", gotForm)
	}
}

func TestAntigravityExchangeCodeError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(400)
		fmt.Fprint(w, `{"error":"invalid_grant","error_description":"bad code"}`)
	}))
	defer srv.Close()
	t.Setenv("ANTIGRAVITY_TOKEN_URL", srv.URL)

	if _, err := antigravity.ExchangeCode(srv.Client(), "bad", "http://localhost:51121/oauth-callback"); err == nil {
		t.Fatal("expected error")
	} else if !strings.Contains(err.Error(), "Antigravity") || !strings.Contains(err.Error(), "400") {
		t.Fatalf("err = %v", err)
	}
}

func TestAntigravityLoadCodeAssistShortCircuit(t *testing.T) {
	t.Setenv("GOOGLE_CLOUD_PROJECT", "")
	t.Setenv("GOOGLE_CLOUD_PROJECT_ID", "")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1internal:loadCodeAssist" {
			t.Errorf("path = %s", r.URL.Path)
		}
		if auth := r.Header.Get("Authorization"); auth != "Bearer tok" {
			t.Errorf("auth = %q", auth)
		}
		if ua := r.Header.Get("User-Agent"); !strings.HasPrefix(ua, "antigravity/") {
			t.Errorf("ua = %q", ua)
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		meta, _ := body["metadata"].(map[string]any)
		if meta["ideType"] != "ANTIGRAVITY" || meta["pluginType"] != "GEMINI" {
			t.Errorf("metadata = %v", body)
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"cloudaicompanionProject":"proj-123"}`)
	}))
	defer srv.Close()
	t.Setenv("ANTIGRAVITY_BASE_URL", srv.URL)

	project, err := antigravity.LoadCodeAssist(srv.Client(), "tok", "")
	if err != nil {
		t.Fatal(err)
	}
	if project != "proj-123" {
		t.Fatalf("project = %q", project)
	}
}

func TestAntigravityLoadCodeAssistPaidTierGuard(t *testing.T) {
	t.Setenv("GOOGLE_CLOUD_PROJECT", "")
	t.Setenv("GOOGLE_CLOUD_PROJECT_ID", "")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"currentTier":{"id":"standard-tier"},"allowedTiers":[]}`)
	}))
	defer srv.Close()
	t.Setenv("ANTIGRAVITY_BASE_URL", srv.URL)

	if _, err := antigravity.LoadCodeAssist(srv.Client(), "tok", ""); err == nil {
		t.Fatal("expected paid-tier error")
	} else if !strings.Contains(err.Error(), "GOOGLE_CLOUD_PROJECT") {
		t.Fatalf("err = %v", err)
	}
}

func TestAntigravityOnboardUserImmediateDone(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1internal:onboardUser" {
			t.Errorf("path = %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"done":true,"response":{"cloudaicompanionProject":{"id":"proj-onboard"}}}`)
	}))
	defer srv.Close()
	t.Setenv("ANTIGRAVITY_BASE_URL", srv.URL)

	project, err := antigravity.OnboardUser(srv.Client(), "tok", "legacy-tier", "")
	if err != nil {
		t.Fatal(err)
	}
	if project != "proj-onboard" {
		t.Fatalf("project = %q", project)
	}
}

func TestAntigravityOnboardUserPolling(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == "POST" {
			fmt.Fprint(w, `{"done":false,"name":"operations/abc"}`)
			return
		}
		if r.URL.Path != "/v1internal/operations/abc" {
			t.Errorf("poll path = %s", r.URL.Path)
		}
		fmt.Fprint(w, `{"done":true,"response":{"cloudaicompanionProject":"proj-polled"}}`)
	}))
	defer srv.Close()
	t.Setenv("ANTIGRAVITY_BASE_URL", srv.URL)

	project, err := antigravity.OnboardUser(srv.Client(), "tok", "legacy-tier", "")
	if err != nil {
		t.Fatal(err)
	}
	if project != "proj-polled" {
		t.Fatalf("project = %q", project)
	}
}
