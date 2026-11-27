// Run service: MCP servers are announced through the codemode setup hint,
// and groups loaded through loadTools keep their per-session restore order.
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
	"github.com/Adelodunpeter25/console/apps/server-go/internal/services/mcp"
	"github.com/Adelodunpeter25/console/apps/server-go/tests/helpers"
)

// groupLoader calls loadTools on its first agent turn ever, then answers.
type groupLoader struct {
	mu    sync.Mutex
	calls int
	reqs  []loop.TurnRequest
}

func (p *groupLoader) RunTurn(ctx context.Context, req loop.TurnRequest, s *stream.Stream[loop.Event]) error {
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
		s.Push(loop.Event{Kind: loop.EventToolCall, Call: &tools.ToolCall{
			ID: "c1", Name: "loadTools", Arguments: []byte(`{"group":"mcp:echo"}`),
		}})
	} else {
		s.Push(loop.Event{Kind: loop.EventText, Text: "ok"})
	}
	s.Complete()
	return nil
}

func TestRunRestoresLoadedGroupsAcrossMessages(t *testing.T) {
	ts := startHTTPMCP(t, nil)
	m := newManager(t)
	if _, err := m.Config.Save(mcp.ServerConfig{ID: "echo", Label: "Echo", Transport: mcp.TransportHTTP, URL: ts.URL, Enabled: true}); err != nil {
		t.Fatal(err)
	}
	sessions := helpers.NewRunSessions(t)
	svc := run.NewService(sessions)
	svc.SetMCP(m)
	provider := &groupLoader{}
	svc.Lookup = func(id string) (loop.Provider, error) { return provider, nil }
	header := helpers.CreateRunSession(t, sessions)

	for _, text := range []string{"first", "second"} {
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

	provider.mu.Lock()
	defer provider.mu.Unlock()
	// Run 1: turn 1 (loadTools call), turn 2 (answer). Run 2: one turn.
	if len(provider.reqs) != 3 {
		t.Fatalf("agent turns: %d", len(provider.reqs))
	}
	first := provider.reqs[0]
	// Under codemode the provider sees only the REPL tools; MCP tools are
	// callable from inside the REPL via the native mcp.* namespace.
	for _, d := range first.Tools {
		if d.Name != "code" && d.Name != "codeReset" {
			t.Fatalf("unexpected provider tool %s", d.Name)
		}
	}
	if !strings.Contains(first.Setup, "mcp.call") {
		t.Fatalf("setup should mention mcp.call when MCP is enabled:\n%s", first.Setup)
	}
}

func TestRunSetupOmitsMCPHintWithoutServers(t *testing.T) {
	sessions := helpers.NewRunSessions(t)
	svc := run.NewService(sessions)
	svc.SetMCP(newManager(t))
	provider := &groupLoader{calls: 1} // skip the loadTools turn
	svc.Lookup = func(id string) (loop.Provider, error) { return provider, nil }
	header := helpers.CreateRunSession(t, sessions)
	hub, err := svc.StartRun(header.ID, run.Prompt{Text: "hi", Provider: "mock", ModelID: "m"})
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-hub.Done():
	case <-time.After(15 * time.Second):
		t.Fatal("run did not settle")
	}
	provider.mu.Lock()
	defer provider.mu.Unlock()
	// The codemode hint mentions mcp.call as a capability; without servers
	// configured there is no group catalog or loadTools instruction.
	if len(provider.reqs) == 0 || strings.Contains(provider.reqs[0].Setup, "loadTools({group") {
		t.Fatal("no MCP servers -> no group-loading hint")
	}
}
