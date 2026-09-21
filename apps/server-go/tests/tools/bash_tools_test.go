// Coverage for bash / bashJob: sync execution (output, exit codes, timeout),
// and background job lifecycle (start, status, output pagination, wait,
// kill, list), all against real subprocesses.
package tests

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/tools"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/services"
)

func TestBashSyncSuccess(t *testing.T) {
	bash := tools.NewBashTool(nil, "")
	args, _ := json.Marshal(map[string]any{"command": "echo hello && echo world >&2"})
	out, err := bash.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	s := resultText(t, out)
	if resultIsError(out) || !strings.Contains(s, "Exit code: 0") || !strings.Contains(s, "hello") || !strings.Contains(s, "world") {
		t.Fatalf("output: %v", out)
	}
}

func TestBashSyncNonZeroExit(t *testing.T) {
	bash := tools.NewBashTool(nil, "")
	args, _ := json.Marshal(map[string]any{"command": "exit 3"})
	out, err := bash.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !resultIsError(out) {
		t.Fatalf("expected isError for non-zero exit: %v", out)
	}
}

func TestBashSyncTimeout(t *testing.T) {
	bash := tools.NewBashTool(nil, "")
	args, _ := json.Marshal(map[string]any{"command": "sleep 5", "timeoutMs": 1000})
	if _, err := bash.Execute(context.Background(), args); err == nil {
		t.Fatal("expected timeout error")
	}
}

func TestBashMissingCommand(t *testing.T) {
	bash := tools.NewBashTool(nil, "")
	args, _ := json.Marshal(map[string]any{"command": ""})
	if _, err := bash.Execute(context.Background(), args); err == nil {
		t.Fatal("expected error for missing command")
	}
}

func TestBashBackgroundDisabledWithoutManager(t *testing.T) {
	bash := tools.NewBashTool(nil, "")
	args, _ := json.Marshal(map[string]any{"command": "sleep 1", "background": true})
	if _, err := bash.Execute(context.Background(), args); err == nil {
		t.Fatal("expected error when background jobs unavailable")
	}
}

func TestBashJobLifecycle(t *testing.T) {
	jobs := services.NewBashJobManager()
	bash := tools.NewBashTool(jobs, "sess1")
	bashJob := tools.NewBashJobTool(jobs, "sess1")

	startArgs, _ := json.Marshal(map[string]any{
		"command": "echo hi && sleep 0.1 && echo bye", "background": true,
	})
	startOut, err := bash.Execute(context.Background(), startArgs)
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	startStr := resultText(t, startOut)
	if !strings.Contains(startStr, "Started: job_") {
		t.Fatalf("start output: %v", startOut)
	}
	jobID := strings.TrimPrefix(strings.SplitN(startStr, "\n", 2)[0], "Started: ")

	waitArgs, _ := json.Marshal(map[string]any{"action": "wait", "jobId": jobID, "waitMs": 3000})
	waitOut, err := bashJob.Execute(context.Background(), waitArgs)
	if err != nil {
		t.Fatalf("wait: %v", err)
	}
	waitStr := resultText(t, waitOut)
	if !strings.Contains(waitStr, "Status: exited") || !strings.Contains(waitStr, "hi") || !strings.Contains(waitStr, "bye") {
		t.Fatalf("wait output: %v", waitOut)
	}

	listArgs, _ := json.Marshal(map[string]any{"action": "list"})
	listOut, err := bashJob.Execute(context.Background(), listArgs)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if s := resultText(t, listOut); !strings.Contains(s, jobID) {
		t.Fatalf("list output: %v", listOut)
	}

	// Different owner session cannot see the job.
	otherJob := tools.NewBashJobTool(jobs, "sess2")
	statusArgs, _ := json.Marshal(map[string]any{"action": "status", "jobId": jobID})
	if _, err := otherJob.Execute(context.Background(), statusArgs); err == nil {
		t.Fatal("expected error: job not owned by sess2")
	}
}

func TestBashJobKill(t *testing.T) {
	jobs := services.NewBashJobManager()
	bash := tools.NewBashTool(jobs, "")
	bashJob := tools.NewBashJobTool(jobs, "")

	startArgs, _ := json.Marshal(map[string]any{"command": "sleep 30", "background": true})
	startOut, err := bash.Execute(context.Background(), startArgs)
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	startStr := resultText(t, startOut)
	jobID := strings.TrimPrefix(strings.SplitN(startStr, "\n", 2)[0], "Started: ")

	killArgs, _ := json.Marshal(map[string]any{"action": "kill", "jobId": jobID})
	killOut, err := bashJob.Execute(context.Background(), killArgs)
	if err != nil {
		t.Fatalf("kill: %v", err)
	}
	if s := resultText(t, killOut); !strings.Contains(s, "Status: killed") {
		t.Fatalf("kill output: %v", killOut)
	}
}

func TestBashJobMissingJobID(t *testing.T) {
	jobs := services.NewBashJobManager()
	bashJob := tools.NewBashJobTool(jobs, "")
	for _, action := range []string{"status", "output", "wait", "kill"} {
		args, _ := json.Marshal(map[string]any{"action": action})
		if _, err := bashJob.Execute(context.Background(), args); err == nil {
			t.Fatalf("expected error for missing jobId on action=%s", action)
		}
	}
}

func TestBashJobUnknownAction(t *testing.T) {
	jobs := services.NewBashJobManager()
	bashJob := tools.NewBashJobTool(jobs, "")
	args, _ := json.Marshal(map[string]any{"action": "bogus"})
	if _, err := bashJob.Execute(context.Background(), args); err == nil {
		t.Fatal("expected error for unknown action")
	}
}
