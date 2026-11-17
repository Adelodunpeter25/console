// Pull-request read-path coverage: list/detail mapping against an
// httptest GitHub (APIBase + GraphQLBase
// swap), the UNKNOWN-mergeability retry, error mapping with distinct
// messages, token safety, and golden-fixture equality with the shared
// proto/testdata/pull_requests fixtures the other clients decode.
package tests

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/db"
	consolev1 "github.com/Adelodunpeter25/console/apps/server-go/internal/gen/console/v1"
	prv "github.com/Adelodunpeter25/console/apps/server-go/internal/providers/github"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/routes"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/services"
	projectssvc "github.com/Adelodunpeter25/console/apps/server-go/internal/services/projects"
)

// graphqlReply is one canned endpoint response: status, extra headers
// (rate-limit counters), and body.
type graphqlReply struct {
	code    int
	headers map[string]string
	body    string
}

func okReply(body string) graphqlReply { return graphqlReply{code: http.StatusOK, body: body} }

// rateLimitedReply is a 403 with an exhausted rate-limit counter.
func rateLimitedReply(body string) graphqlReply {
	return graphqlReply{code: http.StatusForbidden, headers: map[string]string{
		"X-Ratelimit-Remaining": "0", "X-Ratelimit-Reset": "1893456000",
	}, body: body}
}

