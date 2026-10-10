// Path guard: writes are confined to the session's project root, symlink and
// batch escapes are rejected, and glob/grep stop defaulting to the server's
// launch directory. Reads stay unconfined on purpose.
package tests

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/loop"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/permissions"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/tools"
	"github.com/Adelodunpeter25/console/apps/server-go/tests/helpers"
)

// guardProject builds <tmp>/project with an outside sibling <tmp>/outside,
// resolving symlinks the way a real checkout path may need (/tmp on macOS).
func guardProject(t *testing.T) (*permissions.Guard, string, string) {
	t.Helper()
	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatalf("resolve temp dir: %v", err)
	}
	project := filepath.Join(base, "project")
	outside := filepath.Join(base, "outside")
	for _, dir := range []string{project, outside} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
	}
	return permissions.NewGuard(project), project, outside
}

func guardCall(t *testing.T, g *permissions.Guard, name string, args map[string]any) (json.RawMessage, error) {
	t.Helper()
	raw, err := json.Marshal(args)
	if err != nil {
		t.Fatalf("marshal args: %v", err)
	}
	return g.Check(tools.ToolCall{Name: name, Arguments: raw})
}

func pathOf(t *testing.T, raw json.RawMessage) string {
	t.Helper()
	var fields struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal(raw, &fields); err != nil {
		t.Fatalf("decode result: %v", err)
	}
	return fields.Path
}

func TestPathGuardAllowsWriteInsideRoot(t *testing.T) {
	g, project, _ := guardProject(t)
	target := filepath.Join(project, "src", "main.go")
	out, err := guardCall(t, g, "write_file", map[string]any{"path": target, "content": "x"})
	if err != nil {
		t.Fatalf("write inside root should pass: %v", err)
	}
	if got := pathOf(t, out); got != target {
		t.Errorf("path rewritten to %q, want %q", got, target)
	}
}

func TestPathGuardRejectsSiblingProject(t *testing.T) {
	g, _, outside := guardProject(t)
	_, err := guardCall(t, g, "write_file", map[string]any{"path": filepath.Join(outside, "secret.txt")})
	if err == nil {
		t.Fatal("write outside the project should be rejected")
	}
	if !strings.Contains(err.Error(), g.Root()) {
		t.Errorf("error should name the project root %q, got: %v", g.Root(), err)
	}
}

func TestPathGuardRejectsParentEscape(t *testing.T) {
	g, project, _ := guardProject(t)
	_, err := guardCall(t, g, "editFile", map[string]any{"path": filepath.Join(project, "..", "escaped.txt")})
	if err == nil {
		t.Fatal("../ escape should be rejected")
	}
}

func TestPathGuardRejectsRelativeWrite(t *testing.T) {
	g, _, _ := guardProject(t)
	_, err := guardCall(t, g, "write_file", map[string]any{"path": "src/main.go"})
	if err == nil {
		t.Fatal("relative write path should be rejected")
	}
	if !strings.Contains(err.Error(), "must be absolute") {
		t.Errorf("error should explain the absolute-path requirement, got: %v", err)
	}
}

