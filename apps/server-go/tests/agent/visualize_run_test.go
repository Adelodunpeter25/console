// Run service: /visualize switches the visual skill on for a session, the
// skill rides setup, and the tool result carries the artifact. The tool itself
// is always listed (see run.Capabilities), but a session that never asked for
// the skill gets neither its reference nor a rendered card: the first call is
// refused until the skill is read.
package tests

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/loop"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/stream"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/tools"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/run"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/services/visualize"
	"github.com/Adelodunpeter25/console/apps/server-go/tests/helpers"
)

const visualizeCallArgs = `{"spec":{"version":1,"title":"Signups","blocks":[{"type":"metric","label":"MRR","value":"$42.1k"}]}}`

// visualizeCaller calls the visualize tool on its first agent turn, then
// answers. The first turn waits a moment so the test can subscribe to the
// hub before the run produces events.
type visualizeCaller struct {
	mu    sync.Mutex
	calls int
	reqs  []loop.TurnRequest
}

func (p *visualizeCaller) RunTurn(ctx context.Context, req loop.TurnRequest, s *stream.Stream[loop.Event]) error {
	if len(req.Tools) == 0 { // title generation
		s.Push(loop.Event{Kind: loop.EventText, Text: "title"})
		s.Complete()
		return nil
	}
	p.mu.Lock()
	p.calls++
	n := p.calls
	p.reqs = append(p.reqs, req)
	p.mu.Unlock()
	if n == 1 {
		time.Sleep(100 * time.Millisecond)
		s.Push(loop.Event{Kind: loop.EventToolCall, Call: &tools.ToolCall{
			ID: "c1", Name: "visualize", Arguments: []byte(visualizeCallArgs),
		}})
	} else {
		s.Push(loop.Event{Kind: loop.EventText, Text: "ok"})
	}
	s.Complete()
	return nil
}

func (p *visualizeCaller) requests() []loop.TurnRequest {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]loop.TurnRequest(nil), p.reqs...)
}

func hasTool(req loop.TurnRequest, name string) bool {
	for _, d := range req.Tools {
		if d.Name == name {
			return true
		}
	}
	return false
}

func toolList(req loop.TurnRequest) []string {
	names := make([]string, 0, len(req.Tools))
	for _, d := range req.Tools {
		names = append(names, d.Name)
	}
	return names
}

// toolResultContent returns the provider-facing content of the tool result
// in a request, so the test can prove the spec never reaches the provider.
// Providers read Content only, so that is what the model sees.
func toolResultContent(t *testing.T, req loop.TurnRequest) string {
	t.Helper()
	for _, message := range req.Messages {
		msg, ok := message.(loop.ToolResultMessage)
		if !ok || len(msg.Results) == 0 {
			continue
		}
		raw, err := json.Marshal(msg.Results[0].Content)
		if err != nil {
			t.Fatal(err)
		}
		return string(raw)
	}
	return ""
}

