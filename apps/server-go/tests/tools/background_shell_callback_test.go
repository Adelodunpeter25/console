package tests

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/tools"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/services"
)

func TestBackgroundShellPublishesCompletionCallback(t *testing.T) {
	workManager := services.NewBackgroundWorkManager()
	events := make(chan services.BackgroundWorkEvent, 4)
	workManager.SetListener(func(event services.BackgroundWorkEvent) { events <- event })
	jobs := services.NewBashJobManager(nil)
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

	var started, completed services.BackgroundWorkEvent
	deadline := time.After(time.Second)
	for started.Work.ID == "" || completed.Work.ID == "" {
		select {
		case event := <-events:
			switch event.Type {
			case services.BackgroundWorkStartedEvent:
				started = event
			case services.BackgroundWorkCompletedEvent:
				completed = event
			}
		case <-deadline:
			t.Fatalf("timed out waiting for callback: started=%+v completed=%+v", started, completed)
		}
	}
	if started.Work.SessionID != "session-1" || completed.Work.ID != started.Work.ID {
		t.Fatalf("unexpected callback ownership or ids: started=%+v completed=%+v", started, completed)
	}
	if completed.Work.Status != services.BackgroundWorkCompleted {
		t.Fatalf("unexpected completion status: %+v", completed.Work)
	}
	if completed.Work.Metadata["job_id"] == "" {
		t.Fatalf("completion did not retain backend job id: %+v", completed.Work)
	}
}
