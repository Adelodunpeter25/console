// Approval + question decision coverage: allow/deny/answer end-to-end
// through a live run, timeouts, and session-scoped rejection.
package tests

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/loop"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/permissions"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/tools"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/run"
	"github.com/Adelodunpeter25/console/apps/server-go/tests/helpers"
)

// waitHubFrame returns the first frame of the wanted kind, or fails.
// It subscribes with since=0 so already-broadcast frames replay instead
// of being missed by a live-only subscription.
func waitHubFrame(t *testing.T, hub *run.Hub, kind loop.EventKind, timeout time.Duration) loop.Event {
	t.Helper()
	zero := int64(0)
	id, ch, replay := hub.Subscribe(&zero)
	if ch == nil {
		t.Fatalf("hub closed before %s arrived", kind)
	}
	defer hub.Unsubscribe(id)
	for _, f := range replay {
		if f.Event.Kind == kind {
			return f.Event
		}
	}
	deadline := time.After(timeout)
	for {
		select {
		case f, ok := <-ch:
			if !ok {
				t.Fatalf("hub closed before %s arrived", kind)
			}
			if f.Event.Kind == kind {
				return f.Event
			}
		case <-deadline:
			t.Fatalf("timed out waiting for %s", kind)
		}
	}
}

func writeCallArgs(t *testing.T, path, content string) json.RawMessage {
	t.Helper()
	raw, err := json.Marshal(map[string]any{"path": path, "content": content})
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func TestApprovalAllowRunsTool(t *testing.T) {
	sessions := helpers.NewRunSessions(t)
	svc := run.NewService(sessions)
	target := filepath.Join(t.TempDir(), "hello.txt")
	svc.Lookup = func(id string) (loop.Provider, error) {
		return &helpers.MockProvider{Turns: []func() []loop.Event{
			func() []loop.Event {
				return []loop.Event{{Kind: loop.EventToolCall, Call: &tools.ToolCall{
					ID: "c1", Name: "write_file", Arguments: writeCallArgs(t, target, "hi"),
				}}}
			},
			func() []loop.Event { return []loop.Event{{Kind: loop.EventText, Text: "done"}} },
		}}, nil
	}
	header := helpers.CreateRunSession(t, sessions)
	hub, err := svc.StartRun(header.ID, run.Prompt{Text: "write hi", Provider: "mock", ModelID: "m"})
	if err != nil {
		t.Fatal(err)
	}
	// always-ask (session default) prompts for write-tier tools.
	req := waitHubFrame(t, hub, loop.EventPermissionRequest, 10*time.Second)
	if req.Permission == nil || req.Permission.ToolName != "write_file" {
		t.Fatalf("permission request: %+v", req.Permission)
	}
	if !svc.ApprovePermission(header.ID, req.Permission.RequestID, true) {
		t.Fatal("approve must resolve")
	}
	select {
	case <-hub.Done():
	case <-time.After(10 * time.Second):
		t.Fatal("run did not settle")
	}
	if hub.Outcome != run.OutcomeDone {
		t.Fatalf("outcome: %s", hub.Outcome)
	}
	data, err := os.ReadFile(target)
	if err != nil || string(data) != "hi" {
		t.Fatalf("file: %v %q", err, data)
	}
}

func TestApprovalDenyFailsTool(t *testing.T) {
	sessions := helpers.NewRunSessions(t)
	svc := run.NewService(sessions)
	target := filepath.Join(t.TempDir(), "nope.txt")
	svc.Lookup = func(id string) (loop.Provider, error) {
		return &helpers.MockProvider{Turns: []func() []loop.Event{
			func() []loop.Event {
				return []loop.Event{{Kind: loop.EventToolCall, Call: &tools.ToolCall{
					ID: "c1", Name: "write_file", Arguments: writeCallArgs(t, target, "hi"),
				}}}
			},
			func() []loop.Event { return []loop.Event{{Kind: loop.EventText, Text: "ok"}} },
		}}, nil
	}
	header := helpers.CreateRunSession(t, sessions)
	hub, err := svc.StartRun(header.ID, run.Prompt{Text: "write hi", Provider: "mock", ModelID: "m"})
	if err != nil {
		t.Fatal(err)
	}
	req := waitHubFrame(t, hub, loop.EventPermissionRequest, 10*time.Second)
	if !svc.ApprovePermission(header.ID, req.Permission.RequestID, false) {
		t.Fatal("deny must resolve")
	}
	select {
	case <-hub.Done():
	case <-time.After(10 * time.Second):
		t.Fatal("run did not settle")
	}
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Fatal("denied write must not create the file")
	}
}

