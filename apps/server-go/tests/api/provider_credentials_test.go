// Generic provider API keys: storage, and the /api/providers/:id/credential
// endpoints built on it.
package tests

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/providers"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/providers/apikeys"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/providers/opencode"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/routes"
)

// useTempKeyStore points the store at a temp file and clears the OpenCode env
// override, so a developer's real OPENCODE_API_KEY cannot change the result.
func useTempKeyStore(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "provider-keys.json")
	t.Setenv("CONSOLE_PROVIDER_KEYS_PATH", path)
	t.Setenv("OPENCODE_API_KEY", "")
	return path
}

func TestSetAndGetRoundTrips(t *testing.T) {
	useTempKeyStore(t)

	if err := apikeys.Set("opencode", "sk-test-123"); err != nil {
		t.Fatal(err)
	}
	key, ok := apikeys.Get("opencode")
	if !ok || key != "sk-test-123" {
		t.Fatalf("round trip failed: %q ok=%v", key, ok)
	}
	if !apikeys.Has("opencode") {
		t.Fatal("Has should be true after Set")
	}
}

func TestBlankKeysAreRejected(t *testing.T) {
	useTempKeyStore(t)

	// A stored empty string would read as "configured" to the UI while
	// sending an empty bearer upstream.
	for _, blank := range []string{"", "   ", "\t\n"} {
		if err := apikeys.Set("opencode", blank); err == nil {
			t.Fatalf("blank key %q should be rejected", blank)
		}
	}
	if apikeys.Has("opencode") {
		t.Fatal("a rejected key must not leave the provider configured")
	}
}

func TestSetReplacesRatherThanAppends(t *testing.T) {
	useTempKeyStore(t)

	_ = apikeys.Set("opencode", "first")
	_ = apikeys.Set("opencode", "second")
	key, _ := apikeys.Get("opencode")
	if key != "second" {
		t.Fatalf("expected the replacement, got %q", key)
	}
}

func TestClearRemovesOnlyTheNamedProvider(t *testing.T) {
	useTempKeyStore(t)

	_ = apikeys.Set("opencode", "oc-key")
	_ = apikeys.Set("other", "other-key")

	if err := apikeys.Clear("opencode"); err != nil {
		t.Fatal(err)
	}
	if apikeys.Has("opencode") {
		t.Fatal("opencode key should be gone")
	}
	// Clearing one provider must never disturb another — a "disconnect" button
	// that wiped every key would be a nasty surprise.
	if key, ok := apikeys.Get("other"); !ok || key != "other-key" {
		t.Fatalf("unrelated key was disturbed: %q ok=%v", key, ok)
	}
}

func TestClearIsIdempotent(t *testing.T) {
	useTempKeyStore(t)

	if err := apikeys.Clear("opencode"); err != nil {
		t.Fatalf("clearing an unconfigured provider should succeed: %v", err)
	}
}

func TestStoreFileIsNotWorldReadable(t *testing.T) {
	path := useTempKeyStore(t)

	if err := apikeys.Set("opencode", "sk-secret"); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Fatalf("key file must be 0600, got %o", perm)
	}
	// The directory too: in production ~/.console is created by other
	// subsystems at the process umask, which is typically 0755.
	dirInfo, err := os.Stat(filepath.Dir(path))
	if err != nil {
		t.Fatal(err)
	}
	if perm := dirInfo.Mode().Perm(); perm != 0o700 {
		t.Fatalf("key directory must be 0700, got %o", perm)
	}
}

