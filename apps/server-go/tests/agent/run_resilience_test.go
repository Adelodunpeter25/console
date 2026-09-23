// Run resilience: decision failures stay tool-scoped, aborts emit no error
// frame, slow subscribers keep every frame, and a stale run's cleanup never
// touches a newer run.
package tests

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/loop"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/permissions"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/tools"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/run"
	"github.com/Adelodunpeter25/console/apps/server-go/tests/helpers"
)

type failingApprover struct{ err error }

func (f failingApprover) Approve(context.Context, permissions.Request) (bool, error) {
	return false, f.err
}

func TestExecutorDecisionFailureIsToolResult(t *testing.T) {
	failing := tools.NewAskTool(func(context.Context, tools.AskQuestionRequest) (tools.AskAnswer, error) {
		return tools.AskAnswer{}, errors.New("Question timed out waiting for a decision.")
	})
	exec := loop.NewExecutor(tools.NewRegistry(failing), permissions.AlwaysAsk, nil)
	args, _ := json.Marshal(map[string]any{"question": "?"})
	res, err := exec.Execute(context.Background(), tools.ToolCall{ID: "q", Name: "ask", Arguments: args})
	if err != nil {
		t.Fatalf("ask failure must not abort the turn: %v", err)
	}
	if !res.IsError || !strings.Contains(res.Content.(string), "timed out") {
		t.Fatalf("result: %+v", res)
	}

	write := loop.NewExecutor(tools.NewRegistry(tools.DefaultTools()...), permissions.AlwaysAsk, failingApprover{errors.New("Permission request timed out waiting for a decision.")})
	res, err = write.Execute(context.Background(), tools.ToolCall{ID: "w", Name: "write_file", Arguments: writeCallArgs(t, t.TempDir()+"/x", "x")})
	if err != nil || !res.IsError || !strings.Contains(res.Content.(string), "timed out") {
		t.Fatalf("approval failure: %v %+v", err, res)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	res, err = write.Execute(ctx, tools.ToolCall{ID: "w2", Name: "write_file", Arguments: writeCallArgs(t, t.TempDir()+"/y", "y")})
	if err != nil || !res.IsError || res.Content != "Tool execution cancelled by user abort." {
		t.Fatalf("aborted approval: %v %+v", err, res)
	}
}

func TestAskTimeoutContinuesRun(t *testing.T) {
	sessions := helpers.NewRunSessions(t)
	svc := run.NewService(sessions)
	svc.SetDecisionTimeout(50 * time.Millisecond)
	askArgs, _ := json.Marshal(map[string]any{"question": "Pick?"})
	svc.Lookup = func(string) (loop.Provider, error) {
		return &helpers.MockProvider{Turns: []func() []loop.Event{
			func() []loop.Event {
				return []loop.Event{{Kind: loop.EventToolCall, Call: &tools.ToolCall{ID: "q1", Name: "ask", Arguments: askArgs}}}
			},
			func() []loop.Event { return []loop.Event{{Kind: loop.EventText, Text: "moving on"}} },
		}}, nil
	}
	header := helpers.CreateRunSession(t, sessions)
	hub, err := svc.StartRun(header.ID, run.Prompt{Text: "ask", Provider: "mock", ModelID: "m"})
	if err != nil {
		t.Fatal(err)
	}
	zero := int64(0)
	_, ch, replay := hub.Subscribe(&zero)
	frames := collect(t, ch, replay)
	if hub.Outcome != run.OutcomeDone {
		t.Fatalf("outcome: %s", hub.Outcome)
	}
	for _, f := range frames {
		if f.Event.Kind == loop.EventError {
			t.Fatalf("unexpected error frame: %s", f.Event.Text)
		}
	}
}

func TestAbortDuringQuestionHasNoErrorFrame(t *testing.T) {
	sessions := helpers.NewRunSessions(t)
	svc := run.NewService(sessions)
	askArgs, _ := json.Marshal(map[string]any{"question": "Pick?"})
	svc.Lookup = func(string) (loop.Provider, error) {
		return &helpers.MockProvider{Turns: []func() []loop.Event{
			func() []loop.Event {
				return []loop.Event{{Kind: loop.EventToolCall, Call: &tools.ToolCall{ID: "q1", Name: "ask", Arguments: askArgs}}}
			},
			func() []loop.Event { return []loop.Event{{Kind: loop.EventText, Text: "never"}} },
		}}, nil
	}
	header := helpers.CreateRunSession(t, sessions)
	hub, err := svc.StartRun(header.ID, run.Prompt{Text: "ask", Provider: "mock", ModelID: "m"})
	if err != nil {
		t.Fatal(err)
	}
	zero := int64(0)
	_, ch, replay := hub.Subscribe(&zero)
	waitHubFrame(t, hub, loop.EventAskQuestion, 10*time.Second)
	if !svc.Abort(header.ID) {
		t.Fatal("abort")
	}
	frames := collect(t, ch, replay)
	if hub.Outcome != run.OutcomeAborted {
		t.Fatalf("outcome: %s", hub.Outcome)
	}
	sawEnd := false
	for _, f := range frames {
		if f.Event.Kind == loop.EventError {
			t.Fatalf("abort must not emit error frame: %s", f.Event.Text)
		}
		sawEnd = sawEnd || f.Event.Kind == loop.EventSessionEnd
	}
	if !sawEnd {
		t.Fatal("missing sessionEnd")
	}
}

func TestHubSlowSubscriberKeepsAllFrames(t *testing.T) {
	hub := run.NewHub()
	_, ch, _ := hub.Subscribe(nil)
	const n = 5000 // far above the old 256-slot eviction threshold
	for i := 0; i < n; i++ {
		hub.Broadcast(loop.Event{Kind: loop.EventText})
	}
	hub.Broadcast(loop.Event{Kind: loop.EventSessionEnd})
	hub.Close(run.OutcomeDone)
	frames := collect(t, ch, nil)
	if len(frames) != n+1 || frames[n].Event.Kind != loop.EventSessionEnd {
		t.Fatalf("got %d frames", len(frames))
	}
	for i, f := range frames {
		if f.Seq != int64(i+1) {
			t.Fatalf("gap at %d: seq %d", i, f.Seq)
		}
	}
}

func TestHubUnsubscribeClosesChannel(t *testing.T) {
	hub := run.NewHub()
	defer hub.Close(run.OutcomeDone)
	id, ch, _ := hub.Subscribe(nil)
	hub.Broadcast(loop.Event{Kind: loop.EventText})
	hub.Unsubscribe(id)
	deadline := time.After(2 * time.Second)
	for {
		select {
		case _, ok := <-ch:
			if !ok {
				return
			}
		case <-deadline:
			t.Fatal("channel not closed after unsubscribe")
		}
	}
}

func collect(t *testing.T, ch <-chan run.Frame, replay []run.Frame) []run.Frame {
	t.Helper()
	out := append([]run.Frame{}, replay...)
	deadline := time.After(10 * time.Second)
	for {
		select {
		case f, ok := <-ch:
			if !ok {
				return out
			}
			out = append(out, f)
		case <-deadline:
			t.Fatal("stream did not close")
		}
	}
}
