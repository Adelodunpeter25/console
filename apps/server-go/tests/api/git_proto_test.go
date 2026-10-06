// Git migration coverage: proto-built payloads must match the shared golden
// fixtures, and the status/diff/branches/checkout routes keep their semantics.
package tests

import (
	"encoding/json"
	"io"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	consolev1 "github.com/Adelodunpeter25/console/apps/server-go/internal/gen/console/v1"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/routes"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/services"
)

func gitFixture(t *testing.T, name string) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "proto", "testdata", "git", name))
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(string(raw))
}

func TestGitProtoMatchesFixtures(t *testing.T) {
	zero := uint32(0)
	twelve := uint32(12)
	four := uint32(4)
	check := func(name string, msg proto.Message) {
		t.Helper()
		raw, err := protojson.Marshal(msg)
		if err != nil {
			t.Fatal(err)
		}
		if compactJSON(t, raw) != gitFixture(t, name) {
			t.Fatalf("%s drifted:\n got %s\nwant %s", name, compactJSON(t, raw), gitFixture(t, name))
		}
	}

	check("status.json", &consolev1.GitStatusSummary{
		Branch: "main",
		Files: []*consolev1.GitFileEntry{
			{Path: "/r/a.go", Status: "M", Staged: true, Additions: &twelve, Deletions: &four},
			{Path: "/r/b.go", Status: "?", Additions: &zero, Deletions: &zero},
		},
	})
	check("branches.json", &consolev1.GitBranchesResponse{
		Branches: []*consolev1.GitBranchInfo{
			{Name: "main", Current: true},
			{Name: "dev"},
		},
		IsGitRepository: true,
	})
	check("diff.json", &consolev1.GitDiffResponse{
		Path: "/r/a.go", Diff: "--- a/a.go\n+++ b/a.go\n",
	})
	check("checkout.json", &consolev1.GitCheckoutResponse{Branch: "dev"})

	// Fixtures stay parseable with unknown-field tolerance.
	var decoded consolev1.GitStatusSummary
	unmarshal := protojson.UnmarshalOptions{DiscardUnknown: true}
	if err := unmarshal.Unmarshal([]byte(gitFixture(t, "status.json")), &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.GetBranch() != "main" || len(decoded.GetFiles()) != 2 {
		t.Fatalf("decoded: %+v", &decoded)
	}
}

func TestGitRoutesStatusAndCheckout(t *testing.T) {
	repo := gitRepo(t)
	if err := os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte("one\ntwo\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	watch, err := services.NewFsWatchService()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(watch.Close)
	app := fiber.New()
	routes.RegisterGitRoutes(app, services.NewGitService(), watch)

	get := func(target string) (int, map[string]any) {
		req := httptest.NewRequest("GET", target+"?path="+repo, nil)
		resp, err := app.Test(req, 10000)
		if err != nil {
			t.Fatal(err)
		}
		raw, _ := io.ReadAll(resp.Body)
		var envelope struct {
			Success bool            `json:"success"`
			Data    json.RawMessage `json:"data"`
		}
		if err := json.Unmarshal(raw, &envelope); err != nil || !envelope.Success {
			t.Fatalf("envelope: %s", raw)
		}
		var data map[string]any
		if err := json.Unmarshal(envelope.Data, &data); err != nil {
			t.Fatalf("data: %s", envelope.Data)
		}
		return resp.StatusCode, data
	}

	// Untracked file shows with string status and numeric counts.
	if _, data := get("/api/git/status"); data["branch"] != "main" {
		t.Fatalf("branch: %v", data)
	} else {
		files, _ := data["files"].([]any)
		if len(files) != 1 {
			t.Fatalf("files: %v", data)
		}
		entry := files[0].(map[string]any)
		if _, isString := entry["status"].(string); !isString {
			t.Fatalf("status must stay a string: %v", entry)
		}
		if _, isNumber := entry["additions"].(float64); !isNumber {
			t.Fatalf("additions must stay a number: %v", entry)
		}
	}

	// Branches list the current branch; checkout moves it.
	if _, data := get("/api/git/branches"); data["isGitRepository"] != true {
		t.Fatalf("branches: %v", data)
	}
	// The target branch must exist: checkout does not create it.
	branchCmd := exec.Command("git", "branch", "feature")
	branchCmd.Dir = repo
	if out, err := branchCmd.CombinedOutput(); err != nil {
		t.Fatalf("git branch: %s", out)
	}
	checkout := httptest.NewRequest("POST", "/api/git/checkout",
		strings.NewReader(`{"path":"`+repo+`","branch":"feature"}`))
	checkout.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(checkout, 10000)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := io.ReadAll(resp.Body)
	var checkoutEnv struct {
		Success bool            `json:"success"`
		Data    json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(raw, &checkoutEnv); err != nil || !checkoutEnv.Success {
		t.Fatalf("checkout envelope: %s", raw)
	}
	var out map[string]any
	if err := json.Unmarshal(checkoutEnv.Data, &out); err != nil || out["branch"] != "feature" {
		t.Fatalf("checkout data: %s", checkoutEnv.Data)
	}

	// Validation preserved.
	bad := httptest.NewRequest("GET", "/api/git/status", nil)
	if resp, err := app.Test(bad, 10000); err != nil || resp.StatusCode != 400 {
		t.Fatalf("missing path must 400: %v", resp)
	}
}
