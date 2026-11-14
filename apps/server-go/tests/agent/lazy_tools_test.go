// Lazy built-in tools (CONSOLE_HARNESS_LAZY_TOOLS): deferred tools are
// absent from the first request, appear after a loadTools call, persist
// across runs of the session, and bashJob auto-loads when bash starts a
// background job.
package tests

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/loop"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/stream"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/tools"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/run"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/types"
	"github.com/Adelodunpeter25/console/apps/server-go/tests/helpers"
)

// lazyProvider runs a scripted sequence of events per agent turn.
type lazyProvider struct {
	mu     sync.Mutex
	reqs   []loop.TurnRequest
	script [][]loop.Event
}

func (p *lazyProvider) RunTurn(ctx context.Context, req loop.TurnRequest, s *stream.Stream[loop.Event]) error {
	if len(req.Tools) == 0 { // title generation
		s.Push(loop.Event{Kind: loop.EventText, Text: "title"})
		s.Complete()
		return nil
	}
	p.mu.Lock()
	n := len(p.reqs)
	p.reqs = append(p.reqs, req)
	p.mu.Unlock()
	if n < len(p.script) {
		for _, e := range p.script[n] {
			s.Push(e)
		}
	}
	s.Complete()
	return nil
}

func (p *lazyProvider) names(req loop.TurnRequest) map[string]bool {
	out := map[string]bool{}
	for _, d := range req.Tools {
		out[d.Name] = true
	}
	return out
}

func newLazyService(t *testing.T, provider *lazyProvider) (*run.Service, types.SessionHeader) {
	t.Helper()
	sessions := helpers.NewRunSessions(t)
	svc := run.NewService(sessions)
	svc.Lookup = func(string) (loop.Provider, error) { return provider, nil }
	header := helpers.CreateRunSession(t, sessions)
	return svc, header
}

func settleRun(t *testing.T, svc *run.Service, header types.SessionHeader, text string) {
	t.Helper()
	hub, err := svc.StartRun(header.ID, run.Prompt{Text: text, Provider: "mock", ModelID: "m", ApprovalMode: "full-access"})
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-hub.Done():
	case <-time.After(15 * time.Second):
		t.Fatal("run did not settle")
	}
}

func toolCallEvent(name, args string) loop.Event {
	return loop.Event{Kind: loop.EventToolCall, Call: &tools.ToolCall{ID: "c1", Name: name, Arguments: []byte(args)}}
}

func TestLazyToolsHoldBackAndLoad(t *testing.T) {
	t.Setenv("CONSOLE_HARNESS_LAZY_TOOLS", "1")
	provider := &lazyProvider{script: [][]loop.Event{
		{toolCallEvent("loadTools", `{"names":["webSearch"]}`)},
		{{Kind: loop.EventText, Text: "ok"}},
		{{Kind: loop.EventText, Text: "ok"}},
	}}
	svc, header := newLazyService(t, provider)

	settleRun(t, svc, header, "first")
	settleRun(t, svc, header, "second")

	provider.mu.Lock()
	defer provider.mu.Unlock()
	// Run 1: turn 1 (loadTools call), turn 2 (answer). Run 2: one turn.
	if len(provider.reqs) != 3 {
		t.Fatalf("agent turns: %d", len(provider.reqs))
	}
	first, afterLoad, nextMessage := provider.reqs[0], provider.reqs[1], provider.reqs[2]

	firstNames := provider.names(first)
	for _, deferred := range []string{"webSearch", "webFetch", "bashJob", "askMany", "memory", "batchWrite"} {
		if firstNames[deferred] {
			t.Fatalf("deferred tool %s must not be in the first request", deferred)
		}
	}
	if !firstNames["loadTools"] {
		t.Fatal("loadTools must be in the first request")
	}
	if !firstNames["read_file"] || !firstNames["bash"] {
		t.Fatal("core tools must stay in the first request")
	}
	if !strings.Contains(first.Setup, "Deferred tools") {
		t.Fatalf("setup should list deferred tools:\n%s", first.Setup)
	}
	if !provider.names(afterLoad)["webSearch"] {
		t.Fatal("loaded tool must appear on the next turn")
	}
	// A later run of the same session restores the loaded tool, mirroring
	// the MCP group restore.
	if !provider.names(nextMessage)["webSearch"] {
		t.Fatal("loaded tool must be restored on the next user message")
	}
}

func TestLazyToolsAutoLoadBashJobOnBackgroundStart(t *testing.T) {
	t.Setenv("CONSOLE_HARNESS_LAZY_TOOLS", "1")
	provider := &lazyProvider{script: [][]loop.Event{
		{toolCallEvent("bash", `{"command":"echo hi","background":true}`)},
		{{Kind: loop.EventText, Text: "ok"}},
	}}
	svc, header := newLazyService(t, provider)
	// No SetBashJobs: the service self-heals a manager, and the auto-load
	// must work on a bare construction too.
	settleRun(t, svc, header, "start a job")

	provider.mu.Lock()
	defer provider.mu.Unlock()
	// Turn 1 starts the job; turn 2 answers after the result. A fast job may
	// also fire its completion notification as an extra synthetic turn, so
	// only the first request is pinned down.
	if len(provider.reqs) < 2 {
		t.Fatalf("agent turns: %d", len(provider.reqs))
	}
	if provider.names(provider.reqs[0])["bashJob"] {
		t.Fatal("bashJob must be deferred on the first request")
	}
	for _, req := range provider.reqs[1:] {
		if provider.names(req)["bashJob"] {
			return // auto-loaded after the background job started
		}
	}
	t.Fatal("bashJob must auto-load after bash started a background job")
}
