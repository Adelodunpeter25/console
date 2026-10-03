// GitHub git-credential route coverage: PAT accept/reject, status, logout,
// and the guarantee that the token never appears in a response body.
package tests

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/auth"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/providers/github"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/routes"
)

const githubSecret = "ghp_super_secret_value"

// toJSON re-serializes a decoded body for substring leak assertions.
func toJSON(v any) string { b, _ := json.Marshal(v); return string(b) }

// do sends a request through a Fiber app and returns status + decoded body.
func do(t *testing.T, app *fiber.App, method, path, body string) (int, map[string]any) {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req, 10000)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := io.ReadAll(resp.Body)
	var out map[string]any
	_ = json.Unmarshal(raw, &out)
	return resp.StatusCode, out
}

// newGitHubAuthApp builds a Fiber app with only the auth routes registered,
// plus an AuthService whose PAT validation is stubbed.
func newGitHubAuthApp(t *testing.T, validate func(context.Context, *http.Client, string) (string, []string, error)) (*fiber.App, *auth.AuthService) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("GITHUB_CREDENTIALS_PATH", filepath.Join(dir, "github-creds.json"))
	svc := auth.NewAuthService()
	svc.ValidatePAT = validate
	app := fiber.New()
	routes.RegisterAuthRoutes(app, svc)
	return app, svc
}

// RegisterAuthRoutesApp is a thin alias kept for readability at call sites
// that build the app separately from the service.

// okPAT is a stub validator that accepts any non-blank token.
func okPAT(_ context.Context, _ *http.Client, token string) (string, []string, error) {
	if strings.TrimSpace(token) == "" {
		return "", nil, fmt.Errorf("GitHub token is empty.")
	}
	return "octocat", []string{"repo"}, nil
}

func TestGitHubPATRouteStoresAndReportsUsername(t *testing.T) {
	app, _ := newGitHubAuthApp(t, okPAT)

	code, out := do(t, app, "POST", "/api/auth/github/pat", `{"token":"`+githubSecret+`"}`)
	if code != http.StatusOK || out["success"] != true {
		t.Fatalf("pat: %d %v", code, out)
	}
	data, _ := out["data"].(map[string]any)
	if data["connected"] != true || data["username"] != "octocat" {
		t.Fatalf("status payload: %v", data)
	}
	if !github.CredentialExists() {
		t.Fatal("token must be persisted")
	}
	// The token must never be echoed back over the API.
	if body := toJSON(out); strings.Contains(body, githubSecret) {
		t.Fatalf("token leaked in response: %s", body)
	}
}

func TestGitHubPATRouteRejectsInvalidToken(t *testing.T) {
	app, _ := newGitHubAuthApp(t, func(context.Context, *http.Client, string) (string, []string, error) {
		return "", nil, github.ErrInvalidToken
	})

	code, out := do(t, app, "POST", "/api/auth/github/pat", `{"token":"`+githubSecret+`"}`)
	if code != http.StatusBadRequest || out["success"] != false {
		t.Fatalf("invalid token should 400, got %d %v", code, out)
	}
	if github.CredentialExists() {
		t.Fatal("a rejected token must not be stored")
	}
	if body := toJSON(out); strings.Contains(body, githubSecret) {
		t.Fatalf("token leaked in error response: %s", body)
	}
}

func TestGitHubPATRouteUnreachableGitHubIs502(t *testing.T) {
	app, _ := newGitHubAuthApp(t, func(context.Context, *http.Client, string) (string, []string, error) {
		return "", nil, fmt.Errorf("%w: %s", github.ErrUnreachable, context.DeadlineExceeded)
	})

	code, out := do(t, app, "POST", "/api/auth/github/pat", `{"token":"`+githubSecret+`"}`)
	// An unreachable GitHub is not the caller's fault: 502, not 400.
	if code != http.StatusBadGateway {
		t.Fatalf("unreachable GitHub should 502, got %d %v", code, out)
	}
	if github.CredentialExists() {
		t.Fatal("a failed verification must not store the token")
	}
	if body := toJSON(out); strings.Contains(body, githubSecret) {
		t.Fatalf("token leaked in error response: %s", body)
	}
}

// A rejected token must never replace a working one: validation runs before
// the file is written, so the old credential survives a failed attempt.
func TestGitHubPATRouteKeepsExistingTokenOnFailure(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "github-creds.json")
	t.Setenv("GITHUB_CREDENTIALS_PATH", path)

	svc := auth.NewAuthService()
	svc.ValidatePAT = okPAT
	app := fiber.New()
	routes.RegisterAuthRoutes(app, svc)

	if code, _ := do(t, app, "POST", "/api/auth/github/pat", `{"token":"good-token"}`); code != http.StatusOK {
		t.Fatal("initial connect failed")
	}

	// Now make validation fail, against the same credential file.
	svc.ValidatePAT = func(context.Context, *http.Client, string) (string, []string, error) {
		return "", nil, github.ErrInvalidToken
	}
	if code, _ := do(t, app, "POST", "/api/auth/github/pat", `{"token":"bad-token"}`); code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", code)
	}

	cred, err := github.LoadCredential()
	if err != nil {
		t.Fatal(err)
	}
	if cred.Token != "good-token" {
		t.Fatalf("working token must survive a failed attempt, got %q", cred.Token)
	}
}