func TestPathGuardRejectsSymlinkEscape(t *testing.T) {
	g, project, outside := guardProject(t)
	link := filepath.Join(project, "escape")
	if err := os.Symlink(outside, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	_, err := guardCall(t, g, "write_file", map[string]any{"path": filepath.Join(link, "stolen.txt")})
	if err == nil {
		t.Fatal("write through an outward symlink should be rejected")
	}
}

func TestPathGuardAllowsSymlinkInsideRoot(t *testing.T) {
	g, project, _ := guardProject(t)
	real := filepath.Join(project, "real")
	if err := os.MkdirAll(real, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	link := filepath.Join(project, "link")
	if err := os.Symlink(real, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if _, err := guardCall(t, g, "write_file", map[string]any{"path": filepath.Join(link, "ok.txt")}); err != nil {
		t.Fatalf("symlink staying inside the project should pass: %v", err)
	}
}

func TestPathGuardBatchWriteIsAllOrNothing(t *testing.T) {
	g, project, outside := guardProject(t)
	out, err := guardCall(t, g, "batchWrite", map[string]any{
		"files": []map[string]any{
			{"path": filepath.Join(project, "a.txt"), "content": "a"},
			{"path": filepath.Join(outside, "b.txt"), "content": "b"},
		},
	})
	if err == nil {
		t.Fatal("batch with one outside path should be rejected whole")
	}
	if !strings.Contains(err.Error(), "No files were written") {
		t.Errorf("error should state nothing was written, got: %v", err)
	}
	if out != nil {
		t.Error("rejected batch must not return usable arguments")
	}
}

func TestPathGuardBatchWriteAllInside(t *testing.T) {
	g, project, _ := guardProject(t)
	if _, err := guardCall(t, g, "batchWrite", map[string]any{
		"files": []map[string]any{
			{"path": filepath.Join(project, "a.txt"), "content": "a"},
			{"path": filepath.Join(project, "b.txt"), "content": "b"},
		},
	}); err != nil {
		t.Fatalf("batch entirely inside the project should pass: %v", err)
	}
}

func TestPathGuardDefaultsGlobAndGrepRoot(t *testing.T) {
	g, _, _ := guardProject(t)
	for _, tool := range []string{"glob", "grep"} {
		out, err := guardCall(t, g, tool, map[string]any{"pattern": "*.go"})
		if err != nil {
			t.Fatalf("%s: %v", tool, err)
		}
		var fields struct {
			Root string `json:"root"`
		}
		if err := json.Unmarshal(out, &fields); err != nil {
			t.Fatalf("%s decode: %v", tool, err)
		}
		if fields.Root != g.Root() {
			t.Errorf("%s root defaulted to %q, want the project %q", tool, fields.Root, g.Root())
		}
	}
}

func TestPathGuardKeepsExplicitGlobRoot(t *testing.T) {
	g, _, _ := guardProject(t)
	explicit := filepath.Join(t.TempDir(), "elsewhere")
	out, err := guardCall(t, g, "glob", map[string]any{"pattern": "*", "root": explicit})
	if err != nil {
		t.Fatalf("explicit read root must not be a guard failure: %v", err)
	}
	var fields struct {
		Root string `json:"root"`
	}
	if err := json.Unmarshal(out, &fields); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if fields.Root != explicit {
		t.Errorf("explicit root was rewritten to %q, want %q", fields.Root, explicit)
	}
}

func TestPathGuardLeavesReadsUnconfined(t *testing.T) {
	g, _, outside := guardProject(t)
	target := filepath.Join(outside, "notes.md")
	// Reads are deliberately unconfined; only writes are guarded.
	if _, err := guardCall(t, g, "read_file", map[string]any{"path": target}); err != nil {
		t.Errorf("read_file outside the project should stay allowed, got: %v", err)
	}
	if _, err := guardCall(t, g, "list_dir", map[string]any{"path": outside}); err != nil {
		t.Errorf("list_dir outside the project should stay allowed, got: %v", err)
	}
}

func TestPathGuardIgnoresToolsWithoutPaths(t *testing.T) {
	g, _, _ := guardProject(t)
	for _, tool := range []string{"todo", "ask", "webSearch", "bash", "bashJob", "subagent"} {
		if _, err := guardCall(t, g, tool, map[string]any{"action": "list"}); err != nil {
			t.Errorf("%s should pass through untouched, got: %v", tool, err)
		}
	}
}

func TestPathGuardNilIsNoOp(t *testing.T) {
	var g *permissions.Guard
	if got := g.Root(); got != "" {
		t.Errorf("nil guard root should be empty, got %q", got)
	}
	if permissions.NewGuard("") != nil {
		t.Error("NewGuard(\"\") should return nil so the run is not failed")
	}
	raw, err := guardCall(t, g, "write_file", map[string]any{"path": "/etc/passwd"})
	if err != nil || raw == nil {
		t.Errorf("nil guard should pass arguments through unchanged, got %v %v", raw, err)
	}
}

func TestPathGuardMalformedArgumentsReachTheTool(t *testing.T) {
	g, _, _ := guardProject(t)
	call := tools.ToolCall{Name: "write_file", Arguments: json.RawMessage(`"not an object"`)}
	out, err := g.Check(call)
	if err != nil {
		t.Fatalf("malformed args should fall through to the tool, got: %v", err)
	}
	if string(out) != `"not an object"` {
		t.Errorf("arguments were rewritten: %s", out)
	}
}

// TestExecutorGuardBlocksWriteOutright runs the guard through the executor to
// prove a rejected call becomes a tool-scoped isError result and the run
// continues, rather than aborting the turn.
func TestExecutorGuardBlocksWriteOutright(t *testing.T) {
	g, project, outside := guardProject(t)
	executor := loop.NewExecutor(tools.NewRegistry(tools.DefaultTools()...), permissions.FullAccess, helpers.AutoApprover{})
	executor.Guard = g

	target := filepath.Join(outside, "hijacked.txt")
	args, _ := json.Marshal(map[string]any{"path": target, "content": "owned"})
	res, err := executor.Execute(t.Context(), tools.ToolCall{ID: "call_1", Name: "write_file", Arguments: args})
	if err != nil {
		t.Fatalf("a denied write must not fail the run: %v", err)
	}
	if !res.IsError {
		t.Fatal("write outside the project should return an error result")
	}
	if _, statErr := os.Stat(target); statErr == nil {
		t.Error("the file outside the project must not exist")
	}

	inside := filepath.Join(project, "allowed.txt")
	args, _ = json.Marshal(map[string]any{"path": inside, "content": "fine"})
	res, err = executor.Execute(t.Context(), tools.ToolCall{ID: "call_2", Name: "write_file", Arguments: args})
	if err != nil {
		t.Fatalf("write inside the project: %v", err)
	}
	if res.IsError {
		t.Fatalf("write inside the project was blocked: %v", res.Content)
	}
	if data, readErr := os.ReadFile(inside); readErr != nil || string(data) != "fine" {
		t.Errorf("allowed write did not land: %v %q", readErr, data)
	}
}

// TestExecutorGuardRewritesGrepRoot proves the default reaches the tool: a
// grep with no root must search the project, not the process launch dir.
func TestExecutorGuardRewritesGrepRoot(t *testing.T) {
	g, project, _ := guardProject(t)
	if err := os.WriteFile(filepath.Join(project, "marker.go"), []byte("package marker\n"), 0o644); err != nil {
		t.Fatalf("seed file: %v", err)
	}
	executor := loop.NewExecutor(tools.NewRegistry(tools.DefaultTools()...), permissions.FullAccess, helpers.AutoApprover{})
	executor.Guard = g

	args, _ := json.Marshal(map[string]any{"pattern": "marker"})
	res, err := executor.Execute(t.Context(), tools.ToolCall{ID: "call_1", Name: "grep", Arguments: args})
	if err != nil {
		t.Fatalf("grep: %v", err)
	}
	if res.IsError {
		t.Fatalf("grep failed: %v", res.Content)
	}
	if !strings.Contains(toolText(t, res.Content), "marker.go") {
		t.Errorf("grep should have found the seeded file in the project, got: %s", toolText(t, res.Content))
	}
}

func toolText(t *testing.T, content any) string {
	t.Helper()
	parts, ok := content.([]map[string]any)
	if !ok {
		t.Fatalf("unexpected tool result content %T", content)
	}
	var sb strings.Builder
	for _, p := range parts {
		if s, isString := p["text"].(string); isString {
			sb.WriteString(s)
		}
	}
	return sb.String()
}
