package run

import (
	"strings"
	"testing"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/services"
)

func TestBackgroundContextClaimsShellAndSubagentResultsOnce(t *testing.T) {
	service := NewService(nil)
	manager := services.NewBackgroundWorkManager()
	service.SetBackgroundWorkManager(manager)

	shell, err := manager.Register(services.BackgroundWorkOptions{
		SessionID: "session-1", Kind: services.BackgroundWorkShell,
		Metadata: map[string]string{"job_id": "job-1", "command": "go test ./...", "exit_code": "0"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Complete(shell.ID, "session-1", services.BackgroundWorkCompleted, "Command finished with status exited", "", nil); err != nil {
		t.Fatal(err)
	}
	subagent, err := manager.Register(services.BackgroundWorkOptions{
		SessionID: "session-1", Kind: services.BackgroundWorkSubagent,
		Metadata: map[string]string{"name": "scout"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Complete(subagent.ID, "session-1", services.BackgroundWorkFailed, "", "provider failed", nil); err != nil {
		t.Fatal(err)
	}

	context := service.backgroundContext("session-1")
	for _, want := range []string{"job-1", "go test ./...", "Name: scout", "Error: provider failed"} {
		if !strings.Contains(context, want) {
			t.Fatalf("background context missing %q: %s", want, context)
		}
	}
	if again := service.backgroundContext("session-1"); again != "" {
		t.Fatalf("background context was delivered twice: %s", again)
	}
}
