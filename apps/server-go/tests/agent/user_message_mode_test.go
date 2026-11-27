// The user message records the approval mode its turn ran under, so clients
// can mark plan-mode prompts in history.
package tests

import (
	"testing"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/loop"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/tools"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/run"
	"github.com/Adelodunpeter25/console/apps/server-go/tests/helpers"
)

func TestUserMessageRecordsApprovalMode(t *testing.T) {
	sessions := helpers.NewRunSessions(t)
	svc := run.NewService(sessions)
	svc.Lookup = func(id string) (loop.Provider, error) {
		return &helpers.MockProvider{Turns: []func() []loop.Event{
			func() []loop.Event { return []loop.Event{{Kind: loop.EventText, Text: "ok"}} },
			func() []loop.Event { return []loop.Event{{Kind: loop.EventText, Text: "ok"}} },
		}}, nil
	}
	header := helpers.CreateRunSession(t, sessions)

	for _, mode := range []string{"plan-mode", "full-access"} {
		hub, err := svc.StartRun(header.ID, run.Prompt{Text: mode, Provider: "mock", ModelID: "m", ApprovalMode: mode})
		if err != nil {
			t.Fatal(err)
		}
		helpers.WaitSettled(t, hub)
	}

	loaded, err := sessions.Load(header.ID, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]string{}
	for _, raw := range loaded.Messages {
		msg, err := loop.MessageFromProtoBytes(raw)
		if err != nil {
			t.Fatal(err)
		}
		if user, ok := msg.(loop.UserMessage); ok {
			seen[user.Content] = user.ApprovalMode
		}
	}
	if seen["plan-mode"] != "plan-mode" || seen["full-access"] != "full-access" {
		t.Fatalf("recorded modes = %v", seen)
	}

	// Messages stored before the field existed decode with no mode.
	old, err := loop.MessageFromProtoBytes([]byte(`{"user":{"content":"old"}}`))
	if err != nil {
		t.Fatal(err)
	}
	if user := old.(loop.UserMessage); user.ApprovalMode != "" {
		t.Fatalf("legacy mode = %q, want empty", user.ApprovalMode)
	}
}

func TestStaleModeGuardPreventsDemotion(t *testing.T) {
	sessions := helpers.NewRunSessions(t)
	svc := run.NewService(sessions)
	svc.Lookup = func(id string) (loop.Provider, error) {
		return &helpers.MockProvider{Turns: []func() []loop.Event{
			func() []loop.Event { return []loop.Event{{Kind: loop.EventText, Text: "ok"}} },
		}}, nil
	}
	header := helpers.CreateRunSession(t, sessions)
	// The session was upgraded to full-access (e.g. via requestFullAccess or PATCH).
	if err := sessions.UpdateApprovalMode(header.ID, "full-access"); err != nil {
		t.Fatal(err)
	}

	// A client sends a prompt carrying stale "plan-mode".
	hub, err := svc.StartRun(header.ID, run.Prompt{Text: "implement it", Provider: "mock", ModelID: "m", ApprovalMode: "plan-mode"})
	if err != nil {
		t.Fatal(err)
	}
	helpers.WaitSettled(t, hub)

	loaded, err := sessions.Load(header.ID, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	var lastUser loop.UserMessage
	for _, raw := range loaded.Messages {
		msg, err := loop.MessageFromProtoBytes(raw)
		if err != nil {
			t.Fatal(err)
		}
		if u, ok := msg.(loop.UserMessage); ok {
			lastUser = u
		}
	}
	if lastUser.ApprovalMode != "full-access" {
		t.Fatalf("expected turn to run as full-access under stale-mode guard, got %q", lastUser.ApprovalMode)
	}
}

func TestRequestFullAccessBroadcastsModeUpdateEvent(t *testing.T) {
	sessions := helpers.NewRunSessions(t)
	svc := run.NewService(sessions)
	var sawModeUpdate string
	header := helpers.CreateRunSession(t, sessions)
	// Start in plan mode
	if err := sessions.UpdateApprovalMode(header.ID, "plan-mode"); err != nil {
		t.Fatal(err)
	}

	svc.Lookup = func(string) (loop.Provider, error) {
		return &helpers.MockProvider{Turns: []func() []loop.Event{
			func() []loop.Event {
				return []loop.Event{
					{
						Kind: loop.EventToolCall,
						Call: &tools.ToolCall{
							ID:        "call_1",
							Name:      "requestFullAccess",
							Arguments: []byte(`{"reason":"implement feature"}`),
						},
					},
				}
			},
			func() []loop.Event {
				return []loop.Event{{Kind: loop.EventText, Text: "done"}}
			},
		}}, nil
	}

	hub, err := svc.StartRun(header.ID, run.Prompt{Text: "please implement", Provider: "mock", ModelID: "m", ApprovalMode: "plan-mode"})
	if err != nil {
		t.Fatal(err)
	}
	subID, ch, _ := hub.Subscribe(nil)
	defer hub.Unsubscribe(subID)
	done := make(chan struct{})
	go func() {
		defer close(done)
		for f := range ch {
			if f.Event.Kind == loop.EventSessionApprovalModeUpdated {
				sawModeUpdate = f.Event.ApprovalMode
			}
			if f.Event.Kind == loop.EventAskQuestion && f.Event.Ask != nil {
				svc.AnswerQuestion(header.ID, f.Event.Ask.RequestID, tools.AskAnswer{Text: tools.FullAccessApprove})
			}
		}
	}()

	helpers.WaitSettled(t, hub)
	<-done

	if sawModeUpdate != "full-access" {
		t.Fatalf("expected EventSessionApprovalModeUpdated with full-access, got %q", sawModeUpdate)
	}
	h, err := sessions.Header(header.ID)
	if err != nil || h.ApprovalMode != "full-access" {
		t.Fatalf("session header should be updated to full-access, got %+v", h)
	}
}
