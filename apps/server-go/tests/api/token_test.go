// API bearer token coverage (QUIC/WebTransport migration, Phase 0):
// issuer round-trip, tamper/expiry/revocation failures, secret persistence,
// the mint route, and the shared bearer-or-query middleware.
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
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/auth"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/db"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/routes"
)

func tokenTestIssuer() *auth.TokenIssuer {
	return auth.NewTokenIssuerWithSecret([]byte("test-secret-32-bytes-long!!!!"))
}

func TestTokenMintValidateRoundTrip(t *testing.T) {
	issuer := tokenTestIssuer()
	token, exp, err := issuer.Mint("device-1", 0)
	if err != nil {
		t.Fatalf("mint: %v", err)
	}
	if !strings.HasPrefix(token, "ct1_") {
		t.Fatalf("token must carry the ct1_ prefix: %q", token)
	}
	now := time.Now().UnixMilli()
	if exp <= now || exp-now > int64((5*time.Minute).Milliseconds()) {
		t.Fatalf("default TTL must apply, expiresAt=%d now=%d", exp, now)
	}
	deviceID, err := issuer.Validate(token)
	if err != nil {
		t.Fatalf("validate: %v", err)
	}
	if deviceID != "device-1" {
		t.Fatalf("device = %q", deviceID)
	}
}

func TestTokenRejectsTamperedAndMalformed(t *testing.T) {
	issuer := tokenTestIssuer()
	token, _, err := issuer.Mint("device-1", 0)
	if err != nil {
		t.Fatal(err)
	}
	// Flip the last signature character.
	tampered := token[:len(token)-1] + map[string]string{"A": "B"}[token[len(token)-1:]]
	if tampered == token {
		tampered += "A"
	}
	for name, bad := range map[string]string{
		"tampered sig": tampered,
		"no prefix":    strings.TrimPrefix(token, "ct1_"),
		"no dot":       "ct1_nodotshere",
		"bad b64":      "ct1_%%%.%%%",
		"empty":        "",
		"wrong shape":  "ct1_aGVsbG8.d29ybGQ",
	} {
		if _, err := issuer.Validate(bad); !errors.Is(err, auth.ErrTokenInvalid) {
			t.Fatalf("%s: want ErrTokenInvalid, got %v", name, err)
		}
	}
}