func TestOpenCodeRequiresAKey(t *testing.T) {
	useTempKeyStore(t)

	// Go has no anonymous tier. Unconfigured must be a hard, named error — not
	// a fallback identity, which would have silently run the user's traffic on
	// someone else's free plan.
	if got, err := opencode.APIKey(); err == nil {
		t.Fatalf("expected an error with no key configured, got %q", got)
	} else if !errors.Is(err, opencode.ErrNoAPIKey) {
		t.Fatalf("expected ErrNoAPIKey, got %v", err)
	}
	if got, _ := opencode.APIKey(); got != "" {
		t.Fatalf("a failed resolution must not leak a credential, got %q", got)
	}

	// A stored key is used once present.
	if err := apikeys.Set("opencode", "stored-key"); err != nil {
		t.Fatal(err)
	}
	if got, err := opencode.APIKey(); err != nil || got != "stored-key" {
		t.Fatalf("stored key not used: %q %v", got, err)
	}

	// Env wins over stored, so CI can override a configured server.
	t.Setenv("OPENCODE_API_KEY", "env-key")
	if got, err := opencode.APIKey(); err != nil || got != "env-key" {
		t.Fatalf("env should win over stored: %q %v", got, err)
	}

	// Whitespace-only env must not shadow a stored key — a stray newline in a
	// CI secret file would otherwise silently break a working configuration.
	t.Setenv("OPENCODE_API_KEY", "   ")
	if got, err := opencode.APIKey(); err != nil || got != "stored-key" {
		t.Fatalf("blank env must fall through to stored: %q %v", got, err)
	}
}

