package tests

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/tools"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/services/terminal"
)

func runBashJobs(t *testing.T, jobs *terminal.BashJobManager, in map[string]any) string {
	t.Helper()
	raw, _ := json.Marshal(in)
	res, err := tools.NewBashTool(jobs, "s1").Execute(context.Background(), raw)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	return fmt.Sprintf("%v", res)
}

func TestBashForegroundTimeoutPromotesToBackground(t *testing.T) {
	jobs := terminal.NewBashJobManager(nil)
	defer jobs.KillOwnedBy("s1")

	out := runBashJobs(t, jobs, map[string]any{"command": "sleep 5; echo done", "timeoutMs": 300})
	if !strings.Contains(out, "moved to the background") {
		t.Fatalf("expected promotion, got: %s", out)
	}
	if list := jobs.List("s1"); len(list) != 1 || list[0].Status != terminal.BashJobRunning {
		t.Fatalf("expected one running job, got %+v", list)
	}
}

func TestBashForegroundFastCommandLeavesNoJob(t *testing.T) {
	jobs := terminal.NewBashJobManager(nil)
	out := runBashJobs(t, jobs, map[string]any{"command": "echo hi"})
	if !strings.Contains(out, "hi") || !strings.Contains(out, "Exit code: 0") {
		t.Fatalf("unexpected output: %s", out)
	}
	time.Sleep(50 * time.Millisecond)
	if n := len(jobs.List("s1")); n != 0 {
		t.Fatalf("foreground job leaked: %d", n)
	}
}

func TestBashBackgroundHasNoDefaultLifetime(t *testing.T) {
	jobs := terminal.NewBashJobManager(nil)
	defer jobs.KillOwnedBy("s1")
	out := runBashJobs(t, jobs, map[string]any{"command": "sleep 5", "background": true})
	if !strings.Contains(out, "Time limit: none") {
		t.Fatalf("unexpected output: %s", out)
	}
}

func TestBashPlanModeTimeoutHint(t *testing.T) {
	raw, _ := json.Marshal(map[string]any{"command": "sleep 5", "timeoutMs": 200})
	_, err := tools.NewSandboxedBashTool(tools.SandboxReadOnly, nil, "s1").Execute(context.Background(), raw)
	if err == nil || !strings.Contains(err.Error(), "too slow for plan mode") {
		t.Fatalf("expected plan-mode hint, got: %v", err)
	}
}
