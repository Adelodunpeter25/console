// Plan-mode bash runs under a read-only OS sandbox: every command can read and
// use the network and the temp folder, and none can write anywhere else. The
// project folder is placed under the home directory, NOT the system temp
// folder, because temp is writable inside the sandbox by design and a project
// inside it would prove nothing.
package tests

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/tools"
)

func sandboxProject(t *testing.T) string {
	t.Helper()
	if !tools.SandboxAvailable() {
		t.Skip("no usable sandbox on this machine (needs sandbox-exec on macOS, bwrap on Linux)")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("no home directory")
	}
	dir, err := os.MkdirTemp(home, ".console-plan-sandbox-test-")
	if err != nil {
		t.Skipf("cannot create a project under home: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	if err := os.WriteFile(filepath.Join(dir, "existing.txt"), []byte("original"), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

// A sandbox that is present but silently not working would make every other test
// here skip and pass. On a machine that has the tool, the probe must succeed.
func TestSandboxIsAvailableWhereTheToolExists(t *testing.T) {
	var tool string
	switch runtime.GOOS {
	case "darwin":
		tool = "sandbox-exec"
	case "linux":
		tool = "bwrap"
	default:
		t.Skip("no sandbox implementation for " + runtime.GOOS)
	}
	if _, err := exec.LookPath(tool); err != nil {
		t.Skipf("%s is not installed here, so plan-mode bash correctly falls back to unsandboxed", tool)
	}
	if !tools.SandboxAvailable() {
		t.Fatalf("%s is installed but the sandbox probe failed: plan-mode bash would silently run unsandboxed", tool)
	}
}

func planBash(t *testing.T, cwd, command string) (string, bool) {
	t.Helper()
	tool := tools.NewPlanModeBashTool(nil, "")
	args, _ := json.Marshal(map[string]any{"command": command, "cwd": cwd})
	out, err := tool.Execute(context.Background(), args)
	if err != nil {
		return err.Error(), true
	}
	env, ok := out.(tools.Envelope)
	if !ok {
		return "", false
	}
	text := ""
	if parts, ok := env.Content.([]map[string]any); ok {
		for _, p := range parts {
			text += p["text"].(string)
		}
	}
	return text, env.IsError
}

func TestPlanBashCannotWriteTheProject(t *testing.T) {
	dir := sandboxProject(t)
	for name, cmd := range map[string]string{
		"redirect":         "echo x > new.txt",
		"append":           "echo x >> existing.txt",
		"overwrite":        "echo x > existing.txt",
		"touch":            "touch touched.txt",
		"mkdir":            "mkdir sub",
		"rm":               "rm existing.txt",
		"mv":               "mv existing.txt moved.txt",
		"cp":               "cp existing.txt copy.txt",
		"tee":              "echo x | tee tee.txt",
		"sed -i":           "sed -i.bak s/original/changed/ existing.txt",
		"python":           `python3 -c "open('py.txt','w').write('x')"`,
		"subshell":         "(echo x > sub1.txt)",
		"child of a child": `sh -c 'sh -c "echo x > deep.txt"'`,
		"git init":         "git init -q . && echo done",
		"dd":               "dd if=/dev/zero of=dd.bin bs=1 count=4 2>/dev/null",
		"chmod":            "chmod 000 existing.txt",
		"truncate":         ": > existing.txt",
	} {
		planBash(t, dir, cmd)
		_ = name
	}
	entries, _ := os.ReadDir(dir)
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	if len(names) != 1 || names[0] != "existing.txt" {
		t.Fatalf("the sandbox let a write through; project now holds %v", names)
	}
	got, _ := os.ReadFile(filepath.Join(dir, "existing.txt"))
	if string(got) != "original" {
		t.Fatalf("existing.txt was modified: %q", got)
	}
	info, _ := os.Stat(filepath.Join(dir, "existing.txt"))
	if info.Mode().Perm() != 0o644 {
		t.Fatalf("existing.txt permissions changed: %v", info.Mode())
	}
}

func TestPlanBashCannotCommit(t *testing.T) {
	dir := sandboxProject(t)
	run := func(c string) { planBash(t, dir, c) }
	// Set the repository up outside the sandbox, then try to change it inside.
	if out, err := execIn(dir, "git", "init", "-q"); err != nil {
		t.Skipf("git unavailable: %v %s", err, out)
	}
	run("git add -A && git -c user.email=a@b -c user.name=n commit -qm blocked")
	if out, _ := execIn(dir, "git", "log", "--oneline"); strings.TrimSpace(out) != "" && !strings.Contains(out, "does not have any commits") {
		t.Fatalf("a commit landed under the sandbox: %q", out)
	}
}

func TestPlanBashStillReadsAndPipesAndUsesTemp(t *testing.T) {
	dir := sandboxProject(t)
	text, isErr := planBash(t, dir, "cat existing.txt | wc -c && ls | head -1")
	if isErr || !strings.Contains(text, "8") || !strings.Contains(text, "existing.txt") {
		t.Fatalf("read and pipe must work: %v %q", isErr, text)
	}
	tmp := filepath.Join(os.TempDir(), "plan-sandbox-ok-"+filepath.Base(dir))
	defer os.Remove(tmp)
	text, isErr = planBash(t, dir, "echo temp > "+tmp+" && cat "+tmp)
	if isErr || !strings.Contains(text, "temp") {
		t.Fatalf("the system temp folder must stay writable: %v %q", isErr, text)
	}
	text, isErr = planBash(t, dir, "git --version && grep -c original existing.txt")
	if isErr {
		t.Fatalf("read-only git and grep must work: %q", text)
	}
}

func TestPlanBashExplainsABlockedWrite(t *testing.T) {
	dir := sandboxProject(t)
	text, isErr := planBash(t, dir, "echo x > blocked.txt")
	if !isErr {
		t.Fatalf("a blocked write must report failure: %q", text)
	}
	if !strings.Contains(text, "read-only sandbox") || !strings.Contains(text, "plan mode") {
		t.Fatalf("the model needs to be told why, not just 'Operation not permitted': %q", text)
	}
}

func TestPlanBashRefusesBackgroundJobs(t *testing.T) {
	dir := sandboxProject(t)
	text, isErr := planBash(t, dir, "sleep 1")
	_ = text
	_ = isErr
	tool := tools.NewPlanModeBashTool(nil, "")
	args, _ := json.Marshal(map[string]any{"command": "sleep 1", "cwd": dir, "background": true})
	_, err := tool.Execute(context.Background(), args)
	if err == nil || !strings.Contains(err.Error(), "plan mode") {
		t.Fatalf("background jobs must be refused in plan mode: %v", err)
	}
}

// Outside plan mode nothing changes: bash writes as it always did.
func TestOrdinaryBashIsNotSandboxed(t *testing.T) {
	dir := t.TempDir()
	tool := tools.NewBashTool(nil, "")
	args, _ := json.Marshal(map[string]any{"command": "echo x > out.txt", "cwd": dir})
	if _, err := tool.Execute(context.Background(), args); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "out.txt")); err != nil {
		t.Fatalf("ordinary bash must still write: %v", err)
	}
}

func TestPlanModeBashIsMarkedPlanSafe(t *testing.T) {
	safe, ok := tools.NewPlanModeBashTool(nil, "").(tools.PlanSafe)
	if !ok || !safe.PlanSafe() {
		t.Fatal("plan-mode bash must be plan-safe so the executor lets it run")
	}
	if _, ok := tools.NewBashTool(nil, "").(tools.PlanSafe); ok {
		t.Fatal("ordinary bash must NOT be plan-safe: plan mode must keep denying it")
	}
}

func execIn(dir, name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	return string(out), err
}