func TestSetCommonHeadersCarriesTheStoredKey(t *testing.T) {
	useTempKeyStore(t)
	if err := apikeys.Set("opencode", "sk-header-key"); err != nil {
		t.Fatal(err)
	}

	req, err := http.NewRequest("GET", "https://example.invalid", nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := opencode.SetCommonHeaders(req, ""); err != nil {
		t.Fatalf("set headers: %v", err)
	}

	if got := req.Header.Get("Authorization"); got != "Bearer sk-header-key" {
		t.Fatalf("Authorization must carry the stored key, got %q", got)
	}
}

func TestSetCommonHeadersRefusesWithoutAKey(t *testing.T) {
	useTempKeyStore(t)

	req, err := http.NewRequest("GET", "https://example.invalid", nil)
	if err != nil {
		t.Fatal(err)
	}
	err = opencode.SetCommonHeaders(req, "")
	if !errors.Is(err, opencode.ErrNoAPIKey) {
		t.Fatalf("expected ErrNoAPIKey, got %v", err)
	}
	// Sending an unauthenticated request would surface as an opaque upstream
	// 401 instead of naming the thing the user can fix.
	if got := req.Header.Get("Authorization"); got != "" {
		t.Fatalf("no Authorization header may be set without a key, got %q", got)
	}
}

func TestFetchModelsRefusesWithoutAKey(t *testing.T) {
	useTempKeyStore(t)

	// The credential check must happen before the network call, so an
	// unconfigured server reports the real problem.
	_, err := opencode.FetchModels(t.Context(), nil, "https://example.invalid")
	if !errors.Is(err, opencode.ErrNoAPIKey) {
		t.Fatalf("expected ErrNoAPIKey, got %v", err)
	}
}

func TestFreeTierIdentityIsNeverSent(t *testing.T) {
	useTempKeyStore(t)
	t.Setenv("OPENCODE_API_KEY", "")

	// Regression guard. The provider used to end in "public", Zen's anonymous
	// shared bearer, so an unconfigured server consumed the free tier while
	// looking identical to a paid one. Asserted behaviorally: whatever the
	// resolution path, no request may carry that identity.
	req, err := http.NewRequest("GET", "https://example.invalid", nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := opencode.SetCommonHeaders(req, ""); err == nil {
		t.Fatal("an unconfigured provider must not produce a usable request")
	}
	if got := req.Header.Get("Authorization"); strings.Contains(got, "public") {
		t.Fatalf("free-tier identity sent: %q", got)
	}
}

// --- HTTP surface ---------------------------------------------------------

func providerCredentialRequest(t *testing.T, method, path, body string) (int, string) {
	t.Helper()
	app := fiber.New()
	routes.RegisterProviderCredentialRoutes(app)

	var reader io.Reader
	if body != "" {
		reader = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, path, reader)
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, 10000)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	return resp.StatusCode, string(raw)
}

func TestProviderCredentialEndpointsRoundTrip(t *testing.T) {
	useTempKeyStore(t)

	// Before: not configured.
	code, body := providerCredentialRequest(t, "GET", "/api/providers/credentials", "")
	if code != http.StatusOK {
		t.Fatalf("status: %d %s", code, body)
	}
	var status struct {
		Success bool `json:"success"`
		Data    struct {
			Providers map[string]struct {
				Configured *bool   `json:"configured"`
				Source     *string `json:"source"`
			} `json:"providers"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(body), &status); err != nil {
		t.Fatal(err)
	}
	oc, ok := status.Data.Providers["opencode"]
	if !ok {
		t.Fatalf("opencode must always be reported, got %v", status.Data.Providers)
	}
	if oc.Configured == nil || *oc.Configured {
		t.Fatalf("expected configured=false explicitly present, got %v", oc.Configured)
	}

	// Set.
	code, body = providerCredentialRequest(t, "PUT", "/api/providers/opencode/credential", `{"apiKey":"sk-live-abc"}`)
	if code != http.StatusOK {
		t.Fatalf("set: %d %s", code, body)
	}
	if !strings.Contains(body, `"configured":true`) {
		t.Fatalf("set should report configured, got %s", body)
	}

	// The response must not echo the key back.
	if strings.Contains(body, "sk-live-abc") {
		t.Fatalf("the key leaked into the response: %s", body)
	}

	// Delete.
	code, body = providerCredentialRequest(t, "DELETE", "/api/providers/opencode/credential", "")
	if code != http.StatusOK {
		t.Fatalf("delete: %d %s", code, body)
	}
	if !strings.Contains(body, `"configured":false`) {
		t.Fatalf("delete should report configured=false, got %s", body)
	}
	if apikeys.Has("opencode") {
		t.Fatal("key should be gone after DELETE")
	}
}

func TestProviderCredentialStatusNeverContainsTheKey(t *testing.T) {
	useTempKeyStore(t)
	const secret = "sk-super-secret-value"
	if err := apikeys.Set("opencode", secret); err != nil {
		t.Fatal(err)
	}

	for _, path := range []string{"/api/providers/credentials"} {
		_, body := providerCredentialRequest(t, "GET", path, "")
		if strings.Contains(body, secret) {
			t.Fatalf("%s leaked the key: %s", path, body)
		}
		// Not even a prefix: it would leak length and confirm a guess.
		if strings.Contains(body, "sk-super") {
			t.Fatalf("%s leaked a key prefix: %s", path, body)
		}
	}
}

func TestBlankKeyIsRejectedWithABadRequest(t *testing.T) {
	useTempKeyStore(t)

	code, body := providerCredentialRequest(t, "PUT", "/api/providers/opencode/credential", `{"apiKey":"   "}`)
	if code != http.StatusBadRequest {
		t.Fatalf("a blank key should be 400, got %d %s", code, body)
	}
	if apikeys.Has("opencode") {
		t.Fatal("a rejected key must not be stored")
	}
}

func TestOAuthProvidersRejectAPIKeys(t *testing.T) {
	useTempKeyStore(t)

	// claude/codex/antigravity renew their own tokens; accepting a pasted key
	// would imply a credential path that does not exist for them.
	for _, provider := range []string{"claude", "codex", "antigravity", "nope"} {
		code, _ := providerCredentialRequest(t, "PUT", "/api/providers/"+provider+"/credential", `{"apiKey":"sk-x"}`)
		if code != http.StatusNotFound {
			t.Fatalf("%s should reject an API key, got %d", provider, code)
		}
	}
	if apikeys.Has("claude") {
		t.Fatal("a rejected provider must not have a key stored")
	}
}

func TestSupportsAPIKeyMatchesTheCatalog(t *testing.T) {
	if !providers.SupportsAPIKey("opencode") {
		t.Fatal("opencode accepts a key")
	}
	for _, p := range []string{"claude", "codex", "antigravity", "unknown", ""} {
		if providers.SupportsAPIKey(p) {
			t.Fatalf("%q must not accept an API key", p)
		}
	}
}
