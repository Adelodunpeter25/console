// Project scripts config + managed runs coverage.
package tests

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/db"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/services"
)

func newScriptService(t *testing.T) (*services.ProjectScriptsService, string, string) {
	t.Helper()
	manager, err := db.Open(db.OpenOptions{Path: ":memory:"})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(manager.Close)
	projects := services.NewProjectService(manager)
	root := t.TempDir()
	proj, err := projects.Create(services.CreateProjectOptions{Name: "P", Dir: root})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	return services.NewProjectScriptsService(projects), proj.ID, root
}

func writeConsoleToml(t *testing.T, root, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(root, "console.toml"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestParseProjectScripts(t *testing.T) {
	tomlText := `
[scripts]

[scripts.dev]
label = "Dev server"
command = "bun dev"
persistent = true

[scripts.test]
label = "Tests"
command = "bun test"
`
	scripts, err := services.ParseProjectScriptsForTest(tomlText)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(scripts) != 2 {
		t.Fatalf("scripts: %+v", scripts)
	}
	byID := map[string]bool{}
	for _, s := range scripts {
		byID[s.ID] = true
	}
	if !byID["dev"] || !byID["test"] {
		t.Fatalf("missing ids: %+v", scripts)
	}

	// Bad identifier rejected.
	if _, err := services.ParseProjectScriptsForTest(`
[scripts."bad id"]
label = "x"
command = "y"
`); err == nil {
		t.Fatal("bad id should fail")
	}
}

func TestListMissingConfig(t *testing.T) {
	scripts, projectID, _ := newScriptService(t)
	result, err := scripts.List(projectID)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if result.Source != "missing" || len(result.Scripts) != 0 {
		t.Fatalf("unexpected: %+v", result)
	}
}

func TestRunLifecycle(t *testing.T) {
	scripts, projectID, root := newScriptService(t)
	writeConsoleToml(t, root, `
[scripts.hello]
label = "Hello"
command = "echo SCRIPT_RAN && echo oops >&2 && sleep 0.2 && exit 3"
`)

	run, err := scripts.Run(projectID, "hello")
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if run.Status != "running" {
		t.Fatalf("initial status: %s", run.Status)
	}

	// Already-running guard.
	if _, err := scripts.Run(projectID, "hello"); err == nil {
		t.Fatal("second run should fail")
	}

	// Wait for completion.
	deadline := time.Now().Add(5 * time.Second)
	for scripts.IsRunning(projectID, run.RunID) && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	final := scripts.GetRun(projectID, run.RunID)
	if final == nil || final.Status != "failed" {
		t.Fatalf("final run: %+v", final)
	}
	if final.ExitCode == nil || *final.ExitCode != 3 {
		t.Fatalf("exit code: %v", final.ExitCode)
	}
	if !scriptOutputContains(final.Stdout, "SCRIPT_RAN") || !scriptOutputContains(final.Stderr, "oops") {
		t.Fatalf("output capture: stdout=%q stderr=%q", final.Stdout, final.Stderr)
	}
	if final.EndedAt == nil {
		t.Fatal("endedAt missing")
	}

	// Runs list includes the finished run.
	if runs := scripts.ListRuns(projectID); len(runs) != 1 {
		t.Fatalf("listRuns: %d", len(runs))
	}
}

func TestStopRunning(t *testing.T) {
	scripts, projectID, root := newScriptService(t)
	writeConsoleToml(t, root, `
[scripts.sleeper]
label = "Sleeper"
command = "sleep 30"
`)

	run, err := scripts.Run(projectID, "sleeper")
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	time.Sleep(200 * time.Millisecond) // let the process start

	if !scripts.Stop(projectID, run.RunID) {
		t.Fatal("stop returned false")
	}
	if scripts.Stop(projectID, run.RunID) {
		t.Fatal("double stop should return false")
	}
	// The whole process group must die — sleep is a child of sh.
	deadline := time.Now().Add(3 * time.Second)
	for scripts.IsRunning(projectID, run.RunID) && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if scripts.IsRunning(projectID, run.RunID) {
		t.Fatal("run still running after stop")
	}
	if final := scripts.GetRun(projectID, run.RunID); final.Status != "stopped" {
		t.Fatalf("status: %s", final.Status)
	}
}

func TestScriptEnvStripsDaemonPort(t *testing.T) {
	t.Setenv("PORT", "9999")
	t.Setenv("CONSOLE_DAEMON", "1")
	cmd := exec.Command("sh", "-c", "echo ${PORT:-unset}")
	cmd.Env = services.BuildScriptEnv()
	out, err := cmd.Output()
	if err != nil {
		t.Fatal(err)
	}
	if string(out) != "unset\n" {
		t.Fatalf("PORT leaked into script env: %q", string(out))
	}
}

func scriptOutputContains(haystack, needle string) bool {
	return strings.Contains(haystack, needle)
}
