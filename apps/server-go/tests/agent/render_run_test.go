// Run service: /render switches the render skill on for a session, the skill
// rides setup, and the tool result carries the HtmlArtifact. Without /render
// the tools are gated: the first call is refused until the skill is read.
package tests

import (
	"context"
	"github.com/MoeForge/moego/json"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/loop"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/stream"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/tools"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/run"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/services/htmlrender"
	"github.com/Adelodunpeter25/console/apps/server-go/tests/helpers"
)

const renderCallArgs = `{"html":"<!doctype html><html><body><h1>Signups</h1></body></html>","title":"Signups","height":320}`

type renderCaller struct {
	mu    sync.Mutex
	calls int
	reqs  []loop.TurnRequest
}

func (p *renderCaller) RunTurn(ctx context.Context, req loop.TurnRequest, s *stream.Stream[loop.Event]) error {
	if len(req.Tools) == 0 {
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
			ID: "c1", Name: "render", Arguments: []byte(renderCallArgs),
		}})
	} else {
		s.Push(loop.Event{Kind: loop.EventText, Text: "ok"})
	}
	s.Complete()
	return nil
}

func (p *renderCaller) requests() []loop.TurnRequest {
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

func TestRenderCommandActivatesToolAndCarriesArtifact(t *testing.T) {
	sessions := helpers.NewRunSessions(t)
	svc := run.NewService(sessions)
	provider := &renderCaller{}
	svc.Lookup = func(id string) (loop.Provider, error) { return provider, nil }
	header := helpers.CreateRunSession(t, sessions)

	_ = sessions.AddCapability(header.ID, "render")
	hub, err := svc.StartRun(header.ID, run.Prompt{Text: "chart our signups", Provider: "mock", ModelID: "m"})
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

	var livePath string
	check := func(frame run.Frame) {
		if frame.Event.Kind != loop.EventToolExecutionResult || frame.Event.Result == nil {
			return
		}
		h := frame.Event.Result.Html
		if h == nil || h.Path == "" {
			return
		}
		if h.Title != "Signups" || h.Height != 320 {
			t.Errorf("unexpected html artifact: %+v", h)
		}
		livePath = h.Path
	}
	for _, frame := range replay {
		check(frame)
	}
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
	if livePath == "" {
		t.Fatal("no live tool result carried an html artifact")
	}
	if _, err := os.Stat(livePath); err != nil {
		t.Fatalf("rendered scratch file missing at %s: %v", livePath, err)
	}

	reqs := provider.requests()
	if len(reqs) < 2 {
		t.Fatalf("agent turns = %d, want at least 2", len(reqs))
	}
	first := reqs[0]
	if !hasTool(first, "code") {
		t.Fatalf("code tool missing after /render: %v", toolList(first))
	}
	if !strings.Contains(first.Setup, "Render & Preview") {
		t.Fatalf("setup should carry the /render skill:\n%s", first.Setup)
	}
	content := toolResultContent(t, reqs[1])
	if !strings.Contains(content, "Rendered interactive page") {
		t.Fatalf("provider should see the render summary, got %s", content)
	}
	if strings.Contains(content, "<h1>Signups</h1>") {
		t.Fatalf("raw html must not reach the provider, got %s", content)
	}

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
		if result.Results[0].Html != nil && result.Results[0].Html.Path == livePath {
			persisted = true
		}
	}
	if !persisted {
		t.Fatal("persisted tool result lost the html artifact")
	}
}

func TestRenderToolStaysLoadedForLaterMessages(t *testing.T) {
	sessions := helpers.NewRunSessions(t)
	svc := run.NewService(sessions)
	provider := &renderCaller{}
	svc.Lookup = func(id string) (loop.Provider, error) { return provider, nil }
	header := helpers.CreateRunSession(t, sessions)

	_ = sessions.AddCapability(header.ID, "render")
	for _, text := range []string{"chart our signups", "and now a table"} {
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
		if !hasTool(req, "code") {
			t.Fatalf("turn %d lost the code tool: %v", i, toolList(req))
		}
	}
}

func TestRenderIsGatedWithoutCommand(t *testing.T) {
	sessions := helpers.NewRunSessions(t)
	svc := run.NewService(sessions)
	provider := &renderCaller{}
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
	if strings.Contains(reqs[0].Setup, htmlrender.SkillText()) {
		t.Fatalf("setup carries the render skill without /render:\n%s", reqs[0].Setup)
	}

	refusal := toolResultContent(t, reqs[1])
	if !strings.Contains(refusal, "before reading the render skill") {
		t.Fatalf("the first call must be refused until the skill is read, got %s", refusal)
	}

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
				if r.Html != nil {
					t.Fatal("a gated call rendered an html artifact")
				}
			}
		}
	}
}
