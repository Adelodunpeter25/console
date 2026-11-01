// Plan mode is read-only by enforcement, not by request: write and exec tools
// are denied, read tools run, and writePlan is the one write that gets through,
// confined to docs/plan/, and never overwriting what is already there.
package tests

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
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
	got, err := os.ReadFile(filepath.Join(dir, "docs", "plan", "auth-refactor.md"))
	if err != nil || string(got) != "# Plan\n1. do it" {
		t.Fatalf("plan file = %q, %v", got, err)
	}
	// docs/plan/ is a normal tracked folder: nothing in it hides plans from git.
	if _, err := os.Stat(filepath.Join(dir, "docs", "plan", ".gitignore")); err == nil {
		t.Fatal("writePlan must not create a .gitignore in the plan folder")
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
	// docs is a symlink to somewhere else.
	if err := os.Symlink(outside, filepath.Join(cwd, "docs")); err != nil {
		t.Skip("symlinks unavailable")
	}
	if _, err := tools.SavePlan(cwd, "p", "x"); err == nil {
		t.Fatal("a symlinked docs folder must be refused")
	}
	if entries, _ := os.ReadDir(outside); len(entries) != 0 {
		t.Fatalf("a write went through the symlink: %v", entries)
	}

	// The plan file itself is a symlink to a victim file.
	cwd2 := filepath.Join(root, "repo2")
	plans := filepath.Join(cwd2, "docs", "plan")
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
	// The planted link is skipped, not followed: the plan lands under a new
	// name and the victim file is untouched.
	path, err := tools.SavePlan(cwd2, "p", "overwrite")
	if err != nil {
		t.Fatalf("a planted symlink should be stepped around, not fail the save: %v", err)
	}
	if path == tools.PlanDir+"/p.md" {
		t.Fatal("the plan was written through the symlink's name")
	}
	if got, _ := os.ReadFile(victim); string(got) != "keep" {
		t.Fatalf("the victim file was overwritten: %q", got)
	}
	if got, _ := os.ReadFile(filepath.Join(cwd2, filepath.FromSlash(path))); string(got) != "overwrite" {
		t.Fatalf("the plan did not land at %s: %q", path, got)
	}
}

