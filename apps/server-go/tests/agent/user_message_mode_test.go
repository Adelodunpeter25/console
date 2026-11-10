// The user message records the approval mode its turn ran under, so clients
// can mark plan-mode prompts in history.
package tests

import (
	"testing"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/loop"
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
