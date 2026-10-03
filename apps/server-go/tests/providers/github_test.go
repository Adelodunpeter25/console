// GitHub git-credential coverage: the credential file (mode, path override,
// idempotent clear), PAT validation error mapping via httptest, and the git
// credential-helper protocol including host matching.
package tests

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/providers/github"
)

// withCredsPath points the credential file at a temp dir for one test.
func withCredsPath(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("GITHUB_CREDENTIALS_PATH", filepath.Join(dir, "github-creds.json"))
	return dir
}

// setAPIBase points github.ValidatePAT at a test server for one test.
// github.APIBase is a package-level var, so tests that run in parallel could
// race; these are all serial.
func setAPIBase(t *testing.T, url string) {
	t.Helper()
	prev := github.APIBase
	github.APIBase = url
	t.Cleanup(func() { github.APIBase = prev })
}

func TestGitHubCredentialRoundTrip(t *testing.T) {
	withCredsPath(t)

	if github.CredentialExists() {
		t.Fatal("no credential file yet")
	}
	if err := github.SaveCredential(github.Credential{
		Token: "  ghp_secret  ", Username: "@octocat", Scopes: []string{"repo"},
	}); err != nil {
		t.Fatal(err)
	}
	cred, err := github.LoadCredential()
	if err != nil {
		t.Fatal(err)
	}
	// Tokens are trimmed and the cached username normalized to a bare login.
	if cred.Token != "ghp_secret" {
		t.Fatalf("token must be trimmed, got %q", cred.Token)
	}
	if cred.Username != "octocat" {
		t.Fatalf("username must drop the @ prefix, got %q", cred.Username)
	}
	if cred.UpdatedAt.IsZero() {
		t.Fatal("UpdatedAt must be stamped on save")
	}
	if !github.CredentialExists() {
		t.Fatal("credential must report as existing")
	}
}

func TestGitHubCredentialFileMode(t *testing.T) {
	withCredsPath(t)
	if err := github.SaveCredential(github.Credential{Token: "t"}); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(github.CredentialPath())
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Fatalf("token file must be 0600, got %o", perm)
	}
	dirInfo, err := os.Stat(filepath.Dir(github.CredentialPath()))
	if err != nil {
		t.Fatal(err)
	}
	if perm := dirInfo.Mode().Perm(); perm != 0o700 {
		t.Fatalf("credential dir must be 0700, got %o", perm)
	}
}

func TestGitHubCredentialRejectsEmptyToken(t *testing.T) {
	withCredsPath(t)
	if err := github.SaveCredential(github.Credential{Token: "   "}); err == nil {
		t.Fatal("must refuse to store an empty token")
	}
}

func TestGitHubCredentialExistsIgnoresEmptyToken(t *testing.T) {
	withCredsPath(t)
	// Simulate a truncated/hand-edited file: present but unusable.
	if err := os.WriteFile(github.CredentialPath(), []byte(`{"token":""}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if github.CredentialExists() {
		t.Fatal("an empty token must not report as connected")
	}
}

func TestGitHubClearCredentialIsIdempotent(t *testing.T) {
	withCredsPath(t)
	// Clearing with no file present is success, so disconnect never 500s.
	if err := github.ClearCredential(); err != nil {
		t.Fatalf("clear on missing file: %v", err)
	}
	if err := github.SaveCredential(github.Credential{Token: "t"}); err != nil {
		t.Fatal(err)
	}
	if err := github.ClearCredential(); err != nil {
		t.Fatal(err)
	}
	if github.CredentialExists() {
		t.Fatal("credential must be gone after clear")
	}
}

func TestGitHubValidatePATSuccessAndScopes(t *testing.T) {
	withCredsPath(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer ghp_good" {
			t.Errorf("authorization header: %q", got)
		}
		w.Header().Set("X-OAuth-Scopes", "repo, gist")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"login":"octocat"}`))
	}))
	defer srv.Close()
	setAPIBase(t, srv.URL)

	login, scopes, err := github.ValidatePAT(context.Background(), srv.Client(), "ghp_good")
	if err != nil {
		t.Fatal(err)
	}
	if login != "octocat" {
		t.Fatalf("login: %q", login)
	}
	if len(scopes) != 2 || scopes[0] != "repo" || scopes[1] != "gist" {
		t.Fatalf("scopes: %v", scopes)
	}
}

