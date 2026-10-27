// Account-management route coverage (docs/plan/provider-accounts.md Phase
// 3): the account list on /api/auth/status, login-callback registration
// with add-never-activates semantics, dedupe, activate/deselect, delete
// with promotion, the locked env account, and route error codes.
package tests

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/accounts"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/auth"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/db"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/providers/codex"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/routes"
)

// newAuthAccountsApp builds a Fiber app with the auth routes, wired to an
// isolated account store. Credential overrides point at non-existent
// files, mirroring the provider tests: the developer's real ~/.console
// credentials must stay out of tests.
func newAuthAccountsApp(t *testing.T) (*fiber.App, *auth.AuthService) {
	t.Helper()
	dir := t.TempDir()
	for env, name := range map[string]string{
		"CLAUDE_CREDENTIALS_PATH":      "claude-creds.json",
		"CODEX_CREDENTIALS_PATH":       "codex-creds.json",
		"ANTIGRAVITY_CREDENTIALS_PATH": "antigravity-creds.json",
	} {
		t.Setenv(env, filepath.Join(dir, "no-legacy", name))
	}
	database, err := db.Open(db.OpenOptions{Path: ":memory:", StorageDir: dir})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { database.Close() })
	if err := accounts.Configure(database); err != nil {
		t.Fatalf("configure accounts: %v", err)
	}
	svc := auth.NewAuthService()
	app := fiber.New()
	routes.RegisterAuthRoutes(app, svc)
	return app, svc
}

// mapAt indexes nested response data.
func mapAt(t *testing.T, body map[string]any, keys ...string) map[string]any {
	t.Helper()
	cur := body
	for _, k := range keys {
		next, ok := cur[k].(map[string]any)
		if !ok {
			t.Fatalf("response %v missing map at %q", body, k)
		}
		cur = next
	}
	return cur
}

// accountsOf reads the account list from a single provider's status object
// (a "data".<provider> map from /api/auth/status, or the "data"."status"
// map from an account-mutation response).
func accountsOf(t *testing.T, providerStatus map[string]any) []map[string]any {
	t.Helper()
	raw, ok := providerStatus["accounts"].([]any)
	if !ok {
		return nil
	}
	out := make([]map[string]any, 0, len(raw))
	for _, e := range raw {
		entry, ok := e.(map[string]any)
		if !ok {
			t.Fatalf("account entry not an object: %v", e)
		}
		out = append(out, entry)
	}
	return out
}

// statusRoute reads GET /api/auth/status through the app.
func statusRoute(t *testing.T, app *fiber.App) map[string]any {
	t.Helper()
	_, body := do(t, app, "GET", "/api/auth/status", "")
	return body
}

// mustLogin returns a fresh valid state token; the service's Exchange stub
// decides what the callback mints.
func mustLogin(t *testing.T, svc *auth.AuthService, provider string) string {
	t.Helper()
	login, err := svc.GetLoginURLFor(provider)
	if err != nil {
		t.Fatal(err)
	}
	return login.State
}

// twoCodexAccounts registers two codex accounts with millisecond-spaced
// created_at, returning their ids in registration order.
func twoCodexAccounts(t *testing.T) (string, string) {
	t.Helper()
	first, _, err := accounts.Register("codex", "u@x.io", "u@x.io", "")
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(2 * time.Millisecond)
	second, _, err := accounts.Register("codex", "v@x.io", "v@x.io", "")
	if err != nil {
		t.Fatal(err)
	}
	return first.ID, second.ID
}

