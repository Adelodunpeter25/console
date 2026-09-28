package tests

import (
	"context"
	"strings"
	"testing"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/tools"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/types"
	"github.com/Adelodunpeter25/console/apps/server-go/tests/helpers"
)

func TestProjectScriptsTool_FilteredStatusEmptyState(t *testing.T) {
	provider := &mockProjectScriptsProvider{
		scripts: types.ProjectScriptsResult{
			ProjectID: "proj_1",
			Source:    "console.toml",
			Scripts: []types.ProjectScript{
				{ID: "dev", Label: "Dev Server", Command: "npm run dev", Persistent: true},
			},
		},
		runs: map[string]*types.ScriptRun{
			"run_1": {
				RunID:     "run_1",
				ProjectID: "proj_1",
				ScriptID:  "dev",
				Status:    "running",
			},
		},
	}

	tool := tools.NewProjectScriptsTool("proj_1", provider)

	// Status for nonexistent script_id should return specific message
	args := helpers.MustJSONRaw(t, map[string]any{"action": "status", "script_id": "nonexistent"})
	out, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("status nonexistent: %v", err)
	}
	text := resultText(t, out)
	if !strings.Contains(text, "No runs found for script 'nonexistent'.") {
		t.Fatalf("expected specific empty-state message, got: %s", text)
	}

	// Status when no runs exist at all and no filter given
	emptyProvider := &mockProjectScriptsProvider{
		scripts: types.ProjectScriptsResult{
			ProjectID: "proj_2",
			Source:    "console.toml",
		},
	}
	emptyTool := tools.NewProjectScriptsTool("proj_2", emptyProvider)
	allArgs := helpers.MustJSONRaw(t, map[string]any{"action": "status"})
	out, err = emptyTool.Execute(context.Background(), allArgs)
	if err != nil {
		t.Fatalf("status all empty: %v", err)
	}
	text = resultText(t, out)
	if !strings.Contains(text, "No script runs found.") {
		t.Fatalf("expected generic empty message, got: %s", text)
	}
}

func TestProjectScriptsTool_StopLifecycleAndSignalMetadata(t *testing.T) {
	sig := "SIGTERM"
	ended := "2026-09-28T12:00:00.000Z"
	provider := &mockProjectScriptsProvider{
		runs: map[string]*types.ScriptRun{
			"run_1": {
				RunID:     "run_1",
				ProjectID: "proj_1",
				ScriptID:  "dev",
				Status:    "running",
				Stdout:    "Listening on port 3000\n",
			},
		},
	}

	tool := tools.NewProjectScriptsTool("proj_1", provider)

	// Stop run
	stopArgs := helpers.MustJSONRaw(t, map[string]any{"action": "stop", "run_id": "run_1"})
	out, err := tool.Execute(context.Background(), stopArgs)
	if err != nil {
		t.Fatalf("stop run: %v", err)
	}
	text := resultText(t, out)
	if !strings.Contains(text, "Run 'run_1' stopped successfully.") {
		t.Fatalf("unexpected stop text: %s", text)
	}

	// Simulate provider metadata update with signal
	provider.runs["run_1"].Signal = &sig
	provider.runs["run_1"].EndedAt = &ended

	// Check status by run_id
	statusArgs := helpers.MustJSONRaw(t, map[string]any{"action": "status", "run_id": "run_1"})
	out, err = tool.Execute(context.Background(), statusArgs)
	if err != nil {
		t.Fatalf("status run_1: %v", err)
	}
	text = resultText(t, out)
	if !strings.Contains(text, "SIGTERM") || !strings.Contains(text, "stopped") {
		t.Fatalf("expected SIGTERM signal in status output: %s", text)
	}
	if !strings.Contains(text, "--- stdout ---") || !strings.Contains(text, "Listening on port 3000") {
		t.Fatalf("expected surfaced stdout in status output: %s", text)
	}
}

func TestProjectScriptsTool_StartWithOutputAndLogs(t *testing.T) {
	provider := &mockProjectScriptsProvider{
		scripts: types.ProjectScriptsResult{
			ProjectID: "proj_1",
			Source:    "console.toml",
			Scripts: []types.ProjectScript{
				{ID: "build", Label: "Build", Command: "npm run build"},
			},
		},
	}

	tool := tools.NewProjectScriptsTool("proj_1", provider)

	// Start script with wait_ms
	startArgs := helpers.MustJSONRaw(t, map[string]any{
		"action":    "start",
		"script_id": "build",
		"wait_ms":   50,
	})
	out, err := tool.Execute(context.Background(), startArgs)
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	text := resultText(t, out)
	if !strings.Contains(text, "Script 'build' started successfully") {
		t.Fatalf("unexpected start output: %s", text)
	}
}
