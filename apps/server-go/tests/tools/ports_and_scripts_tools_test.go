package tests

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/tools"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/types"
	"github.com/Adelodunpeter25/console/apps/server-go/tests/helpers"
)

type mockPortsProvider struct {
	ports       []types.ClientPort
	forwardFunc func(port int, projectID string) (types.ClientPort, error)
}

func (m *mockPortsProvider) List(host, projectID string) []types.ClientPort {
	var out []types.ClientPort
	for _, p := range m.ports {
		if p.ProjectID == nil || *p.ProjectID == projectID {
			out = append(out, p)
		}
	}
	return out
}

func (m *mockPortsProvider) Forward(port int, projectID string) (types.ClientPort, error) {
	if m.forwardFunc != nil {
		return m.forwardFunc(port, projectID)
	}
	for _, p := range m.ports {
		if p.Port == port {
			return p, nil
		}
	}
	return types.ClientPort{}, fmt.Errorf("Port %d is not listening.", port)
}

func TestPortsTool_Unavailable(t *testing.T) {
	tool := tools.NewPortsTool("proj_1", nil)
	args := helpers.MustJSONRaw(t, map[string]any{"action": "list"})
	_, err := tool.Execute(context.Background(), args)
	if err == nil || !strings.Contains(err.Error(), "unavailable") {
		t.Fatalf("expected unavailable error, got %v", err)
	}
}

func TestPortsTool_List(t *testing.T) {
	pid := "proj_1"
	provider := &mockPortsProvider{
		ports: []types.ClientPort{
			{Port: 3000, URL: "http://localhost:45000", ProjectID: &pid},
			{Port: 5173, URL: "http://localhost:45001", ProjectID: &pid},
		},
	}

	tool := tools.NewPortsTool("proj_1", provider)

	// List
	args := helpers.MustJSONRaw(t, map[string]any{"action": "list"})
	out, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	text := resultText(t, out)
	if !strings.Contains(text, "3000") || !strings.Contains(text, "5173") {
		t.Fatalf("unexpected list output: %s", text)
	}

	// Empty list
	emptyTool := tools.NewPortsTool("proj_2", provider)
	out, err = emptyTool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("list empty: %v", err)
	}
	text = resultText(t, out)
	if !strings.Contains(text, "No active port forwards") {
		t.Fatalf("expected empty message, got: %s", text)
	}
}

func TestPortsTool_Forward(t *testing.T) {
	pid := "proj_1"
	provider := &mockPortsProvider{
		forwardFunc: func(port int, projectID string) (types.ClientPort, error) {
			if port == 8080 {
				return types.ClientPort{Port: 8080, URL: "http://localhost:45002", ProjectID: &pid}, nil
			}
			return types.ClientPort{}, fmt.Errorf("Port %d is not listening on 127.0.0.1.", port)
		},
	}

	tool := tools.NewPortsTool("proj_1", provider)

	// Valid forward
	args := helpers.MustJSONRaw(t, map[string]any{"action": "forward", "port": 8080})
	out, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("forward: %v", err)
	}
	text := resultText(t, out)
	if !strings.Contains(text, "Port 8080 forwarded successfully") || !strings.Contains(text, "45002") {
		t.Fatalf("unexpected forward output: %s", text)
	}

	// Invalid port number
	args = helpers.MustJSONRaw(t, map[string]any{"action": "forward", "port": 0})
	_, err = tool.Execute(context.Background(), args)
	if err == nil || !strings.Contains(err.Error(), "greater than 0") {
		t.Fatalf("expected port > 0 error, got %v", err)
	}

	// Port not listening
	args = helpers.MustJSONRaw(t, map[string]any{"action": "forward", "port": 9999})
	_, err = tool.Execute(context.Background(), args)
	if err == nil || !strings.Contains(err.Error(), "not listening") {
		t.Fatalf("expected not listening error, got %v", err)
	}
}