func TestAuthAccountsStatusListsAccounts(t *testing.T) {
	app, _ := newAuthAccountsApp(t)
	twoCodexAccounts(t)

	body := statusRoute(t, app)
	list := accountsOf(t, mapAt(t, body, "data", "codex"))
	if len(list) != 2 {
		t.Fatalf("codex accounts = %v", list)
	}
	if list[0]["active"] != true || list[1]["active"] != false {
		t.Fatalf("first account must be active: %v", list)
	}
	// createdAt reaches the wire as a protojson string, not a JSON number.
	got, isString := list[0]["createdAt"].(string)
	if !isString || got == "" {
		t.Fatalf("createdAt = %v (type %T), want a protojson string", list[0]["createdAt"], list[0]["createdAt"])
	}

	// Old-style fields still describe the active account (backwards
	// compatibility). LoadCredential cannot read a credential (no file was
	// saved yet), so loggedIn comes from CredentialExists.
	providerStatus := mapAt(t, body, "data", "codex")
	if providerStatus["loggedIn"] != true {
		t.Fatalf("legacy fields = %v", providerStatus)
	}

	// A provider with no accounts keeps the pre-accounts shape entirely.
	antigravity := mapAt(t, body, "data", "antigravity")
	if _, has := antigravity["accounts"]; has {
		t.Fatalf("antigravity must have no accounts key: %v", antigravity)
	}
}

func TestAuthAccountsLoginRegistersActiveFirst(t *testing.T) {
	app, svc := newAuthAccountsApp(t)
	svc.Exchange = func(code, verifier, redirectURI string) (codex.OAuthCredential, error) {
		return codex.OAuthCredential{AccessToken: "a", RefreshToken: "r", ExpiresAt: 1 << 40, AccountID: "acc-1", Email: "user@example.com"}, nil
	}

	result, err := svc.HandleCallbackFor("codex", "auth-code", mustLogin(t, svc, "codex"))
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	if !strings.HasPrefix(result.AccountID, "acc_") || !result.AccountActive {
		t.Fatalf("login result = %+v, want a fresh active account", result)
	}
	list := accountsOf(t, mapAt(t, statusRoute(t, app), "data", "codex"))
	if len(list) != 1 || list[0]["email"] != "user@example.com" || list[0]["active"] != true {
		t.Fatalf("registered account = %v", list)
	}
	// The credential landed in the account's file — the active load reads it.
	cred, err := codex.LoadCredential()
	if err != nil || cred.AccountID != "acc-1" {
		t.Fatalf("credential load = %+v (err=%v)", cred, err)
	}
}

func TestAuthAccountsSecondLoginDoesNotStealActive(t *testing.T) {
	_, svc := newAuthAccountsApp(t)
	svc.Exchange = func(code, verifier, redirectURI string) (codex.OAuthCredential, error) {
		return codex.OAuthCredential{AccessToken: "a", RefreshToken: "r", ExpiresAt: 1 << 40, AccountID: "acc-1", Email: "user@example.com"}, nil
	}
	if _, err := svc.HandleCallbackFor("codex", "c1", mustLogin(t, svc, "codex")); err != nil {
		t.Fatalf("first login: %v", err)
	}

	// Second login with a different identity: new account, still not active.
	svc.Exchange = func(code, verifier, redirectURI string) (codex.OAuthCredential, error) {
		return codex.OAuthCredential{AccessToken: "b", RefreshToken: "r2", ExpiresAt: 1 << 40, AccountID: "acc-2", Email: "other@example.com"}, nil
	}
	result, err := svc.HandleCallbackFor("codex", "c2", mustLogin(t, svc, "codex"))
	if err != nil {
		t.Fatalf("second login: %v", err)
	}
	if result.AccountActive {
		t.Fatalf("second login must not activate: %+v", result)
	}
	list, err := accounts.List("codex")
	if err != nil || len(list) != 2 {
		t.Fatalf("accounts after second login = %v (err=%v)", list, err)
	}
}

func TestAuthAccountsReloginDedupes(t *testing.T) {
	_, svc := newAuthAccountsApp(t)
	svc.Exchange = func(code, verifier, redirectURI string) (codex.OAuthCredential, error) {
		return codex.OAuthCredential{AccessToken: "a", RefreshToken: "r", ExpiresAt: 1 << 40, AccountID: "acc-1", Email: "user@example.com"}, nil
	}
	if _, err := svc.HandleCallbackFor("codex", "c1", mustLogin(t, svc, "codex")); err != nil {
		t.Fatal(err)
	}
	// Same identity (acc-1): update in place, no duplicated row.
	result, err := svc.HandleCallbackFor("codex", "c2", mustLogin(t, svc, "codex"))
	if err != nil {
		t.Fatalf("re-login: %v", err)
	}
	if !result.AccountActive {
		t.Fatal("re-login of the active account stays active")
	}
	list, err := accounts.List("codex")
	if err != nil || len(list) != 1 {
		t.Fatalf("re-login must dedupe, got %v (err=%v)", list, err)
	}
}

