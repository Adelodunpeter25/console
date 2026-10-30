package tests

import (
	"context"
	"encoding/json"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/services/terminal"
	"strings"
	"testing"
	"time"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/tools"
	backgroundsvc "github.com/Adelodunpeter25/console/apps/server-go/internal/services/background"
)

func TestBackgroundShellPublishesCompletionCallback(t *testing.T) {
	workManager := backgroundsvc.NewManager()
	events := make(chan backgroundsvc.Event, 4)
	workManager.SetListener(func(event backgroundsvc.Event) { events <- event })
	jobs := terminal.NewBashJobManager(nil)
	jobs.SetBackgroundWorkManager(workManager)

	bash := tools.NewBashTool(jobs, "session-1")
	args, _ := json.Marshal(map[string]any{"command": "printf callback", "background": true})
	result, err := bash.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	start := resultText(t, result)
	if !strings.Contains(start, "Background work: work_") {
		t.Fatalf("start result did not include work id: %s", start)
	}

	var started, completed backgroundsvc.Event
	deadline := time.After(time.Second)
	for started.Work.ID == "" || completed.Work.ID == "" {
		select {
		case event := <-events:
			switch event.Type {
			case backgroundsvc.EventStarted:
				started = event
			case backgroundsvc.EventCompleted:
				completed = event
			}
		case <-deadline:
			t.Fatalf("timed out waiting for callback: started=%+v completed=%+v", started, completed)
		}
	}
	if started.Work.SessionID != "session-1" || completed.Work.ID != started.Work.ID {
		t.Fatalf("unexpected callback ownership or ids: started=%+v completed=%+v", started, completed)
	}
	if completed.Work.Status != backgroundsvc.StatusCompleted {
		t.Fatalf("unexpected completion status: %+v", completed.Work)
	}
	if completed.Work.Metadata["job_id"] == "" {
		t.Fatalf("completion did not retain backend job id: %+v", completed.Work)
	}
}

// The agent's bashJob kill goes straight to BashJobManager.Kill rather than
// the UI cancel route, so it must still publish the terminal work event; a
// missing event leaves the record "running" and the shell chip visible.
func TestBackgroundShellKillPublishesCancellation(t *testing.T) {
	workManager := backgroundsvc.NewManager()
	events := make(chan backgroundsvc.Event, 8)
	workManager.SetListener(func(event backgroundsvc.Event) { events <- event })
	jobs := terminal.NewBashJobManager(nil)
	jobs.SetBackgroundWorkManager(workManager)

	bash := tools.NewBashTool(jobs, "session-1")
	bashJob := tools.NewBashJobTool(jobs, "session-1")
	args, _ := json.Marshal(map[string]any{"command": "sleep 30", "background": true})
	result, err := bash.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	start := resultText(t, result)
	var jobID, workID string
	for _, line := range strings.Split(start, "\n") {
		switch {
		case strings.HasPrefix(line, "Started: "):
			jobID = strings.TrimSpace(strings.TrimPrefix(line, "Started: "))
		case strings.HasPrefix(line, "Background work: "):
			workID = strings.TrimSpace(strings.TrimPrefix(line, "Background work: "))
		}
	}
	if jobID == "" || workID == "" {
		t.Fatalf("start result missing ids: %s", start)
	}

	killArgs, _ := json.Marshal(map[string]any{"action": "kill", "jobId": jobID})
	if _, err := bashJob.Execute(context.Background(), killArgs); err != nil {
		t.Fatalf("kill: %v", err)
	}

	var cancelled backgroundsvc.Event
	deadline := time.After(2 * time.Second)
	for cancelled.Work.ID == "" {
		select {
		case event := <-events:
			if event.Type == backgroundsvc.EventCancelled {
				cancelled = event
			}
		case <-deadline:
			t.Fatalf("timed out waiting for cancellation callback for %s", workID)
		}
	}
	if cancelled.Work.ID != workID || cancelled.Work.Status != backgroundsvc.StatusCancelled {
		t.Fatalf("unexpected cancellation event: %+v", cancelled.Work)
	}

	work, err := workManager.Get(workID, "session-1")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if work.Status != backgroundsvc.StatusCancelled {
		t.Fatalf("work record still %q after kill: %+v", work.Status, work)
	}
}