type mockProjectScriptsProvider struct {
	scripts     types.ProjectScriptsResult
	runs        map[string]*types.ScriptRun
	startErr    error
	listErr     error
	nextRunID   int
}

func (m *mockProjectScriptsProvider) List(projectID string) (types.ProjectScriptsResult, error) {
	if m.listErr != nil {
		return types.ProjectScriptsResult{}, m.listErr
	}
	return m.scripts, nil
}

func (m *mockProjectScriptsProvider) Run(projectID, scriptID string) (types.ScriptRun, error) {
	if m.startErr != nil {
		return types.ScriptRun{}, m.startErr
	}
	m.nextRunID++
	runID := fmt.Sprintf("run_%d", m.nextRunID)
	run := types.ScriptRun{
		RunID:     runID,
		ProjectID: projectID,
		ScriptID:  scriptID,
		Status:    "running",
	}
	if m.runs == nil {
		m.runs = make(map[string]*types.ScriptRun)
	}
	m.runs[runID] = &run
	return run, nil
}

func (m *mockProjectScriptsProvider) Stop(projectID, runID string) bool {
	if m.runs == nil {
		return false
	}
	run, ok := m.runs[runID]
	if !ok || run.ProjectID != projectID || run.Status != "running" {
		return false
	}
	run.Status = "stopped"
	return true
}

func (m *mockProjectScriptsProvider) ListRuns(projectID string) []types.ScriptRun {
	var out []types.ScriptRun
	for _, r := range m.runs {
		if r.ProjectID == projectID {
			out = append(out, *r)
		}
	}
	return out
}

func (m *mockProjectScriptsProvider) GetRun(projectID, runID string) *types.ScriptRun {
	if m.runs == nil {
		return nil
	}
	run, ok := m.runs[runID]
	if !ok || run.ProjectID != projectID {
		return nil
	}
	return run
}

func TestProjectScriptsTool_Lifecycle(t *testing.T) {
	provider := &mockProjectScriptsProvider{
		scripts: types.ProjectScriptsResult{
			ProjectID: "proj_1",
			Source:    "console.toml",
			Scripts: []types.ProjectScript{
				{ID: "dev", Label: "Dev Server", Command: "npm run dev", Persistent: true},
				{ID: "build", Label: "Build", Command: "npm run build", Persistent: false},
			},
		},
	}

	tool := tools.NewProjectScriptsTool("proj_1", provider)

	// List
	args := helpers.MustJSONRaw(t, map[string]any{"action": "list"})
	out, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	text := resultText(t, out)
	if !strings.Contains(text, "Dev Server") || !strings.Contains(text, "npm run dev") {
		t.Fatalf("unexpected list output: %s", text)
	}

	// Start dev
	args = helpers.MustJSONRaw(t, map[string]any{"action": "start", "script_id": "dev"})
	out, err = tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	text = resultText(t, out)
	if !strings.Contains(text, "started successfully") || !strings.Contains(text, "run_1") {
		t.Fatalf("unexpected start output: %s", text)
	}

	// Status by script_id
	args = helpers.MustJSONRaw(t, map[string]any{"action": "status", "script_id": "dev"})
	out, err = tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("status by script: %v", err)
	}
	text = resultText(t, out)
	if !strings.Contains(text, "run_1") || !strings.Contains(text, "running") {
		t.Fatalf("unexpected status output: %s", text)
	}

	// Stop by script_id
	args = helpers.MustJSONRaw(t, map[string]any{"action": "stop", "script_id": "dev"})
	out, err = tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("stop: %v", err)
	}
	text = resultText(t, out)
	if !strings.Contains(text, "stopped successfully") {
		t.Fatalf("unexpected stop output: %s", text)
	}

	// Status by run_id
	args = helpers.MustJSONRaw(t, map[string]any{"action": "status", "run_id": "run_1"})
	out, err = tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("status by run_id: %v", err)
	}
	text = resultText(t, out)
	if !strings.Contains(text, "stopped") {
		t.Fatalf("unexpected run status: %s", text)
	}
}