func TestAuthAccountsActivateDeselectDelete(t *testing.T) {
	app, _ := newAuthAccountsApp(t)
	firstID, secondID := twoCodexAccounts(t)

	// Activate the second.
	code, body := do(t, app, "PATCH", "/api/auth/accounts/codex/"+secondID, `{"active":true}`)
	if code != 200 {
		t.Fatalf("patch = %d %v", code, body)
	}
	list := accountsOf(t, mapAt(t, body, "data", "status"))
	if len(list) != 2 || list[0]["active"] != false || list[1]["active"] != true || list[1]["id"] != secondID {
		t.Fatalf("post-activate list = %v", list)
	}

	// Unknown account: 404.
	if code, _ := do(t, app, "PATCH", "/api/auth/accounts/codex/acc_unknown", `{"active":true}`); code != 404 {
		t.Fatalf("unknown patch = %d, want 404", code)
	}
	// Empty body: 400.
	if code, _ := do(t, app, "PATCH", "/api/auth/accounts/codex/"+firstID, `{}`); code != 400 {
		t.Fatalf("empty patch = %d, want 400", code)
	}
	// Rename.
	if code, body = do(t, app, "PATCH", "/api/auth/accounts/codex/"+firstID, `{"label":"Work"}`); code != 200 {
		t.Fatalf("rename = %d %v", code, body)
	}
	if list := accountsOf(t, mapAt(t, body, "data", "status")); list[0]["label"] != "Work" {
		t.Fatalf("label = %v", list)
	}

	// Delete the active account: the remaining one is promoted, and the
	// response already reflects it (no second status read needed).
	code, body = do(t, app, "DELETE", "/api/auth/accounts/codex/"+secondID, "")
	if code != 200 {
		t.Fatalf("delete = %d %v", code, body)
	}
	list = accountsOf(t, mapAt(t, body, "data", "status"))
	if len(list) != 1 || list[0]["id"] != firstID || list[0]["active"] != true {
		t.Fatalf("after delete = %v, want promotion of the remaining account", list)
	}
	// Deleting again is idempotent: 200, not 404.
	if code, _ := do(t, app, "DELETE", "/api/auth/accounts/codex/"+secondID, ""); code != 200 {
		t.Fatalf("idempotent delete = %d, want 200", code)
	}
}

func TestAuthAccountsEnvAccountLocked(t *testing.T) {
	app, _ := newAuthAccountsApp(t)
	t.Setenv("CLAUDE_OAUTH_TOKEN", "env-tok")

	list := accountsOf(t, mapAt(t, statusRoute(t, app), "data", "claude"))
	if len(list) != 1 || list[0]["id"] != "env" || list[0]["active"] != true {
		t.Fatalf("claude accounts under env = %v, want exactly the env account", list)
	}

	// The synthetic account cannot be deselected or removed while the env
	// variable owns the provider.
	if code, out := do(t, app, "PATCH", "/api/auth/accounts/claude/env", `{"active":false}`); code != 409 {
		t.Fatalf("env patch = %d %v, want 409", code, out)
	}
	if code, _ := do(t, app, "DELETE", "/api/auth/accounts/claude/env", ""); code != 409 {
		t.Fatalf("env delete = %d, want 409", code)
	}
}

func TestAuthAccountsUnknownProviders(t *testing.T) {
	app, _ := newAuthAccountsApp(t)

	if code, _ := do(t, app, "PATCH", "/api/auth/accounts/devin/acc_x", `{"active":true}`); code != 501 {
		t.Fatalf("devin patch = %d, want 501", code)
	}
	if code, _ := do(t, app, "DELETE", "/api/auth/accounts/devin/acc_x", ""); code != 501 {
		t.Fatalf("devin delete = %d, want 501", code)
	}
	if code, _ := do(t, app, "PATCH", "/api/auth/accounts/nope/acc_x", `{"active":true}`); code != 400 {
		t.Fatalf("unknown provider patch = %d, want 400", code)
	}
}