// A fine-grained PAT returns no X-OAuth-Scopes header at all. That must
// succeed with a nil scope list rather than being treated as a bad token.
func TestGitHubValidatePATFineGrainedHasNoScopes(t *testing.T) {
	withCredsPath(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"login":"octocat"}`))
	}))
	defer srv.Close()
	setAPIBase(t, srv.URL)

	login, scopes, err := github.ValidatePAT(context.Background(), srv.Client(), "github_pat_fine")
	if err != nil {
		t.Fatalf("fine-grained PAT without scopes must validate: %v", err)
	}
	if login != "octocat" || scopes != nil {
		t.Fatalf("login=%q scopes=%v", login, scopes)
	}
}

func TestGitHubValidatePATRejects(t *testing.T) {
	withCredsPath(t)
	cases := []struct {
		name       string
		status     int
		body       string
		wantErr    error
		wantSubstr string
	}{
		{name: "bad token", status: 401, body: `{"message":"Bad credentials"}`, wantErr: github.ErrInvalidToken},
		{name: "rate limited", status: 403, body: `{"message":"rate limit"}`, wantErr: github.ErrUnreachable, wantSubstr: "rate limited"},
		{name: "github down", status: 503, body: `{}`, wantErr: github.ErrUnreachable, wantSubstr: "unavailable"},
		{name: "unexpected", status: 418, body: `{}`, wantSubstr: "unexpected response"},
		{name: "no login", status: 200, body: `{}`, wantSubstr: "did not return an account"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer srv.Close()
			setAPIBase(t, srv.URL)

			_, _, err := github.ValidatePAT(context.Background(), srv.Client(), "ghp_x")
			if err == nil {
				t.Fatal("expected an error")
			}
			if tc.wantErr != nil && !errors.Is(err, tc.wantErr) {
				t.Fatalf("err = %v, want %v", err, tc.wantErr)
			}
			if tc.wantSubstr != "" && !strings.Contains(err.Error(), tc.wantSubstr) {
				t.Fatalf("err = %v, want substring %q", err, tc.wantSubstr)
			}
			// A 403 must never be reported as a bad token: re-pasting a
			// working token would not help, and the route would answer 400
			// instead of 502.
			if tc.status == 403 && errors.Is(err, github.ErrInvalidToken) {
				t.Fatalf("403 must not blame the token: %v", err)
			}
		})
	}
}

func TestGitHubValidatePATUnreachable(t *testing.T) {
	withCredsPath(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	url := srv.URL
	srv.Close() // nothing is listening now

	setAPIBase(t, url)
	_, _, err := github.ValidatePAT(context.Background(), &http.Client{}, "ghp_x")
	if err == nil {
		t.Fatal("expected an error when GitHub is unreachable")
	}
	// errors.Is, not a substring match: the route maps this to 502.
	if !errors.Is(err, github.ErrUnreachable) {
		t.Fatalf("unreachable must be distinguishable from a bad token: %v", err)
	}
	if errors.Is(err, github.ErrInvalidToken) {
		t.Fatalf("unreachable must not be reported as a bad token: %v", err)
	}
}

func TestGitHubValidatePATEmptyToken(t *testing.T) {
	withCredsPath(t)
	if _, _, err := github.ValidatePAT(context.Background(), nil, "   "); err == nil {
		t.Fatal("blank token must be rejected before any network call")
	}
}

func TestGitHubParseScopes(t *testing.T) {
	if got := github.ParseScopes(""); got != nil {
		t.Fatalf("absent header must yield nil, got %v", got)
	}
	if got := github.ParseScopes("  "); got != nil {
		t.Fatalf("blank header must yield nil, got %v", got)
	}
	got := github.ParseScopes(" repo , ,gist ")
	if len(got) != 2 || got[0] != "repo" || got[1] != "gist" {
		t.Fatalf("scopes: %v", got)
	}
}

// helperGet runs the credential helper and returns its stdout.
func helperGet(t *testing.T, stdin string) string {
	t.Helper()
	var out strings.Builder
	code := github.RunCredentialHelper("get", strings.NewReader(stdin), &out)
	if code != 0 {
		t.Fatalf("helper exit code %d", code)
	}
	return out.String()
}

func TestGitHubCredentialHelperAnswersForGitHub(t *testing.T) {
	withCredsPath(t)
	if err := github.SaveCredential(github.Credential{Token: "ghp_secret"}); err != nil {
		t.Fatal(err)
	}
	out := helperGet(t, "protocol=https\nhost=github.com\npath=org/repo.git\n\n")
	if !strings.Contains(out, "username=x-access-token") {
		t.Fatalf("missing username: %q", out)
	}
	if !strings.Contains(out, "password=ghp_secret") {
		t.Fatalf("missing password: %q", out)
	}
	if !strings.HasSuffix(out, "\n\n") {
		t.Fatalf("response must be blank-line terminated: %q", out)
	}
}

func TestGitHubCredentialHelperHostMatching(t *testing.T) {
	withCredsPath(t)
	if err := github.SaveCredential(github.Credential{Token: "ghp_secret"}); err != nil {
		t.Fatal(err)
	}
	// git lowercases and may include a port; both must still match.
	for _, host := range []string{"github.com", "GitHub.com", "github.com:443"} {
		if out := helperGet(t, "protocol=https\nhost="+host+"\n\n"); !strings.Contains(out, "ghp_secret") {
			t.Fatalf("host %q must match: %q", host, out)
		}
	}
	// Anything else must stay silent so git falls through to normal behavior.
	for _, host := range []string{
		"github.com.evil.test", // lookalike must never get the token
		"evil-github.com",      //
		"notgithub.com",        //
		"gitlab.com",           //
		"github.com.evil.test:443",
	} {
		if out := helperGet(t, "protocol=https\nhost="+host+"\n\n"); out != "" {
			t.Fatalf("host %q must not receive credentials: %q", host, out)
		}
	}
}

func TestGitHubCredentialHelperProtocolHandling(t *testing.T) {
	withCredsPath(t)
	if err := github.SaveCredential(github.Credential{Token: "ghp_secret"}); err != nil {
		t.Fatal(err)
	}
	// ssh must be refused: insteadOf rewrites the URL, but defense in depth.
	if out := helperGet(t, "protocol=ssh\nhost=github.com\n\n"); out != "" {
		t.Fatalf("ssh protocol must be refused: %q", out)
	}
	// An absent protocol is tolerated (manual `git credential fill`).
	if out := helperGet(t, "host=github.com\n\n"); !strings.Contains(out, "ghp_secret") {
		t.Fatalf("absent protocol must match: %q", out)
	}
	// Unknown keys must be ignored, not rejected.
	if out := helperGet(t, "protocol=https\nhost=github.com\nwwwauth[]=basic realm=x\nusername=zzz\n\n"); !strings.Contains(out, "ghp_secret") {
		t.Fatalf("unknown keys must be ignored: %q", out)
	}
	// No host means nothing to match.
	if out := helperGet(t, "protocol=https\n\n"); out != "" {
		t.Fatalf("missing host must stay silent: %q", out)
	}
}

// With no stored token the helper must produce nothing, which makes git
// behave exactly as it does today (stock auth error, no crash, no leak).
func TestGitHubCredentialHelperSilentWithoutToken(t *testing.T) {
	withCredsPath(t)
	if out := helperGet(t, "protocol=https\nhost=github.com\n\n"); out != "" {
		t.Fatalf("no token must yield no credentials: %q", out)
	}
}

// store/erase are acknowledged no-ops: the server owns the file, so a git
// command in a terminal must not be able to overwrite or drop the token.
func TestGitHubCredentialHelperStoreEraseAreNoOps(t *testing.T) {
	withCredsPath(t)
	if err := github.SaveCredential(github.Credential{Token: "ghp_secret"}); err != nil {
		t.Fatal(err)
	}
	for _, op := range []string{"store", "erase"} {
		var out strings.Builder
		if code := github.RunCredentialHelper(op, strings.NewReader("protocol=https\nhost=github.com\npassword=attacker\n\n"), &out); code != 0 {
			t.Fatalf("%s must succeed, got %d", op, code)
		}
		if out.String() != "" {
			t.Fatalf("%s must write nothing: %q", op, out.String())
		}
	}
	cred, err := github.LoadCredential()
	if err != nil {
		t.Fatal(err)
	}
	if cred.Token != "ghp_secret" {
		t.Fatalf("token must be unchanged by store/erase, got %q", cred.Token)
	}
}

func TestGitHubCredentialHelperUnknownVerb(t *testing.T) {
	withCredsPath(t)
	var out strings.Builder
	if code := github.RunCredentialHelper("bogus", strings.NewReader(""), &out); code == 0 {
		t.Fatal("unknown verb must report failure")
	}
	if out.String() != "" {
		t.Fatalf("unknown verb must not write to the protocol stream: %q", out.String())
	}
}