func TestAskQuestionAnswered(t *testing.T) {
	sessions := helpers.NewRunSessions(t)
	svc := run.NewService(sessions)
	askArgs, _ := json.Marshal(map[string]any{"question": "Pick a color?", "options": []string{"red", "blue"}})
	svc.Lookup = func(id string) (loop.Provider, error) {
		return &helpers.MockProvider{Turns: []func() []loop.Event{
			func() []loop.Event {
				return []loop.Event{{Kind: loop.EventToolCall, Call: &tools.ToolCall{
					ID: "q1", Name: "ask", Arguments: askArgs,
				}}}
			},
			func() []loop.Event { return []loop.Event{{Kind: loop.EventText, Text: "thanks"}} },
		}}, nil
	}
	header := helpers.CreateRunSession(t, sessions)
	hub, err := svc.StartRun(header.ID, run.Prompt{Text: "ask me", Provider: "mock", ModelID: "m"})
	if err != nil {
		t.Fatal(err)
	}
	req := waitHubFrame(t, hub, loop.EventAskQuestion, 10*time.Second)
	if req.Ask == nil || req.Ask.Question != "Pick a color?" {
		t.Fatalf("ask request: %+v", req.Ask)
	}
	if !svc.AnswerQuestion(header.ID, req.Ask.RequestID, tools.AskAnswer{Text: "blue"}) {
		t.Fatal("answer must resolve")
	}
	select {
	case <-hub.Done():
	case <-time.After(10 * time.Second):
		t.Fatal("run did not settle")
	}
	if hub.Outcome != run.OutcomeDone {
		t.Fatalf("outcome: %s", hub.Outcome)
	}
}

func TestDecisionTimeout(t *testing.T) {
	d := run.NewDecisions()
	d.Timeout = 50 * time.Millisecond
	hub := run.NewHub()
	defer hub.Close(run.OutcomeDone)
	approver := d.ApproverFor("s1", hub)
	if _, err := approver.Approve(context.Background(), permissions.Request{RequestID: "perm_x", ToolName: "bash"}); err == nil ||
		!strings.Contains(err.Error(), "timed out") {
		t.Fatalf("expected timeout, got %v", err)
	}
	handler := d.AskHandlerFor("s1", hub)
	if _, err := handler(context.Background(), tools.AskQuestionRequest{RequestID: "q_x", Question: "?"}); err == nil ||
		!strings.Contains(err.Error(), "timed out") {
		t.Fatalf("expected timeout, got %v", err)
	}
}

func TestRejectAllIsSessionScoped(t *testing.T) {
	sessions := helpers.NewRunSessions(t)
	svc := run.NewService(sessions)
	svc.Lookup = func(id string) (loop.Provider, error) {
		return &helpers.MockProvider{Turns: []func() []loop.Event{
			func() []loop.Event {
				args, _ := json.Marshal(map[string]any{"path": filepath.Join(t.TempDir(), "x.txt"), "content": "hi"})
				return []loop.Event{{Kind: loop.EventToolCall, Call: &tools.ToolCall{ID: "c1", Name: "write_file", Arguments: args}}}
			},
			func() []loop.Event { return []loop.Event{{Kind: loop.EventText, Text: "done"}} },
		}}, nil
	}
	headerA := helpers.CreateRunSession(t, sessions)
	headerB := helpers.CreateRunSession(t, sessions)
	hubA, err := svc.StartRun(headerA.ID, run.Prompt{Text: "a", Provider: "mock", ModelID: "m"})
	if err != nil {
		t.Fatal(err)
	}
	hubB, err := svc.StartRun(headerB.ID, run.Prompt{Text: "b", Provider: "mock", ModelID: "m"})
	if err != nil {
		t.Fatal(err)
	}
	waitHubFrame(t, hubA, loop.EventPermissionRequest, 10*time.Second)
	reqB := waitHubFrame(t, hubB, loop.EventPermissionRequest, 10*time.Second)
	// Aborting A must not resolve B's pending approval.
	if !svc.Abort(headerA.ID) {
		t.Fatal("abort A must succeed")
	}
	select {
	case <-hubA.Done():
	case <-time.After(10 * time.Second):
		t.Fatal("run A did not settle")
	}
	if hubA.Outcome != run.OutcomeAborted {
		t.Fatalf("outcome A: %s", hubA.Outcome)
	}
	if !svc.IsActive(headerB.ID) {
		t.Fatal("run B must still be active")
	}
	if !svc.ApprovePermission(headerB.ID, reqB.Permission.RequestID, true) {
		t.Fatal("run B approval must still resolve")
	}
	select {
	case <-hubB.Done():
	case <-time.After(10 * time.Second):
		t.Fatal("run B did not settle")
	}
	if hubB.Outcome != run.OutcomeDone {
		t.Fatalf("outcome B: %s", hubB.Outcome)
	}
}
