// console.pr.* coverage: repo defaults from the session's project, the
// draft-by-default create, merge filling the head SHA from a prior read,
// and results carrying the server-computed merge_blocker. The fake GitHub
// records what the kernel actually sent.
package console

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/db"
	prv "github.com/Adelodunpeter25/console/apps/server-go/internal/providers/github"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/services"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/services/projects"
)

// prFakeGitHub records the write bodies the kernel sends.
type prFakeGitHub struct {
	detailReads int
	createBody  map[string]any
	mergeBody   map[string]any
	mergeCalls  int
	readyCalls  int
	detailJSON  string
}

// newPRRuntime builds a console kernel whose project p-hello resolves to
// github.com/octo/hello, against a fake GitHub with the given detail
// response.
func newPRRuntime(t *testing.T, fake *prFakeGitHub) *Runtime {
	t.Helper()
	if fake.detailJSON == "" {
		fake.detailJSON = prFakeDetail
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, "/graphql"):
			var body struct {
				Query string `json:"query"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			if strings.Contains(body.Query, "pullRequest(number:") {
				fake.detailReads++
				_, _ = w.Write([]byte(fake.detailJSON))
				return
			}
			if strings.Contains(body.Query, "updatePullRequest") {
				fake.readyCalls++
				_, _ = w.Write([]byte(`{"data":{"updatePullRequest":{"pullRequest":{"isDraft":false}}}}`))
				return
			}
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"errors":[{"message":"unknown query"}]}`))
		case r.Method == "POST" && strings.HasSuffix(r.URL.Path, "/pulls"):
			_ = json.NewDecoder(r.Body).Decode(&fake.createBody)
			_, _ = w.Write([]byte(`{"number":12,"html_url":"https://github.com/octo/hello/pull/12"}`))
		case r.Method == "PUT" && strings.HasSuffix(r.URL.Path, "/merge"):
			fake.mergeCalls++
			_ = json.NewDecoder(r.Body).Decode(&fake.mergeBody)
			_, _ = w.Write([]byte(`{"merged":true,"sha":"deadbeef99"}`))
		case r.Method == "GET" && strings.HasSuffix(r.URL.Path, "/repos/octo/hello"):
			_, _ = w.Write([]byte(`{"allow_merge_commit":true,"allow_squash_merge":true,"allow_rebase_merge":false}`))
		default:
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{}`))
		}
	}))
	t.Cleanup(server.Close)
	prv.GraphQLBase = server.URL + "/graphql"
	prv.APIBase = server.URL
	t.Cleanup(func() {
		prv.GraphQLBase = "https://api.github.com/graphql"
		prv.APIBase = "https://api.github.com"
	})

	storage := t.TempDir()
	t.Setenv("GITHUB_CREDENTIALS_PATH", filepath.Join(storage, "github-creds.json"))
	if err := prv.SaveCredential(prv.Credential{Token: "ghp_kernel_test_token", Username: "octocat"}); err != nil {
		t.Fatal(err)
	}

	database, err := db.Open(db.OpenOptions{Path: filepath.Join(storage, "global.db"), StorageDir: storage})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(database.Close)
	projectsSvc := projects.NewProjectService(database)

	repoDir := t.TempDir()
	gitInit := func(args ...string) {
		cmd := exec.Command("git", append([]string{"-C", repoDir}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %s: %s", strings.Join(args, " "), out)
		}
	}
	gitInit("init", "-q")
	gitInit("remote", "add", "origin", "https://github.com/octo/hello.git")
	if _, err := projectsSvc.Create(projects.CreateProjectOptions{ID: "p-hello", Name: "hello", Dir: repoDir}); err != nil {
		t.Fatal(err)
	}

	pulls := services.NewPullRequestService(projectsSvc)
	pulls.RetryDelay = time.Millisecond
	runtime, err := NewRuntime(nil, nil, nil, projectsSvc, nil, nil, nil, nil, nil, QueueControl{}, "p-hello", repoDir)
	if err != nil {
		t.Fatal(err)
	}
	runtime.SetPullRequests(pulls)
	return runtime
}

const prFakeDetail = `{"data": {"repository": {"pullRequest": {
	"id": "PR_node_1",
	"number": 12, "title": "Add retry", "state": "OPEN", "isDraft": false,
	"author": {"login": "octocat"}, "updatedAt": "2026-11-01T10:00:00Z",
	"headRefName": "add-retry", "baseRefName": "main", "reviewDecision": "APPROVED",
	"additions": 42, "deletions": 7, "changedFiles": 3,
	"url": "https://github.com/octo/hello/pull/12",
	"repository": {"owner": {"login": "octo"}, "name": "hello"},
	"commits": {"nodes": [{"commit": {"statusCheckRollup": {"state": "SUCCESS", "contexts": {"nodes": [
		{"__typename": "CheckRun", "name": "ci", "status": "COMPLETED", "conclusion": "SUCCESS", "detailsUrl": "https://example.com/ci"}
	]}}}}]},
	"body": "Please review.", "headRefOid": "abc123def",
	"mergeable": "MERGEABLE", "mergeStateStatus": "CLEAN",
	"reviews": {"nodes": []},
	"comments": {"nodes": [], "pageInfo": {"endCursor": "", "hasNextPage": false}}
}}}}`

func TestConsolePRCreateDefaultsToDraftFromProjectRepo(t *testing.T) {
	fake := &prFakeGitHub{}
	runtime := newPRRuntime(t, fake)

	// No owner/repo args: they resolve from the session's project origin.
	// No draft arg: the create goes out as a draft.
	created := evalJSON(t, runtime, `console.pr.create({title:"Add retry", head:"add-retry", base:"main"})`).(map[string]any)
	if created["number"] != float64(12) {
		t.Fatalf("created = %#v", created)
	}
	if got, _ := fake.createBody["draft"].(bool); !got {
		t.Fatalf("agent-created PR must default to draft, sent %v", fake.createBody["draft"])
	}
	if fake.createBody["head"] != "add-retry" || fake.createBody["base"] != "main" {
		t.Fatalf("create body = %#v", fake.createBody)
	}

	// An explicit draft:false opens it ready for review.
	if _, err := runtime.Eval(t.Context(), `console.pr.create({title:"x", head:"h", base:"m", draft:false})`, time.Second); err != nil {
		t.Fatal(err)
	}
	if got, _ := fake.createBody["draft"].(bool); got {
		t.Fatal("explicit draft:false must reach GitHub as false")
	}
}

func TestConsolePRMergeFillsHeadSHAFromRead(t *testing.T) {
	fake := &prFakeGitHub{}
	runtime := newPRRuntime(t, fake)

	// No expected_head: the kernel reads the PR and merges the exact commit
	// it saw; the result reports the SHA GitHub merged.
	merged := evalJSON(t, runtime, `console.pr.merge({number:12, method:"squash"})`).(map[string]any)
	if merged["mergedSha"] != "deadbeef99" {
		t.Fatalf("merge result = %#v", merged)
	}
	if fake.mergeBody["sha"] != "abc123def" {
		t.Fatalf("merge must carry the read head SHA, sent %v", fake.mergeBody["sha"])
	}
	if fake.mergeBody["merge_method"] != "squash" {
		t.Fatalf("merge method = %v", fake.mergeBody["merge_method"])
	}
	if fake.detailReads < 1 {
		t.Fatal("merge without expected_head must read the PR first")
	}
}

func TestConsolePRReadCarriesMergeBlocker(t *testing.T) {
	// One failing check, branch-protection BLOCKED: the result must say why
	// a merge cannot proceed, so the model is told rather than retrying.
	blocked := strings.NewReplacer(
		`"mergeable": "MERGEABLE"`, `"mergeable": "CONFLICTING"`,
		`"mergeStateStatus": "CLEAN"`, `"mergeStateStatus": "BLOCKED"`,
		`"conclusion": "SUCCESS"`, `"conclusion": "FAILURE"`,
	).Replace(prFakeDetail)
	fake := &prFakeGitHub{detailJSON: blocked}
	runtime := newPRRuntime(t, fake)

	read := evalJSON(t, runtime, `console.pr.read({number:12})`).(map[string]any)
	if read["mergeState"] != "blocked" {
		t.Fatalf("mergeState = %#v", read["mergeState"])
	}
	if blocker, _ := read["mergeBlocker"].(string); !strings.Contains(blocker, "1 check failing") {
		t.Fatalf("mergeBlocker = %#v", read["mergeBlocker"])
	}
}

func TestConsolePRReadyFlipsDraft(t *testing.T) {
	fake := &prFakeGitHub{}
	runtime := newPRRuntime(t, fake)

	evalJSON(t, runtime, `console.pr.ready({number:12})`)
	if fake.readyCalls != 1 {
		t.Fatalf("ready must go through the draft mutation, calls = %d", fake.readyCalls)
	}
}

// The kernel methods run under the session's project: an unknown PR number
// against the fake is a clean error, not a panic.
func TestConsolePRUnknownNumberIsCleanError(t *testing.T) {
	fake := &prFakeGitHub{detailJSON: `{"errors":[{"type":"NOT_FOUND","message":"Could not resolve"}]}`}
	runtime := newPRRuntime(t, fake)
	if _, err := runtime.Eval(t.Context(), `console.pr.read({number:99})`, time.Second); err == nil {
		t.Fatal("unknown PR must error")
	}
}
