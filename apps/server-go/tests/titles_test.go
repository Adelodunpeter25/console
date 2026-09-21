// Session title coverage: detection, fallback, sanitize, LLM generation,
// and end-to-end titling on a fresh run.
package tests

import (
	"context"
	"testing"
	"time"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/loop"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/stream"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/titles"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/run"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/types"
)

func TestTitleHelpers(t *testing.T) {
	for _, generic := range []string{"", "  ", "New Session", "New mobile session", "New Chat", "Untitled"} {
		if !titles.IsGenericTitle(generic) {
			t.Fatalf("generic: %q", generic)
		}
	}
	if titles.IsGenericTitle("My project") {
		t.Fatal("real title must not be generic")
	}
	if got := titles.FallbackTitle("  hello   world  "); got != "hello world" {
		t.Fatalf("fallback: %q", got)
	}
	long := "this is a fairly long prompt that definitely exceeds thirty five characters"
	if got := titles.FallbackTitle(long); got != "this is a fairly long prompt that d..." {
		t.Fatalf("fallback trunc: %q", got)
	}
	if got := titles.SanitizeTitle(`"Fix the login bug."`); got != "Fix the login bug" {
		t.Fatalf("sanitize: %q", got)
	}
	if got := titles.SanitizeTitle("one two three four five six seven eight nine ten"); got != "one two three four five six seven eight" {
		t.Fatalf("word cap: %q", got)
	}
}

type titleProvider struct{ text string }

func (p *titleProvider) RunTurn(ctx context.Context, req loop.TurnRequest, s *stream.Stream[loop.Event]) error {
	if p.text != "" {
		s.Push(loop.Event{Kind: loop.EventText, Text: p.text})
	}
	s.Complete()
	return nil
}

func TestGenerateTitle(t *testing.T) {
	if got := titles.Generate(context.Background(), &titleProvider{text: `"Short login fix."`}, "m", "fix login"); got != "Short login fix" {
		t.Fatalf("generated: %q", got)
	}
	if got := titles.Generate(context.Background(), &titleProvider{}, "m", "fix login"); got != "" {
		t.Fatalf("empty must yield blank: %q", got)
	}
}

func TestRunTitlesFreshSession(t *testing.T) {
	sessions := newRunSessions(t)
	svc := run.NewService(sessions)
	svc.Lookup = func(id string) (loop.Provider, error) {
		return queueMock("done"), nil
	}
	header, err := sessions.Create(types.CreateSessionOptions{
		Cwd: t.TempDir(), ModelID: "m", Provider: "mock", Title: "New Session",
	})
	if err != nil {
		t.Fatal(err)
	}
	hub, err := svc.StartRun(header.ID, run.Prompt{Text: "fix the login bug please", Provider: "mock", ModelID: "m"})
	if err != nil {
		t.Fatal(err)
	}
	waitSettled(t, hub)
	// Title generation runs off the critical path; poll briefly.
	deadline := time.Now().Add(5 * time.Second)
	for {
		loaded, err := sessions.Load(header.ID, 0, 0)
		if err != nil {
			t.Fatal(err)
		}
		if !titles.IsGenericTitle(loaded.Header.Title) {
			if loaded.Header.Title != "done" {
				t.Fatalf("title: %q", loaded.Header.Title)
			}
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("title was never generated")
		}
		time.Sleep(50 * time.Millisecond)
	}
}