func TestTokenExpiredTokensFail(t *testing.T) {
	issuer := tokenTestIssuer()
	token, _, err := issuer.Mint("device-1", -time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := issuer.Validate(token); !errors.Is(err, auth.ErrTokenExpired) {
		t.Fatalf("want ErrTokenExpired, got %v", err)
	}
}

func TestTokenRevocationIsPerToken(t *testing.T) {
	issuer := tokenTestIssuer()
	revoked, _, err := issuer.Mint("device-1", 0)
	if err != nil {
		t.Fatal(err)
	}
	kept, _, err := issuer.Mint("device-1", 0)
	if err != nil {
		t.Fatal(err)
	}
	if err := issuer.Revoke(revoked); err != nil {
		t.Fatalf("revoke: %v", err)
	}
	if _, err := issuer.Validate(revoked); !errors.Is(err, auth.ErrTokenRevoked) {
		t.Fatalf("want ErrTokenRevoked, got %v", err)
	}
	if deviceID, err := issuer.Validate(kept); err != nil || deviceID != "device-1" {
		t.Fatalf("sibling token must survive: %v %q", err, deviceID)
	}
	if err := issuer.Revoke("ct1_bogus.bogus"); !errors.Is(err, auth.ErrTokenInvalid) {
		t.Fatalf("revoking garbage must fail invalid, got %v", err)
	}
}

func TestTokenWrongSecretFails(t *testing.T) {
	token, _, err := tokenTestIssuer().Mint("device-1", 0)
	if err != nil {
		t.Fatal(err)
	}
	other := auth.NewTokenIssuerWithSecret([]byte("a-different-test-secret-!!!!"))
	if _, err := other.Validate(token); !errors.Is(err, auth.ErrTokenInvalid) {
		t.Fatalf("want ErrTokenInvalid, got %v", err)
	}
}

func TestTokenMintRequiresDevice(t *testing.T) {
	if _, _, err := tokenTestIssuer().Mint("   ", 0); err == nil {
		t.Fatal("blank device must fail")
	}
}

func TestTokenSecretPersistsAcrossIssuers(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CONSOLE_STORAGE_DIR", dir)
	t.Setenv("CONSOLE_TOKEN_SECRET_PATH", "")

	first := auth.NewTokenIssuer()
	token, _, err := first.Mint("device-1", 0)
	if err != nil {
		t.Fatal(err)
	}
	// A second issuer over the same storage must accept the first one's
	// token: the secret survived on disk, not in memory.
	second := auth.NewTokenIssuer()
	if deviceID, err := second.Validate(token); err != nil || deviceID != "device-1" {
		t.Fatalf("restart must keep tokens valid: %v %q", err, deviceID)
	}
	info, err := os.Stat(filepath.Join(dir, "api-token-secret"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("secret file must be 0600, got %o", info.Mode().Perm())
	}
}

// The old freely-mintable token route is gone: a device credential is
// issued only by exchanging a one-time pairing grant. The issuer itself
// still exists — it signs the device tokens the pairing flow hands out —
// and is covered above.
//
// What replaced the route's coverage is the device-token middleware, which
// is exercised in device_auth_test.go. The query-string credential path
// that the route also accepted is gone from ordinary HTTP entirely.
func tokenRouteApp() (*fiber.App, *auth.TokenIssuer) {
	issuer := tokenTestIssuer()
	app := fiber.New()
	return app, issuer
}

// The freely-mintable token route is gone for good: there is no path that
// turns a caller-supplied device id into a credential.
func TestTokenMintRouteIsGone(t *testing.T) {
	app, _ := tokenRouteApp()

	code, out := do(t, app, "POST", "/api/auth/token", `{"deviceId":"phone-1"}`)
	if code == http.StatusOK {
		t.Fatalf("the mint route must not exist: %d %v", code, out)
	}
}

// doTokenRequest sends a request with an optional Authorization header and
// returns status + decoded body.
func doTokenRequest(t *testing.T, app *fiber.App, method, target, authHeader string) (int, map[string]any) {
	t.Helper()
	req := httptest.NewRequest(method, target, nil)
	if authHeader != "" {
		req.Header.Set("Authorization", authHeader)
	}
	resp, err := app.Test(req, 10000)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := io.ReadAll(resp.Body)
	var out map[string]any
	_ = json.Unmarshal(raw, &out)
	return resp.StatusCode, out
}

// The device-token middleware replaces the old bearer middleware.
//
// The important difference is the query string: a credential in a URL
// leaks into access logs, proxy logs and browser history, so ordinary
// HTTP routes accept the header only. The WebTransport handshake keeps
// its own scoped copy of that path, because a CONNECT request from some
// clients cannot carry headers.
func TestDeviceTokenMiddlewareRejectsQueryStringToken(t *testing.T) {
	database, err := db.Open(db.OpenOptions{Path: ":memory:"})
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	issuer := auth.NewTokenIssuerWithSecret([]byte("device-token-middleware-secret!"))
	manager := auth.NewManager(database, issuer)
	grant, _, err := manager.CreateBootstrapGrant()
	if err != nil {
		t.Fatal(err)
	}
	paired, err := manager.Pair(grant, "Desktop", "desktop")
	if err != nil {
		t.Fatal(err)
	}

	app := fiber.New()
	routes.RegisterPairingRoutes(app, manager)
	app.Use(routes.RequireDeviceToken(manager))
	app.Get("/protected", func(c *fiber.Ctx) error {
		deviceID, ok := routes.DeviceIDFromContext(c)
		if !ok {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"success": false})
		}
		return c.JSON(fiber.Map{"success": true, "data": fiber.Map{"deviceId": deviceID}})
	})

	if code, _ := doTokenRequest(t, app, "GET", "/protected", ""); code != http.StatusUnauthorized {
		t.Fatalf("missing token should 401, got %d", code)
	}
	if code, _ := doTokenRequest(t, app, "GET", "/protected", "Bearer bogus"); code != http.StatusUnauthorized {
		t.Fatalf("bad token should 401, got %d", code)
	}
	if code, _ := doTokenRequest(t, app, "GET", "/protected", "Basic "+paired.Token); code != http.StatusUnauthorized {
		t.Fatalf("wrong scheme should 401, got %d", code)
	}
	// A credential in the query string must not authenticate anything.
	if code, _ := doTokenRequest(t, app, "GET", "/protected?token="+paired.Token, ""); code != http.StatusUnauthorized {
		t.Fatalf("query token must be rejected, got %d", code)
	}

	code, out := doTokenRequest(t, app, "GET", "/protected", "Bearer "+paired.Token)
	if code != http.StatusOK {
		t.Fatalf("header token should pass, got %d %v", code, out)
	}
	if data, _ := out["data"].(map[string]any); data["deviceId"] != paired.ID {
		t.Fatalf("device id must reach the handler: %v", data)
	}
	// Lowercase scheme is valid per RFC 9110.
	if code, _ := doTokenRequest(t, app, "GET", "/protected", "bearer "+paired.Token); code != http.StatusOK {
		t.Fatalf("lowercase bearer should pass, got %d", code)
	}
}