// newPullRequestEnv spins up an httptest GitHub answering the GraphQL and
// REST reads, points the provider package at it, stores a token, registers
// one project whose origin is github.com/octo/hello, and returns a Fiber
// app with only the pull-request routes plus the service (for tunables).
func newPullRequestEnv(t *testing.T, graphql func(query string) graphqlReply, rest func(path, body string) graphqlReply) (*fiber.App, *services.PullRequestService) {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var reply graphqlReply
		if strings.HasSuffix(r.URL.Path, "/graphql") {
			var body struct {
				Query string `json:"query"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			reply = graphql(body.Query)
		} else {
			raw, _ := io.ReadAll(r.Body)
			reply = rest(r.URL.Path, string(raw))
		}
		for k, v := range reply.headers {
			w.Header().Set(k, v)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(reply.code)
		_, _ = w.Write([]byte(reply.body))
	}))
	t.Cleanup(server.Close)

	prv.GraphQLBase = server.URL + "/graphql"
	prv.APIBase = server.URL
	t.Cleanup(func() {
		prv.GraphQLBase = "https://api.github.com/graphql"
		prv.APIBase = "https://api.github.com"
	})

	dir := t.TempDir()
	t.Setenv("GITHUB_CREDENTIALS_PATH", filepath.Join(dir, "github-creds.json"))
	if err := prv.SaveCredential(prv.Credential{Token: githubSecret, Username: "octocat", Scopes: []string{"repo"}}); err != nil {
		t.Fatal(err)
	}

	manager, err := db.Open(db.OpenOptions{Path: ":memory:"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(manager.Close)
	projects := projectssvc.NewProjectService(manager)

	// A registered project whose origin is github.com/octo/hello.
	repoDir := t.TempDir()
	gitInit := func(args ...string) {
		cmd := exec.Command("git", append([]string{"-C", repoDir}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %s: %s", strings.Join(args, " "), out)
		}
	}
	gitInit("init", "-q")
	gitInit("remote", "add", "origin", "https://github.com/octo/hello.git")
	if _, err := projects.Create(projectssvc.CreateProjectOptions{ID: "p-hello", Name: "hello", Dir: repoDir}); err != nil {
		t.Fatal(err)
	}

	svc := services.NewPullRequestService(projects)
	svc.RetryDelay = time.Millisecond
	app := fiber.New()
	routes.RegisterPullRequestRoutes(app, svc)
	return app, svc
}

// --- Canned GitHub responses. ---

const prNode1 = `{
	"number": 12, "title": "Add retry", "state": "OPEN", "isDraft": false,
	"author": {"login": "octocat"}, "updatedAt": "2026-11-01T10:00:00Z",
	"headRefName": "add-retry", "baseRefName": "main", "reviewDecision": "APPROVED",
	"additions": 42, "deletions": 7, "changedFiles": 3,
	"url": "https://github.com/octo/hello/pull/12",
	"repository": {"owner": {"login": "octo"}, "name": "hello"},
	"commits": {"nodes": [{"commit": {"statusCheckRollup": {"state": "SUCCESS"}}}]}
}`

const prNode2 = `{
	"number": 13, "title": "Fix crash", "state": "OPEN", "isDraft": true,
	"author": {"login": "octocat"}, "updatedAt": "2026-11-02T10:00:00Z",
	"headRefName": "fix-crash", "baseRefName": "main",
	"changedFiles": 1, "url": "https://github.com/octo/hello/pull/13",
	"repository": {"owner": {"login": "octo"}, "name": "hello"},
	"commits": {"nodes": [{"commit": {"statusCheckRollup": {"state": "FAILURE"}}}]}
}`

const prSearchResponse = `{"data": {"search": {"nodes": [` + prNode1 + `, ` + prNode2 + `]}}}`

const prRepoListResponse = `{"data": {"repository": {"pullRequests": {"nodes": [` + prNode1 + `, ` + prNode2 + `]}}}}`

const prDetailResponse = `{"data": {"repository": {"pullRequest": {
	"number": 12, "title": "Add retry", "state": "OPEN", "isDraft": false,
	"author": {"login": "octocat"}, "updatedAt": "2026-11-01T10:00:00Z",
	"headRefName": "add-retry", "baseRefName": "main", "reviewDecision": "APPROVED",
	"additions": 42, "deletions": 7, "changedFiles": 3,
	"url": "https://github.com/octo/hello/pull/12",
	"repository": {"owner": {"login": "octo"}, "name": "hello"},
	"commits": {"nodes": [{"commit": {"statusCheckRollup": {"state": "SUCCESS", "contexts": {"nodes": [
		{"__typename": "CheckRun", "name": "ci", "status": "COMPLETED", "conclusion": "SUCCESS", "detailsUrl": "https://example.com/ci"},
		{"__typename": "StatusContext", "name": "docs", "state": "SUCCESS", "targetUrl": "https://example.com/docs"}
	]}}}}]},
	"body": "Please review.", "headRefOid": "abc123def",
	"mergeable": "MERGEABLE", "mergeStateStatus": "CLEAN",
	"reviews": {"nodes": [{"author": {"login": "octocat"}, "state": "APPROVED", "submittedAt": "2026-11-01T11:00:00Z"}]},
	"comments": {"nodes": [{"author": {"login": "octocat"}, "body": "LGTM", "createdAt": "2026-11-01T12:00:00Z"}],
		"pageInfo": {"endCursor": "cur1", "hasNextPage": false}}
}}}}`

const prRestRepoResponse = `{"allow_merge_commit": true, "allow_squash_merge": true, "allow_rebase_merge": false}`

const prRestFilesResponse = `[
	{"filename": "src/retry.go", "status": "modified", "additions": 42, "deletions": 7,
	 "patch": "@@ -1,2 +1,2 @@\n-old\n+new"},
	{"filename": "assets/logo.png", "status": "added", "additions": 0, "deletions": 0}
]`

// newFakeGitHub wires the standard fake: GraphQL dispatched on query shape,
// REST on path.
func newFakeGitHub(t *testing.T, detail string) (*fiber.App, *services.PullRequestService) {
	t.Helper()
	return newPullRequestEnv(t,
		func(query string) graphqlReply {
			switch {
			case strings.Contains(query, "author:@me"):
				return okReply(prSearchResponse)
			case strings.Contains(query, "review-requested:@me"):
				return okReply(prSearchResponse)
			case strings.Contains(query, "pullRequest(number:"):
				return okReply(detail)
			case strings.Contains(query, "pullRequests(states:"):
				return okReply(prRepoListResponse)
			}
			return graphqlReply{code: http.StatusBadRequest, body: `{"errors":[{"message":"unknown query"}]}`}
		},
		func(path, body string) graphqlReply {
			switch {
			case strings.HasSuffix(path, "/repos/octo/hello"):
				return okReply(prRestRepoResponse)
			case strings.HasSuffix(path, "/pulls/12/files"):
				return okReply(prRestFilesResponse)
			}
			return graphqlReply{code: http.StatusNotFound, body: `{"message":"Not Found"}`}
		})
}

// pullRequestsFixture reads a golden fixture from the shared proto/testdata
// directory all three clients decode.
func pullRequestsFixture(t *testing.T, name string) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "proto", "testdata", "pull_requests", name))
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(string(raw))
}

// requireFixtureEqual compares one golden fixture against canonical
// protojson output, or regenerates it when UPDATE_PULL_REQUEST_FIXTURES=1.
// protojson deliberately injects a random space after commas, so both sides
// are compacted before comparing: the check is canonical form, not bytes.
func requireFixtureEqual(t *testing.T, name, got string) {
	t.Helper()
	if os.Getenv("UPDATE_PULL_REQUEST_FIXTURES") == "1" {
		path := filepath.Join("..", "..", "..", "..", "proto", "testdata", "pull_requests", name)
		if err := os.WriteFile(path, []byte(got+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	if want := pullRequestsFixture(t, name); got != want {
		t.Errorf("%s mismatch:\n got: %s\nwant: %s", name, got, want)
	}
}

// marshalPRJSON marshals a proto message to compact canonical JSON
// (protojson injects a random space after commas; compactJSON strips it).
func marshalPRJSON(t *testing.T, msg proto.Message) string {
	t.Helper()
	raw, err := protojson.Marshal(msg)
	if err != nil {
		t.Fatal(err)
	}
	return compactJSON(t, raw)
}

// decodePRData re-marshals the decoded envelope data and unmarshals it into
// a proto message (the do helper decodes into generic maps).
func decodePRData(t *testing.T, out map[string]any, msg proto.Message) {
	t.Helper()
	raw, err := json.Marshal(out["data"])
	if err != nil {
		t.Fatal(err)
	}
	if err := (protojson.UnmarshalOptions{DiscardUnknown: true}).Unmarshal(raw, msg); err != nil {
		t.Fatal(err)
	}
}

func TestPullRequestsRepoListAndDetailMatchFixtures(t *testing.T) {
	app, _ := newFakeGitHub(t, prDetailResponse)

	// Repo-scoped list: trimmed rows, computed checks_state/status_label.
	code, out := do(t, app, "GET", "/api/pull-requests?scope=repo&project_id=p-hello", "")
	if code != http.StatusOK || out["success"] != true {
		t.Fatalf("list: %d %v", code, out)
	}
	var list consolev1.PullRequestListResponse
	decodePRData(t, out, &list)
	if len(list.PullRequests) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(list.PullRequests))
	}
	row := list.PullRequests[0]
	if row.Number != 12 || row.State != "open" || row.Author != "octocat" ||
		row.ChecksState != "passing" || row.StatusLabel != "Approved" ||
		row.RepoOwner != "octo" || row.RepoName != "hello" {
		t.Fatalf("row mapping: %+v", row)
	}
	if row.GetAdditions() != 42 || row.GetDeletions() != 7 {
		t.Fatalf("numstat mapping: %+v", row)
	}
	requireFixtureEqual(t, "summary.json", marshalPRJSON(t, row))

	// Detail: computed merge_state, checks, reviews, comments.
	code, out = do(t, app, "GET", "/api/pull-requests/octo/hello/12", "")
	if code != http.StatusOK || out["success"] != true {
		t.Fatalf("detail: %d %v", code, out)
	}
	var detail consolev1.PullRequestDetail
	decodePRData(t, out, &detail)
	if detail.MergeState != "ready" || detail.MergeBlocker != "" {
		t.Fatalf("merge readiness: %q / %q", detail.MergeState, detail.MergeBlocker)
	}
	if detail.HeadRefOid != "abc123def" || detail.Summary.Number != 12 {
		t.Fatalf("detail mapping: %+v", &detail)
	}
	if len(detail.AllowedMergeMethods) != 2 || detail.AllowedMergeMethods[0] != "merge" ||
		detail.AllowedMergeMethods[1] != "squash" {
		t.Fatalf("merge methods: %v", detail.AllowedMergeMethods)
	}
	if len(detail.Checks) != 2 || detail.Checks[0].Name != "ci" || detail.Checks[1].Status != "success" {
		t.Fatalf("checks: %+v", detail.Checks)
	}
	requireFixtureEqual(t, "detail.json", marshalPRJSON(t, &detail))
}

func TestPullRequestsSearchScopesSpanRepos(t *testing.T) {
	for scope, wantQualifier := range map[string]string{"mine": "author:@me", "review": "review-requested:@me"} {
		t.Run(scope, func(t *testing.T) {
			seen := ""
			app, _ := newPullRequestEnv(t,
				func(query string) graphqlReply {
					seen = query
					return okReply(prSearchResponse)
				},
				func(path, body string) graphqlReply { return okReply(prRestRepoResponse) })

			code, out := do(t, app, "GET", "/api/pull-requests?scope="+scope, "")
			if code != http.StatusOK || out["success"] != true {
				t.Fatalf("%s: %d %v", scope, code, out)
			}
			if !strings.Contains(seen, wantQualifier) || !strings.Contains(seen, "is:pr") {
				t.Fatalf("%s search query missing %q: %s", scope, wantQualifier, seen)
			}
		})
	}
}

func TestPullRequestsMergeabilityUnknownRetriesOnce(t *testing.T) {
	unknownDetail := strings.ReplaceAll(prDetailResponse, `"mergeable": "MERGEABLE"`, `"mergeable": "UNKNOWN"`)
	blockedDetail := strings.ReplaceAll(prDetailResponse, `"mergeStateStatus": "CLEAN"`, `"mergeStateStatus": "BLOCKED"`)

	// First detail read reports UNKNOWN (GitHub is still computing); the
	// single re-read gets a verdict.
	reads := 0
	app, _ := newPullRequestEnv(t,
		func(query string) graphqlReply {
			if strings.Contains(query, "pullRequest(number:") {
				reads++
				if reads == 1 {
					return okReply(unknownDetail)
				}
				return okReply(blockedDetail)
			}
			return okReply(prRestRepoResponse)
		},
		func(_, _ string) graphqlReply { return okReply(prRestRepoResponse) })

	code, out := do(t, app, "GET", "/api/pull-requests/octo/hello/12", "")
	if code != http.StatusOK {
		t.Fatalf("detail: %d %v", code, out)
	}
	if reads != 2 {
		t.Fatalf("expected exactly one re-read after UNKNOWN, got %d reads", reads)
	}
	if got := out["data"].(map[string]any)["mergeState"]; got != "blocked" {
		t.Fatalf("mergeState after retry: %v", got)
	}
	// The retry resolved it, so the result must have been cached: a second
	// read serves the verdict without a new upstream call.
	if _, _ = do(t, app, "GET", "/api/pull-requests/octo/hello/12", ""); reads != 2 {
		t.Fatalf("verdict should be cached, reads = %d", reads)
	}

	// Still UNKNOWN after the single re-read: reported as checking, never
	// mergeable — and not cached, so the next read re-checks.
	reads = 0
	app2, _ := newPullRequestEnv(t,
		func(query string) graphqlReply {
			if strings.Contains(query, "pullRequest(number:") {
				reads++
				return okReply(unknownDetail)
			}
			return okReply(prRestRepoResponse)
		},
		func(_, _ string) graphqlReply { return okReply(prRestRepoResponse) })
	code, out = do(t, app2, "GET", "/api/pull-requests/octo/hello/12", "")
	if code != http.StatusOK {
		t.Fatalf("detail: %d %v", code, out)
	}
	if got := out["data"].(map[string]any)["mergeState"]; got != "checking" {
		t.Fatalf("still-UNKNOWN must report checking, got %v", got)
	}
	if reads != 2 {
		t.Fatalf("expected exactly one re-read, got %d", reads)
	}
	do(t, app2, "GET", "/api/pull-requests/octo/hello/12", "")
	if reads != 4 {
		t.Fatalf("a checking result must not be cached (reads = %d)", reads)
	}
}

func TestPullRequestsErrorMappingIsDistinct(t *testing.T) {
	cases := []struct {
		name    string
		graphql graphqlReply
		status  int
		message string
	}{
		{
			name:    "token rejected",
			graphql: graphqlReply{code: http.StatusUnauthorized, body: `{}`},
			status:  http.StatusUnauthorized,
			message: "Reconnect in Settings",
		},
		{
			name:    "rate limited",
			graphql: rateLimitedReply(`{"errors":[{"type":"RATE_LIMITED"}]}`),
			status:  http.StatusTooManyRequests,
			message: "rate limit reached",
		},
		{
			name:    "missing scope",
			graphql: graphqlReply{code: http.StatusForbidden, body: `{"errors":[{"type":"INSUFFICIENT_SCOPES"}]}`},
			status:  http.StatusForbidden,
			message: "scope",
		},
		{
			name:    "not found or no access",
			graphql: graphqlReply{code: http.StatusOK, body: `{"errors":[{"type":"NOT_FOUND","message":"Could not resolve"}]}`},
			status:  http.StatusNotFound,
			message: "no access",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			app, _ := newPullRequestEnv(t,
				func(string) graphqlReply { return tc.graphql },
				func(_, _ string) graphqlReply { return okReply(prRestRepoResponse) })
			code, out := do(t, app, "GET", "/api/pull-requests/octo/hello/12", "")
			if code != tc.status {
				t.Fatalf("status: %d, want %d (%v)", code, tc.status, out)
			}
			if msg, _ := out["error"].(string); !strings.Contains(msg, tc.message) {
				t.Fatalf("message %q missing %q", msg, tc.message)
			}
		})
	}

	// REST 422 surfaces GitHub's own validation text.
	app, _ := newPullRequestEnv(t,
		func(string) graphqlReply { return okReply(prDetailResponse) },
		func(path, body string) graphqlReply {
			if strings.HasSuffix(path, "/pulls/12/files") {
				return graphqlReply{code: http.StatusUnprocessableEntity, body: `{"message":"Validation Failed"}`}
			}
			return okReply(prRestRepoResponse)
		})
	code, out := do(t, app, "GET", "/api/pull-requests/octo/hello/12/files", "")
	if code != http.StatusUnprocessableEntity {
		t.Fatalf("422: %d %v", code, out)
	}
	if msg, _ := out["error"].(string); msg != "Validation Failed" {
		t.Fatalf("GitHub's validation text should surface verbatim, got %q", msg)
	}
}

func TestPullRequestsTokenNeverInResponses(t *testing.T) {
	app, _ := newFakeGitHub(t, prDetailResponse)
	for _, path := range []string{
		"/api/pull-requests?scope=repo&project_id=p-hello",
		"/api/pull-requests/octo/hello/12",
		"/api/pull-requests/octo/hello/12/files",
		"/api/pull-requests/octo/hello/12/files/src/retry.go",
	} {
		code, out := do(t, app, "GET", path, "")
		if code != http.StatusOK {
			t.Fatalf("%s: %d %v", path, code, out)
		}
		if body := toJSON(out); strings.Contains(body, githubSecret) {
			t.Fatalf("token leaked in %s", path)
		}
	}
}

func TestPullRequestsFilesListAndSingleDiff(t *testing.T) {
	app, _ := newFakeGitHub(t, prDetailResponse)

	// The file list never carries patches.
	code, out := do(t, app, "GET", "/api/pull-requests/octo/hello/12/files", "")
	if code != http.StatusOK {
		t.Fatalf("files: %d %v", code, out)
	}
	if body := toJSON(out); strings.Contains(body, "patch") || strings.Contains(body, "-old") {
		t.Fatalf("file list must strip patches: %s", body)
	}

	// One file's patch converts to unified diff text for the diff viewers.
	code, out = do(t, app, "GET", "/api/pull-requests/octo/hello/12/files/src/retry.go", "")
	if code != http.StatusOK {
		t.Fatalf("diff: %d %v", code, out)
	}
	data := out["data"].(map[string]any)
	diff, _ := data["diff"].(string)
	for _, want := range []string{"diff --git a/src/retry.go b/src/retry.go", "--- a/src/retry.go", "+++ b/src/retry.go", "@@ -1,2 +1,2 @@", "+new"} {
		if !strings.Contains(diff, want) {
			t.Fatalf("unified diff missing %q: %q", want, diff)
		}
	}

	// A file GitHub omitted the patch for (binary/too large) reports
	// too_large with the PR URL, not a diff (protojson omits the empty
	// string).
	code, out = do(t, app, "GET", "/api/pull-requests/octo/hello/12/files/assets/logo.png", "")
	if code != http.StatusOK {
		t.Fatalf("too-large: %d %v", code, out)
	}
	data = out["data"].(map[string]any)
	if data["tooLarge"] != true {
		t.Fatalf("too-large mapping: %v", data)
	}
	if d, present := data["diff"]; present && d != "" {
		t.Fatalf("too-large must carry no diff, got %v", d)
	}

	// A path that is not part of the PR is a clear error.
	code, _ = do(t, app, "GET", "/api/pull-requests/octo/hello/12/files/nope.go", "")
	if code != http.StatusBadRequest {
		t.Fatalf("unknown file: %d", code)
	}
}

func TestPullRequestsRepoDraftFilter(t *testing.T) {
	app, _ := newFakeGitHub(t, prDetailResponse)
	code, out := do(t, app, "GET", "/api/pull-requests?scope=repo&project_id=p-hello&draft=true", "")
	if code != http.StatusOK {
		t.Fatalf("draft=true: %d %v", code, out)
	}
	data := out["data"].(map[string]any)
	rows, _ := data["pullRequests"].([]any)
	if len(rows) != 1 {
		t.Fatalf("draft=true should keep only the draft row: %v", out["data"])
	}
}

func TestPullRequestsNotConnectedIsDistinct(t *testing.T) {
	app, _ := newFakeGitHub(t, prDetailResponse)
	// Point the credential path at an empty directory: no token stored.
	dir := t.TempDir()
	t.Setenv("GITHUB_CREDENTIALS_PATH", filepath.Join(dir, "github-creds.json"))
	code, out := do(t, app, "GET", "/api/pull-requests?scope=mine", "")
	if code != http.StatusBadRequest {
		t.Fatalf("not connected: %d %v", code, out)
	}
	if msg, _ := out["error"].(string); !strings.Contains(msg, "not connected") {
		t.Fatalf("message: %v", out)
	}
}

func TestParseGitHubRemote(t *testing.T) {
	cases := []struct {
		remote string
		owner  string
		repo   string
		ok     bool
	}{
		{"https://github.com/octo/hello.git", "octo", "hello", true},
		{"https://github.com/octo/hello", "octo", "hello", true},
		{"git@github.com:octo/hello.git", "octo", "hello", true},
		{"ssh://git@github.com/octo/hello.git", "octo", "hello", true},
		{"https://gitlab.com/octo/hello.git", "", "", false},
		{"https://github.com/octo", "", "", false},
	}
	for _, tc := range cases {
		owner, repo, ok := services.ParseGitHubRemote(tc.remote)
		if ok != tc.ok || owner != tc.owner || repo != tc.repo {
			t.Errorf("ParseGitHubRemote(%q) = %q, %q, %v", tc.remote, owner, repo, ok)
		}
	}
}

func TestPullRequestsUnknownProjectIsRejected(t *testing.T) {
	app, _ := newFakeGitHub(t, prDetailResponse)
	code, out := do(t, app, "GET", "/api/pull-requests?scope=repo&project_id=nope", "")
	if code != http.StatusBadRequest {
		t.Fatalf("unknown project: %d %v", code, out)
	}
	if msg, _ := out["error"].(string); !strings.Contains(msg, "Unknown project") {
		t.Fatalf("message: %v", out)
	}
}

// --- Write path. ---

// prWriteRecorder records what the fake GitHub received.
type prWriteRecorder struct {
	detailReads int
	createBody  map[string]any
	mergeBody   map[string]any
	mergeCalls  int
	readyCalls  int
	// Overrides for the canned responses.
	mergeStatus int
	mergeBodyFn string
	detailJSON  string
}

// newPullRequestWriteEnv extends newPullRequestEnv with write endpoints and
// a recorder.
func newPullRequestWriteEnv(t *testing.T, rec *prWriteRecorder) *fiber.App {
	t.Helper()
	if rec.detailJSON == "" {
		rec.detailJSON = prDetailResponse
	}
	app, _ := newPullRequestEnv(t,
		func(query string) graphqlReply {
			switch {
			case strings.Contains(query, "pullRequest(number:"):
				rec.detailReads++
				return okReply(rec.detailJSON)
			case strings.Contains(query, "updatePullRequest"):
				rec.readyCalls++
				return okReply(`{"data":{"updatePullRequest":{"pullRequest":{"isDraft":false}}}}`)
			}
			return graphqlReply{code: http.StatusBadRequest, body: `{"errors":[{"message":"unknown query"}]}`}
		},
		func(path, body string) graphqlReply {
			switch {
			case strings.HasSuffix(path, "/repos/octo/hello"):
				return okReply(prRestRepoResponse)
			case strings.HasSuffix(path, "/pulls/12/files"):
				return okReply(prRestFilesResponse)
			case strings.HasSuffix(path, "/pulls/12"):
				return okReply(`{"number": 12, "state": "closed", "html_url": "https://github.com/octo/hello/pull/12"}`)
			case strings.HasSuffix(path, "/pulls"):
				_ = json.Unmarshal([]byte(body), &rec.createBody)
				return okReply(`{"number": 12, "html_url": "https://github.com/octo/hello/pull/12"}`)
			case strings.HasSuffix(path, "/merge"):
				rec.mergeCalls++
				_ = json.Unmarshal([]byte(body), &rec.mergeBody)
				if rec.mergeStatus != 0 {
					return graphqlReply{code: rec.mergeStatus, body: rec.mergeBodyFn}
				}
				return okReply(`{"merged": true, "sha": "deadbeef99"}`)
			case strings.HasSuffix(path, "/comments"):
				return okReply(`{"user": {"login": "octocat"}, "body": "LGTM", "created_at": "2026-11-03T10:00:00Z"}`)
			}
			return graphqlReply{code: http.StatusNotFound, body: `{"message":"Not Found"}`}
		})
	return app
}

func TestPullRequestsCreateDefaultsToDraft(t *testing.T) {
	rec := &prWriteRecorder{}
	app := newPullRequestWriteEnv(t, rec)

	// No draft field: the create goes out as a draft.
	code, out := do(t, app, "POST", "/api/pull-requests", `{"owner":"octo","repo":"hello","title":"Add retry","body":"Please review.","head":"add-retry","base":"main"}`)
	if code != http.StatusOK || out["success"] != true {
		t.Fatalf("create: %d %v", code, out)
	}
	data := out["data"].(map[string]any)
	if data["number"] != float64(12) || data["url"] == "" {
		t.Fatalf("create payload: %v", data)
	}
	if got, _ := rec.createBody["draft"].(bool); !got {
		t.Fatalf("create must default to draft, sent %v", rec.createBody["draft"])
	}

	// An explicit draft:false opens it ready for review.
	code, _ = do(t, app, "POST", "/api/pull-requests", `{"owner":"octo","repo":"hello","title":"T","head":"h","base":"m","draft":false}`)
	if code != http.StatusOK {
		t.Fatalf("create draft:false: %d", code)
	}
	if got, _ := rec.createBody["draft"].(bool); got {
		t.Fatal("explicit draft:false must reach GitHub as false")
	}

	// title/head/base are required.
	code, out = do(t, app, "POST", "/api/pull-requests", `{"owner":"octo","repo":"hello","title":"T","head":"h"}`)
	if code != http.StatusBadRequest {
		t.Fatalf("missing base: %d %v", code, out)
	}
}

func TestPullRequestsMergeSHAGuardAndMethodCheck(t *testing.T) {
	rec := &prWriteRecorder{}
	app := newPullRequestWriteEnv(t, rec)

	// A merge without the head SHA it is supposed to land is refused before
	// anything is sent.
	code, out := do(t, app, "POST", "/api/pull-requests/octo/hello/12/merge", `{"method":"squash"}`)
	if code != http.StatusBadRequest {
		t.Fatalf("merge without expected_head: %d %v", code, out)
	}
	if rec.mergeCalls != 0 {
		t.Fatal("merge without expected_head must not reach GitHub")
	}

	// A method the repository does not allow is rejected before the request.
	code, out = do(t, app, "POST", "/api/pull-requests/octo/hello/12/merge", `{"method":"rebase","expected_head":"abc123def"}`)
	if code != http.StatusBadRequest {
		t.Fatalf("disallowed method: %d %v", code, out)
	}
	if msg, _ := out["error"].(string); !strings.Contains(msg, "rebase") || !strings.Contains(msg, "squash") {
		t.Fatalf("message should name the disallowed and the allowed methods: %v", out)
	}
	if rec.mergeCalls != 0 {
		t.Fatal("disallowed method must not reach GitHub")
	}

	// The happy path carries the SHA to GitHub and reports what was merged.
	code, out = do(t, app, "POST", "/api/pull-requests/octo/hello/12/merge", `{"method":"squash","expected_head":"abc123def"}`)
	if code != http.StatusOK || out["success"] != true {
		t.Fatalf("merge: %d %v", code, out)
	}
	if rec.mergeBody["sha"] != "abc123def" || rec.mergeBody["merge_method"] != "squash" {
		t.Fatalf("merge body = %v", rec.mergeBody)
	}
	if out["data"].(map[string]any)["mergedSha"] != "deadbeef99" {
		t.Fatalf("merge payload: %v", out["data"])
	}

	// A moved head: GitHub's 409 is a refusal with a reload message.
	rec2 := &prWriteRecorder{mergeStatus: http.StatusConflict, mergeBodyFn: `{"message":"head was not at expected"}`}
	app2 := newPullRequestWriteEnv(t, rec2)
	code, out = do(t, app2, "POST", "/api/pull-requests/octo/hello/12/merge", `{"method":"merge","expected_head":"abc123def"}`)
	if code != http.StatusConflict {
		t.Fatalf("409: %d %v", code, out)
	}
	if msg, _ := out["error"].(string); !strings.Contains(msg, "Reload") {
		t.Fatalf("conflict message: %v", out)
	}

	// Not mergeable: GitHub's 405 wording surfaces verbatim.
	rec3 := &prWriteRecorder{mergeStatus: http.StatusMethodNotAllowed, mergeBodyFn: `{"message":"Pull Request is not mergeable"}`}
	app3 := newPullRequestWriteEnv(t, rec3)
	code, out = do(t, app3, "POST", "/api/pull-requests/octo/hello/12/merge", `{"method":"merge","expected_head":"abc123def"}`)
	if code != http.StatusUnprocessableEntity {
		t.Fatalf("405: %d %v", code, out)
	}
	if msg, _ := out["error"].(string); msg != "Pull Request is not mergeable" {
		t.Fatalf("GitHub's 405 text should surface verbatim, got %q", msg)
	}
}

func TestPullRequestsMergeRefusesDraftEarly(t *testing.T) {
	draftDetail := strings.ReplaceAll(prDetailResponse, `"isDraft": false`, `"isDraft": true`)
	rec := &prWriteRecorder{detailJSON: draftDetail}
	app := newPullRequestWriteEnv(t, rec)

	code, out := do(t, app, "POST", "/api/pull-requests/octo/hello/12/merge", `{"method":"merge","expected_head":"abc123def"}`)
	if code != http.StatusBadRequest {
		t.Fatalf("draft merge: %d %v", code, out)
	}
	if msg, _ := out["error"].(string); !strings.Contains(msg, "mark ready") {
		t.Fatalf("message: %v", out)
	}
	if rec.mergeCalls != 0 {
		t.Fatal("a draft must be refused before any merge request")
	}
}

func TestPullRequestsUpdateAndCommentAndInvalidation(t *testing.T) {
	rec := &prWriteRecorder{}
	app := newPullRequestWriteEnv(t, rec)

	// A first detail read warms the cache; the second is served from it.
	do(t, app, "GET", "/api/pull-requests/octo/hello/12", "")
	do(t, app, "GET", "/api/pull-requests/octo/hello/12", "")
	if rec.detailReads != 1 {
		t.Fatalf("detail should be cached (reads = %d)", rec.detailReads)
	}

	// Marking ready goes through the draft mutation; the refreshed detail
	// comes back in the same response.
	code, out := do(t, app, "PATCH", "/api/pull-requests/octo/hello/12", `{"draft":false}`)
	if code != http.StatusOK || out["success"] != true {
		t.Fatalf("ready: %d %v", code, out)
	}
	if rec.readyCalls != 1 {
		t.Fatalf("ready must use the draft mutation, calls = %d", rec.readyCalls)
	}
	if out["data"].(map[string]any)["detail"] == nil {
		t.Fatalf("update must return the refreshed detail: %v", out["data"])
	}

	// A state change goes through PATCH with only the state field.
	code, out = do(t, app, "PATCH", "/api/pull-requests/octo/hello/12", `{"state":"closed"}`)
	if code != http.StatusOK {
		t.Fatalf("close: %d %v", code, out)
	}

	// A comment is created and echoed back.
	code, out = do(t, app, "POST", "/api/pull-requests/octo/hello/12/comments", `{"body":"LGTM"}`)
	if code != http.StatusOK {
		t.Fatalf("comment: %d %v", code, out)
	}
	comment := out["data"].(map[string]any)["comment"].(map[string]any)
	if comment["author"] != "octocat" || comment["body"] != "LGTM" {
		t.Fatalf("comment payload: %v", comment)
	}

	// Every write invalidated the cache: the next detail read goes upstream
	// again.
	if _, err := app.Test(httptest.NewRequest("GET", "/api/pull-requests/octo/hello/12", nil), 10000); err != nil {
		t.Fatal(err)
	}
	if rec.detailReads <= 1 {
		t.Fatalf("writes must invalidate the cached detail (reads = %d)", rec.detailReads)
	}

	// The token never appears in a write response.
	for _, tc := range []struct{ method, path, body string }{
		{"POST", "/api/pull-requests", `{"owner":"octo","repo":"hello","title":"T","head":"h","base":"m"}`},
		{"PATCH", "/api/pull-requests/octo/hello/12", `{"state":"open"}`},
		{"POST", "/api/pull-requests/octo/hello/12/merge", `{"method":"merge","expected_head":"abc123def"}`},
		{"POST", "/api/pull-requests/octo/hello/12/comments", `{"body":"hi"}`},
	} {
		code, out := do(t, app, tc.method, tc.path, tc.body)
		if code != http.StatusOK {
			t.Fatalf("%s %s: %d %v", tc.method, tc.path, code, out)
		}
		if body := toJSON(out); strings.Contains(body, githubSecret) {
			t.Fatalf("token leaked in %s %s", tc.method, tc.path)
		}
	}
}
