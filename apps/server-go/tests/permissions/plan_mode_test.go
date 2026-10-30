// Plan mode is read-only by enforcement, not by request: write and exec tools
// are denied, read tools run, and writePlan is the one write that gets through,
// confined to .console/plans/.
package tests

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/loop"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/permissions"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/tools"
)

type denyApprover struct{ asked *int }

func (d denyApprover) Approve(context.Context, permissions.Request) (bool, error) {
	*d.asked++
	return true, nil
}

func planCall(t *testing.T, name string, args any) tools.ToolCall {
	t.Helper()
	raw, err := json.Marshal(args)
	if err != nil {
		t.Fatal(err)
	}
	return tools.ToolCall{ID: "c1", Name: name, Arguments: raw}
}

func TestPlanModeDeniesWritesAndExecButNeverPrompts(t *testing.T) {
	dir := t.TempDir()
	asked := 0
	registry := tools.NewRegistry(tools.WriteFile, tools.EditFile, tools.BatchWrite, tools.Bash)
	exec := loop.NewExecutor(registry, permissions.PlanMode, denyApprover{&asked})

	for _, call := range []tools.ToolCall{
		planCall(t, "write_file", map[string]any{"path": filepath.Join(dir, "a.txt"), "content": "x"}),
		planCall(t, "editFile", map[string]any{"path": filepath.Join(dir, "a.txt"), "oldContent": "a", "newContent": "b"}),
		planCall(t, "batchWrite", map[string]any{"files": []map[string]any{{"path": filepath.Join(dir, "b.txt"), "content": "x"}}}),
		planCall(t, "bash", map[string]any{"command": "touch " + filepath.Join(dir, "c.txt")}),
	} {
		res, err := exec.Execute(context.Background(), call)
		if err != nil {
			t.Fatal(err)
		}
		if !res.IsError || !strings.Contains(res.Content.(string), "plan mode is read-only") {
			t.Errorf("%s: want a plan-mode deny, got %#v", call.Name, res)
		}
	}
	if asked != 0 {
		t.Fatalf("plan mode must deny, not prompt: approver asked %d times", asked)
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 0 {
		t.Fatalf("a denied call still wrote files: %v", entries)
	}
}

func TestPlanModeStillRunsReadTools(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	exec := loop.NewExecutor(tools.NewRegistry(tools.ReadFile, tools.ListDir), permissions.PlanMode, nil)
	res, err := exec.Execute(context.Background(), planCall(t, "read_file", map[string]any{"path": filepath.Join(dir, "notes.txt")}))
	if err != nil || res.IsError {
		t.Fatalf("read_file must work in plan mode: %v %#v", err, res)
	}
}

func TestPlanModeAllowsWritePlanOnly(t *testing.T) {
	dir := t.TempDir()
	exec := loop.NewExecutor(tools.NewRegistry(tools.NewWritePlanTool(dir), tools.WriteFile), permissions.PlanMode, nil)

	res, err := exec.Execute(context.Background(), planCall(t, "writePlan", map[string]any{"name": "Auth Refactor", "content": "# Plan\n1. do it"}))
	if err != nil || res.IsError {
		t.Fatalf("writePlan must be allowed in plan mode: %v %#v", err, res)
	}
	got, err := os.ReadFile(filepath.Join(dir, ".console", "plans", "auth-refactor.md"))
	if err != nil || string(got) != "# Plan\n1. do it" {
		t.Fatalf("plan file = %q, %v", got, err)
	}
	// The folder ignores itself; no tracked file was touched.
	ignore, err := os.ReadFile(filepath.Join(dir, ".console", "plans", ".gitignore"))
	if err != nil || strings.TrimSpace(string(ignore)) != "*" {
		t.Fatalf(".gitignore = %q, %v", ignore, err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".gitignore")); err == nil {
		t.Fatal("writePlan must not create or edit the repository's own .gitignore")
	}
	// ...and ordinary writes are still denied alongside it.
	res, _ = exec.Execute(context.Background(), planCall(t, "write_file", map[string]any{"path": filepath.Join(dir, "x.txt"), "content": "x"}))
	if !res.IsError {
		t.Fatal("write_file must stay denied next to writePlan")
	}
}

// writePlan writes without an approval prompt, so it must be unable to reach
// anything but its own folder, whatever name the model supplies.
func TestWritePlanCannotEscapeItsFolder(t *testing.T) {
	root := t.TempDir()
	cwd := filepath.Join(root, "repo")
	if err := os.MkdirAll(cwd, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"../../escape", "..", "/etc/passwd", "a/b", `..\..\x`, "....//x", "x\x00y"} {
		path, err := tools.SavePlan(cwd, name, "pwned")
		if err != nil {
			continue
		}
		// Anything accepted must have been flattened into a plain filename inside the folder.
		if strings.Contains(path, "..") || strings.Count(path, "/") != 2 || !strings.HasPrefix(path, tools.PlanDir+"/") {
			t.Errorf("name %q produced path %q", name, path)
		}
		if _, statErr := os.Stat(filepath.Join(root, "escape")); statErr == nil {
			t.Errorf("name %q wrote outside the repo", name)
		}
	}
	// Nothing landed beside the repo.
	siblings, _ := os.ReadDir(root)
	if len(siblings) != 1 {
		t.Fatalf("files escaped the working directory: %v", siblings)
	}
	if _, err := tools.PlanFileName("!!!"); err == nil {
		t.Fatal("a name with no letters or digits must be rejected")
	}
}

func TestWritePlanRefusesSymlinkTricks(t *testing.T) {
	root := t.TempDir()
	cwd := filepath.Join(root, "repo")
	outside := filepath.Join(root, "outside")
	for _, d := range []string{cwd, outside} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	// .console is a symlink to somewhere else.
	if err := os.Symlink(outside, filepath.Join(cwd, ".console")); err != nil {
		t.Skip("symlinks unavailable")
	}
	if _, err := tools.SavePlan(cwd, "p", "x"); err == nil {
		t.Fatal("a symlinked .console must be refused")
	}
	if entries, _ := os.ReadDir(outside); len(entries) != 0 {
		t.Fatalf("a write went through the symlink: %v", entries)
	}

	// The plan file itself is a symlink to a victim file.
	cwd2 := filepath.Join(root, "repo2")
	plans := filepath.Join(cwd2, ".console", "plans")
	if err := os.MkdirAll(plans, 0o755); err != nil {
		t.Fatal(err)
	}
	victim := filepath.Join(root, "victim.txt")
	if err := os.WriteFile(victim, []byte("keep"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(victim, filepath.Join(plans, "p.md")); err != nil {
		t.Skip("symlinks unavailable")
	}
	if _, err := tools.SavePlan(cwd2, "p", "overwrite"); err == nil {
		t.Fatal("a symlinked plan file must be refused")
	}
	if got, _ := os.ReadFile(victim); string(got) != "keep" {
		t.Fatalf("the victim file was overwritten: %q", got)
	}
}

func TestWritePlanLimitsAndInputs(t *testing.T) {
	dir := t.TempDir()
	// There is no size cap: a long plan is written whole.
	long := strings.Repeat("a line of the plan\n", 100_000)
	if _, err := tools.SavePlan(dir, "long", long); err != nil {
		t.Fatalf("a long plan must be accepted: %v", err)
	}
	if got, _ := os.ReadFile(filepath.Join(dir, ".console", "plans", "long.md")); string(got) != long {
		t.Fatal("a long plan was truncated")
	}
	if _, err := tools.SavePlan(dir, "empty", "   "); err == nil {
		t.Fatal("an empty plan must be refused")
	}
	if _, err := tools.SavePlan("", "p", "x"); err == nil {
		t.Fatal("no working directory must be refused, not written to the current one")
	}
	// Replacing a plan with the same name overwrites it.
	if _, err := tools.SavePlan(dir, "same", "one"); err != nil {
		t.Fatal(err)
	}
	if _, err := tools.SavePlan(dir, "same", "two"); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(filepath.Join(dir, ".console", "plans", "same.md")); string(got) != "two" {
		t.Fatalf("plan = %q, want the replacement", got)
	}
}

// Memory is read tier so planning can keep notes; it is not a project edit.
func TestMemoryIsAllowedInPlanMode(t *testing.T) {
	tool := tools.NewMemoryTool("p", nil)
	if tool.Tier() != tools.TierRead {
		t.Fatalf("memory tier = %s, want read", tool.Tier())
	}
	if permissions.Resolve(permissions.PlanMode, tool.Tier()) != permissions.Allow {
		t.Fatal("memory must be allowed in plan mode")
	}
}

// A subagent inherits the parent's mode, so delegating is not a way around the deny.
func TestPlanModeAppliesToSubagentsToo(t *testing.T) {
	registry := tools.NewRegistry(tools.WriteFile)
	exec := loop.NewExecutor(registry, permissions.PlanMode, nil)
	dir := t.TempDir()
	res, _ := exec.Execute(context.Background(), planCall(t, "write_file", map[string]any{"path": filepath.Join(dir, "s.txt"), "content": "x"}))
	if !res.IsError {
		t.Fatal("an executor in plan mode must deny writes, subagent or not")
	}
}