func TestWritePlanLimitsAndInputs(t *testing.T) {
	dir := t.TempDir()
	// There is no size cap: a long plan is written whole.
	long := strings.Repeat("a line of the plan\n", 100_000)
	if _, err := tools.SavePlan(dir, "long", long); err != nil {
		t.Fatalf("a long plan must be accepted: %v", err)
	}
	if got, _ := os.ReadFile(filepath.Join(dir, "docs", "plan", "long.md")); string(got) != long {
		t.Fatal("a long plan was truncated")
	}
	if _, err := tools.SavePlan(dir, "empty", "   "); err == nil {
		t.Fatal("an empty plan must be refused")
	}
	if _, err := tools.SavePlan("", "p", "x"); err == nil {
		t.Fatal("no working directory must be refused, not written to the current one")
	}
	// A name that is taken is never replaced: the next save gets a suffix.
	first, err := tools.SavePlan(dir, "same", "one")
	if err != nil || first != "docs/plan/same.md" {
		t.Fatalf("first = %q, %v", first, err)
	}
	second, err := tools.SavePlan(dir, "same", "two")
	if err != nil || second != "docs/plan/same-01.md" {
		t.Fatalf("second = %q, %v", second, err)
	}
	if got, _ := os.ReadFile(filepath.Join(dir, "docs", "plan", "same.md")); string(got) != "one" {
		t.Fatalf("the first plan was overwritten: %q", got)
	}
	if got, _ := os.ReadFile(filepath.Join(dir, "docs", "plan", "same-01.md")); string(got) != "two" {
		t.Fatalf("second plan = %q", got)
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

// Plan mode allows bash only in its sandboxed form. The ordinary bash tool, and
// background jobs, stay denied.
func TestPlanModeAllowsOnlySandboxedBash(t *testing.T) {
	dir := t.TempDir()
	asked := 0

	plain := loop.NewExecutor(tools.NewRegistry(tools.NewBashTool(nil, "")), permissions.PlanMode, denyApprover{&asked})
	res, _ := plain.Execute(context.Background(), planCall(t, "bash", map[string]any{"command": "echo x > " + filepath.Join(dir, "p.txt")}))
	if !res.IsError || !strings.Contains(res.Content.(string), "plan mode is read-only") {
		t.Fatalf("ordinary bash must stay denied in plan mode: %#v", res)
	}
	if _, err := os.Stat(filepath.Join(dir, "p.txt")); err == nil {
		t.Fatal("ordinary bash wrote a file in plan mode")
	}

	sandboxed := loop.NewExecutor(tools.NewRegistry(tools.NewPlanModeBashTool(nil, "")), permissions.PlanMode, denyApprover{&asked})
	res, err := sandboxed.Execute(context.Background(), planCall(t, "bash", map[string]any{"command": "echo hello", "cwd": dir}))
	if err != nil || res.IsError {
		t.Fatalf("sandboxed bash must run in plan mode: %v %#v", err, res)
	}
	if asked != 0 {
		t.Fatalf("plan mode must not prompt: asked %d times", asked)
	}

	// Outside plan mode the sandboxed flag grants nothing special.
	if permissions.Resolve(permissions.AlwaysAsk, tools.TierExec) != permissions.Prompt {
		t.Fatal("always-ask still prompts for exec")
	}
}

// docs/plan/ holds documents people edit by hand. A plan whose name matches one
// must never replace it, however many times the name repeats.
func TestWritePlanNeverOverwritesAnExistingDocument(t *testing.T) {
	dir := t.TempDir()
	plans := filepath.Join(dir, "docs", "plan")
	if err := os.MkdirAll(plans, 0o755); err != nil {
		t.Fatal(err)
	}
	const handWritten = "# Orchestration\nedited by a person\n"
	if err := os.WriteFile(filepath.Join(plans, "orchestration.md"), []byte(handWritten), 0o644); err != nil {
		t.Fatal(err)
	}
	var used []string
	for i := 0; i < 5; i++ {
		path, err := tools.SavePlan(dir, "orchestration", "agent plan "+string(rune('A'+i)))
		if err != nil {
			t.Fatal(err)
		}
		used = append(used, path)
	}
	if got, _ := os.ReadFile(filepath.Join(plans, "orchestration.md")); string(got) != handWritten {
		t.Fatalf("a hand-written document was overwritten: %q", got)
	}
	want := []string{"orchestration-01.md", "orchestration-02.md", "orchestration-03.md", "orchestration-04.md", "orchestration-05.md"}
	for i, w := range want {
		if used[i] != "docs/plan/"+w {
			t.Errorf("save %d used %q, want docs/plan/%s", i, used[i], w)
		}
	}
	// Every save kept its own content.
	for i, w := range want {
		got, _ := os.ReadFile(filepath.Join(plans, w))
		if string(got) != "agent plan "+string(rune('A'+i)) {
			t.Errorf("%s = %q", w, got)
		}
	}
}

// The folder is created when the project has no docs/ at all.
func TestWritePlanCreatesDocsPlanWhenMissing(t *testing.T) {
	dir := t.TempDir()
	if _, err := os.Stat(filepath.Join(dir, "docs")); err == nil {
		t.Fatal("test setup: docs must not exist yet")
	}
	path, err := tools.SavePlan(dir, "fresh", "# hi")
	if err != nil || path != "docs/plan/fresh.md" {
		t.Fatalf("path = %q, %v", path, err)
	}
	if info, err := os.Stat(filepath.Join(dir, "docs", "plan")); err != nil || !info.IsDir() {
		t.Fatalf("docs/plan was not created: %v", err)
	}
}

// Two saves of the same name racing must each land in their own file: the
// exclusive create, not a check beforehand, decides who gets which name.
func TestWritePlanConcurrentSavesDoNotCollide(t *testing.T) {
	dir := t.TempDir()
	const n = 12
	paths := make(chan string, n)
	errs := make(chan error, n)
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			p, err := tools.SavePlan(dir, "race", fmt.Sprintf("content-%d", i))
			if err != nil {
				errs <- err
				return
			}
			paths <- p
		}(i)
	}
	wg.Wait()
	close(paths)
	close(errs)
	for err := range errs {
		t.Fatalf("a concurrent save failed: %v", err)
	}
	seen := map[string]bool{}
	for p := range paths {
		if seen[p] {
			t.Fatalf("two saves were given the same file %s", p)
		}
		seen[p] = true
	}
	if len(seen) != n {
		t.Fatalf("%d distinct files for %d saves", len(seen), n)
	}
	// And every payload survived somewhere.
	contents := map[string]bool{}
	for p := range seen {
		b, _ := os.ReadFile(filepath.Join(dir, filepath.FromSlash(p)))
		contents[string(b)] = true
	}
	if len(contents) != n {
		t.Fatalf("a plan's content was lost: %d distinct contents for %d saves", len(contents), n)
	}
}

// The tool tells the model when its name was changed, so it can refer to the file.
func TestWritePlanToolReportsASuffix(t *testing.T) {
	dir := t.TempDir()
	tool := tools.NewWritePlanTool(dir)
	call := func() string {
		out, err := tool.Execute(context.Background(), []byte(`{"name":"report","content":"x"}`))
		if err != nil {
			t.Fatal(err)
		}
		parts, _ := out.([]map[string]any)
		return parts[0]["text"].(string)
	}
	if first := call(); !strings.Contains(first, "docs/plan/report.md") || strings.Contains(first, "suffix") {
		t.Fatalf("first save = %q", first)
	}
	second := call()
	if !strings.Contains(second, "docs/plan/report-01.md") || !strings.Contains(second, "nothing was overwritten") {
		t.Fatalf("a renamed save must say so: %q", second)
	}
}