func TestVisualizeCommandActivatesToolAndCarriesArtifact(t *testing.T) {
	sessions := helpers.NewRunSessions(t)
	svc := run.NewService(sessions)
	provider := &visualizeCaller{}
	svc.Lookup = func(id string) (loop.Provider, error) { return provider, nil }
	header := helpers.CreateRunSession(t, sessions)

	hub, err := svc.StartRun(header.ID, run.Prompt{Text: "/visualize chart our signups", Provider: "mock", ModelID: "m"})
	if err != nil {
		t.Fatal(err)
	}
	var since int64
	id, frames, replay := hub.Subscribe(&since)
	defer hub.Unsubscribe(id)
	select {
	case <-hub.Done():
	case <-time.After(15 * time.Second):
		t.Fatal("run did not settle")
	}

	sawVisual := false
	check := func(frame run.Frame) {
		if frame.Event.Kind != loop.EventToolExecutionResult || frame.Event.Result == nil {
			return
		}
		visual := frame.Event.Result.Visual
		if visual == nil || visual.Version != 1 {
			return
		}
		if !json.Valid(visual.Spec) {
			t.Error("live visual spec is not valid JSON")
		}
		sawVisual = true
	}
	for _, frame := range replay {
		check(frame)
	}
	// The subscriber queue is pumped asynchronously, so drain until it goes
	// quiet rather than assuming the frames are already waiting. The channel
	// closes when the run settles.
	idle := time.After(300 * time.Millisecond)
	for {
		select {
		case frame, ok := <-frames:
			if !ok {
				goto drained
			}
			check(frame)
			idle = time.After(300 * time.Millisecond)
		case <-idle:
			goto drained
		}
	}
drained:
	if !sawVisual {
		t.Fatal("no live tool result carried a visual artifact")
	}

	reqs := provider.requests()
	if len(reqs) < 2 {
		t.Fatalf("agent turns = %d, want at least 2", len(reqs))
	}
	first := reqs[0]
	if !hasTool(first, "visualize") {
		t.Fatalf("visualize tool missing after /visualize: %v", toolList(first))
	}
	if !strings.Contains(first.Setup, "Visualize") {
		t.Fatalf("setup should carry the /visualize skill:\n%s", first.Setup)
	}
	// The provider sees the summary only: the spec is client-facing.
	content := toolResultContent(t, reqs[1])
	if !strings.Contains(content, "Rendered visual") {
		t.Fatalf("provider should see the visual summary, got %s", content)
	}
	if strings.Contains(content, "blocks") {
		t.Fatalf("the spec must not reach the provider, got %s", content)
	}

	// The artifact is persisted with the tool result, so a reopened session
	// can still render the card.
	loaded, err := sessions.Load(header.ID, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	persisted := false
	for _, message := range loaded.Messages {
		decoded, err := loop.MessageFromProtoBytes(message)
		if err != nil {
			continue
		}
		result, ok := decoded.(loop.ToolResultMessage)
		if !ok || len(result.Results) == 0 {
			continue
		}
		if result.Results[0].Visual != nil {
			persisted = true
		}
	}
	if !persisted {
		t.Fatal("persisted tool result lost the visual artifact")
	}
}

func TestVisualizeToolStaysLoadedForLaterMessages(t *testing.T) {
	sessions := helpers.NewRunSessions(t)
	svc := run.NewService(sessions)
	provider := &visualizeCaller{}
	svc.Lookup = func(id string) (loop.Provider, error) { return provider, nil }
	header := helpers.CreateRunSession(t, sessions)

	for _, text := range []string{"/visualize chart our signups", "and now a table"} {
		hub, err := svc.StartRun(header.ID, run.Prompt{Text: text, Provider: "mock", ModelID: "m"})
		if err != nil {
			t.Fatal(err)
		}
		select {
		case <-hub.Done():
		case <-time.After(15 * time.Second):
			t.Fatal("run did not settle")
		}
	}

	for i, req := range provider.requests() {
		if !hasTool(req, "visualize") {
			t.Fatalf("turn %d lost the visualize tool: %v", i, toolList(req))
		}
	}
}

// Without /visualize the tool is still listed, but it is gated: the skill
// reference is not in setup, a call renders nothing, and the refusal hands the
// model the reference so the retry works. This is what stops a stray call from
// drawing a card in a chat that never asked for one.
func TestVisualizeIsGatedWithoutCommand(t *testing.T) {
	sessions := helpers.NewRunSessions(t)
	svc := run.NewService(sessions)
	provider := &visualizeCaller{}
	svc.Lookup = func(id string) (loop.Provider, error) { return provider, nil }
	header := helpers.CreateRunSession(t, sessions)

	hub, err := svc.StartRun(header.ID, run.Prompt{Text: "chart our signups", Provider: "mock", ModelID: "m"})
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-hub.Done():
	case <-time.After(15 * time.Second):
		t.Fatal("run did not settle")
	}

	reqs := provider.requests()
	if len(reqs) < 2 {
		t.Fatalf("agent turns = %d, want at least 2", len(reqs))
	}
	if strings.Contains(reqs[0].Setup, visualize.SkillText()) {
		t.Fatalf("setup carries the visualize skill without /visualize:\n%s", reqs[0].Setup)
	}

	// The model's call was refused, with the reference as the answer.
	refusal := toolResultContent(t, reqs[1])
	if !strings.Contains(refusal, "before reading the visualize skill") {
		t.Fatalf("the first call must be refused until the skill is read, got %s", refusal)
	}

	// Nothing rendered: no persisted tool result carries a visual artifact.
	loaded, err := sessions.Load(header.ID, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, message := range loaded.Messages {
		decoded, err := loop.MessageFromProtoBytes(message)
		if err != nil {
			continue
		}
		if result, ok := decoded.(loop.ToolResultMessage); ok {
			for _, r := range result.Results {
				if r.Visual != nil {
					t.Fatal("a gated call rendered a visual artifact")
				}
			}
		}
	}
}
