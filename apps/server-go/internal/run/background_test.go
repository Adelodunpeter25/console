package run

import (
	"strings"
	"testing"
	"time"

	backgroundsvc "github.com/Adelodunpeter25/console/apps/server-go/internal/services/background"
	terminalsvc "github.com/Adelodunpeter25/console/apps/server-go/internal/services/terminal"
)

func TestBackgroundContextClaimsShellAndSubagentResultsOnce(t *testing.T) {
	service := NewService(nil)
	manager := backgroundsvc.NewManager()
	service.SetBackgroundWorkManager(manager)

	shell, err := manager.Register(backgroundsvc.Options{
		SessionID: "session-1", Kind: backgroundsvc.KindShell,
		Metadata: map[string]string{"job_id": "job-1", "command": "go test ./...", "exit_code": "0"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Complete(shell.ID, "session-1", backgroundsvc.StatusCompleted, "Command finished with status exited", "", nil); err != nil {
		t.Fatal(err)
	}
	subagent, err := manager.Register(backgroundsvc.Options{
		SessionID: "session-1", Kind: backgroundsvc.KindSubagent,
		Metadata: map[string]string{"name": "scout"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Complete(subagent.ID, "session-1", backgroundsvc.StatusFailed, "", "provider failed", nil); err != nil {
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

func TestBackgroundContextIncludesCompletedShellOutput(t *testing.T) {
	service := NewService(nil)
	manager := backgroundsvc.NewManager()
	jobs := terminalsvc.NewBashJobManager(nil)
	service.SetBackgroundWorkManager(manager)
	service.SetBashJobs(jobs)

	snapshot, err := jobs.Start(terminalsvc.StartBashJobOptions{
		Command:        "printf stdout; printf stderr >&2",
		Cwd:            t.TempDir(),
		OwnerSessionID: "session-1",
		WorkManager:    manager,
	})
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(time.Second)
	for {
		final, waitErr := jobs.Status(snapshot.JobID, "session-1")
		if waitErr != nil {
			t.Fatal(waitErr)
		}
		if final.Status != terminalsvc.BashJobRunning {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("background shell did not finish")
		}
		time.Sleep(20 * time.Millisecond)
	}

	context := service.backgroundContext("session-1")
	for _, want := range []string{"stdout:\nstdout", "stderr:\nstderr", "Exit code: 0"} {
		if !strings.Contains(context, want) {
			t.Fatalf("background context missing %q: %s", want, context)
		}
	}
}

func TestBackgroundWakeMessageNamesTheKind(t *testing.T) {
	work := func(kinds ...backgroundsvc.Kind) []backgroundsvc.Work {
		out := make([]backgroundsvc.Work, 0, len(kinds))
		for _, kind := range kinds {
			out = append(out, backgroundsvc.Work{Kind: kind})
		}
		return out
	}
	cases := map[string][]backgroundsvc.Work{
		"Bash tool completed, continuing agent":       work(backgroundsvc.KindShell),
		"Subagent completed, continuing agent":        work(backgroundsvc.KindSubagent, backgroundsvc.KindSubagent),
		"Session finished, continuing agent":          work(backgroundsvc.KindSession),
		"Background work completed, continuing agent": work(backgroundsvc.KindSession, backgroundsvc.KindShell),
	}
	for want, items := range cases {
		if got := wakeMessage(items); got != want {
			t.Errorf("wakeMessage(%v) = %q, want %q", items, got, want)
		}
	}
}

func TestBackgroundContextNamesWaitedSessions(t *testing.T) {
	service := NewService(nil)
	manager := backgroundsvc.NewManager()
	service.SetBackgroundWorkManager(manager)
	wait, err := manager.Register(backgroundsvc.Options{
		SessionID: "session-1", Kind: backgroundsvc.KindSession,
		Metadata: map[string]string{"session_ids": "a,b", "mode": "all"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Complete(wait.ID, "session-1", backgroundsvc.StatusCompleted, "a: done", "", nil); err != nil {
		t.Fatal(err)
	}
	context := service.backgroundContext("session-1")
	for _, want := range []string{"(session)", "Sessions: a,b", "Summary: a: done"} {
		if !strings.Contains(context, want) {
			t.Fatalf("background context missing %q: %s", want, context)
		}
	}
}
