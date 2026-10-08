// Auth migration coverage: the /api/auth payloads must match the shared
// golden fixtures. The OAuth flow behaviour (state single-use, credential
// persistence) lives in auth_test.go.
package tests

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/auth"
	consolev1 "github.com/Adelodunpeter25/console/apps/server-go/internal/gen/console/v1"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/routes"
)

func authFixture(t *testing.T, name string) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "proto", "testdata", "auth", name))
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(string(raw))
}

func checkAuthFixture(t *testing.T, name string, msg proto.Message) {
	t.Helper()
	raw, err := protojson.Marshal(msg)
	if err != nil {
		t.Fatal(err)
	}
	if compactJSON(t, raw) != authFixture(t, name) {
		t.Fatalf("%s drifted:\n got %s\nwant %s", name, compactJSON(t, raw), authFixture(t, name))
	}
}

func TestAuthStatusFixture(t *testing.T) {
	checkAuthFixture(t, "status.json", routes.AuthStatusToProtoForTest(auth.AuthStatus{
		Antigravity: auth.ProviderAuthStatus{LoggedIn: true, Email: "adelodunjoseph892@gmail.com"},
		Codex:       auth.ProviderAuthStatus{LoggedIn: true, Email: "adelodunpeter24@gmail.com"},
		Claude:      auth.ProviderAuthStatus{LoggedIn: true, Email: "peter@sinettechnologies.com"},
	}))
}

// Every provider is always present, so a logged-out provider emits {} rather
// than disappearing — "logged out" must never look like "not reported".
func TestAuthStatusLoggedOutFixture(t *testing.T) {
	checkAuthFixture(t, "status_logged_out.json", routes.AuthStatusToProtoForTest(auth.AuthStatus{}))
}

func TestAuthLoginUrlFixture(t *testing.T) {
	checkAuthFixture(t, "login_url.json", &consolev1.OAuthLoginUrlResponse{
		Provider:    "codex",
		AuthUrl:     "https://auth.openai.com/oauth/authorize?state=st_123",
		State:       "st_123",
		RedirectUri: "http://localhost:1455/auth/callback",
	})
}

func TestAuthCallbackFixture(t *testing.T) {
	checkAuthFixture(t, "callback.json", &consolev1.OAuthCallbackResponse{
		Provider: "codex", UserEmail: proto.String("user@example.com"),
	})
}

// An unset project id used to answer null; protojson drops the unset optional
// so the key is simply absent. Clients read it as a defaulting Option, so this
// is the same "no project id" they saw before.
func TestAuthProjectIDFixtures(t *testing.T) {
	checkAuthFixture(t, "project_id_unset.json", &consolev1.ProjectIDResponse{})
	checkAuthFixture(t, "project_id_set.json", &consolev1.ProjectIDResponse{
		ProjectId: proto.String("my-gcp-project"),
	})
}

// A credential can carry a token with no account email, so absent must not
// collapse into "".
func TestAuthGithubStatusFixture(t *testing.T) {
	checkAuthFixture(t, "github_status.json", &consolev1.GitHubAuthStatus{
		Connected: true, Username: proto.String("adelodunpeter"),
		Scopes: []string{"repo", "workflow"},
	})
}