func TestGitHubPATRouteRejectsBlankToken(t *testing.T) {
	app, _ := newGitHubAuthApp(t, okPAT)
	code, out := do(t, app, "POST", "/api/auth/github/pat", `{"token":"   "}`)
	if code != http.StatusBadRequest {
		t.Fatalf("blank token should 400, got %d %v", code, out)
	}
}

func TestGitHubStatusRoute(t *testing.T) {
	app, _ := newGitHubAuthApp(t, okPAT)

	// Disconnected to start.
	code, out := do(t, app, "GET", "/api/auth/github/status", "")
	if code != http.StatusOK {
		t.Fatalf("status: %d", code)
	}
	data, _ := out["data"].(map[string]any)
	if data["connected"] != false {
		t.Fatalf("expected disconnected: %v", data)
	}

	if code, _ := do(t, app, "POST", "/api/auth/github/pat", `{"token":"`+githubSecret+`"}`); code != http.StatusOK {
		t.Fatal("connect failed")
	}
	_, out = do(t, app, "GET", "/api/auth/github/status", "")
	data, _ = out["data"].(map[string]any)
	if data["connected"] != true || data["username"] != "octocat" {
		t.Fatalf("expected connected: %v", data)
	}
	if body := toJSON(out); strings.Contains(body, githubSecret) {
		t.Fatalf("token leaked in status response: %s", body)
	}
}

func TestGitHubLogoutRoute(t *testing.T) {
	app, _ := newGitHubAuthApp(t, okPAT)
	if code, _ := do(t, app, "POST", "/api/auth/github/pat", `{"token":"`+githubSecret+`"}`); code != http.StatusOK {
		t.Fatal("connect failed")
	}

	code, out := do(t, app, "POST", "/api/auth/github/logout", "")
	if code != http.StatusOK || out["success"] != true {
		t.Fatalf("logout: %d %v", code, out)
	}
	if github.CredentialExists() {
		t.Fatal("credential file must be removed on logout")
	}
	_, out = do(t, app, "GET", "/api/auth/github/status", "")
	data, _ := out["data"].(map[string]any)
	if data["connected"] != false {
		t.Fatalf("expected disconnected after logout: %v", data)
	}
	// Idempotent: a second logout must not 500.
	if code, _ := do(t, app, "POST", "/api/auth/github/logout", ""); code != http.StatusOK {
		t.Fatalf("second logout should succeed, got %d", code)
	}
}

// The shared auth status endpoint must report the GitHub row and must not
// break the other providers while doing so.
func TestGetStatusIncludesGitHub(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("GITHUB_CREDENTIALS_PATH", filepath.Join(dir, "github-creds.json"))
	t.Setenv("CODEX_CREDENTIALS_PATH", filepath.Join(dir, "missing-codex.json"))
	t.Setenv("OPENAI_CODEX_OAUTH_TOKEN", "")
	t.Setenv("CLAUDE_CREDENTIALS_PATH", filepath.Join(dir, "missing-claude.json"))
	t.Setenv("CLAUDE_OAUTH_TOKEN", "")
	t.Setenv("ANTHROPIC_OAUTH_TOKEN", "")
	t.Setenv("ANTIGRAVITY_CREDENTIALS_PATH", filepath.Join(dir, "missing-ag.json"))

	svc := auth.NewAuthService()
	if status := svc.GetStatus(); status.GitHub.Connected {
		t.Fatalf("must be disconnected without a credential file: %+v", status.GitHub)
	}
	if err := github.SaveCredential(github.Credential{Token: githubSecret, Username: "octocat", Scopes: []string{"repo"}}); err != nil {
		t.Fatal(err)
	}
	status := svc.GetStatus()
	if !status.GitHub.Connected || status.GitHub.Username != "octocat" {
		t.Fatalf("github row: %+v", status.GitHub)
	}
	if len(status.GitHub.Scopes) != 1 {
		t.Fatalf("cached scopes should survive: %+v", status.GitHub.Scopes)
	}
	// A missing GitHub credential must not affect the LLM providers.
	if status.Codex.LoggedIn || status.Claude.LoggedIn || status.Antigravity.LoggedIn {
		t.Fatalf("unrelated providers must stay logged out: %+v", status)
	}
	// The serialized status must never carry the token.
	raw, err := json.Marshal(status)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), githubSecret) {
		t.Fatalf("token leaked in auth status: %s", raw)
	}
}
