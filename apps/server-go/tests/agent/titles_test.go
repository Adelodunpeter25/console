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
	"github.com/Adelodunpeter25/console/apps/server-go/tests/helpers"
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
	if got := titles.FallbackTitle("  hello   world  "); got != "Hello world" {
		t.Fatalf("fallback: %q", got)
	}
	long := "this is a fairly long prompt that definitely exceeds thirty five characters"
	if got := titles.FallbackTitle(long); got != "This is a fairly long prompt that d..." {
		t.Fatalf("fallback trunc: %q", got)
	}
	if got := titles.SanitizeTitle(`"Fix the login bug."`); got != "Fix the login bug" {
		t.Fatalf("sanitize: %q", got)
	}
	if got := titles.SanitizeTitle("one two three four five six seven eight nine ten"); got != "One two three four five six seven eight" {
		t.Fatalf("word cap: %q", got)
	}
}

// Titles reach the UI from three paths (create, LLM generation, manual
// rename) and all three must leave the first letter capitalized, since
// desktop and mobile render the stored value verbatim.
func TestTitlesCapitalizeFirstLetter(t *testing.T) {
	cases := []struct{ in, want string }{
		{"fix login bug", "Fix login bug"},
		{"  spaced out  ", "Spaced out"},
		{"ALL CAPS", "ALL CAPS"},
		{"already Fine", "Already Fine"},
		{"eBay fixes", "EBay fixes"},
		{"🔥 fix login", "🔥 Fix login"},
		// SanitizeTitle strips edge quotes and lead markers before
		// capitalizing, so these land on the first letter itself.
		{`"quoted title"`, "Quoted title"},
		{"- dash led", "Dash led"},
		{"...ellipsis", "...Ellipsis"},
		// Caseless leading runes are skipped: the first *letter* is
		// what gets capitalized, not position zero.
		{"123 numeric", "123 Numeric"},
		{"v2 api", "V2 api"},
		{"日本語 title", "日本語 Title"},
		{"élan vital", "Élan vital"},
		{"", ""},
		{"   ", ""},
	}
	for _, c := range cases {
		if got := titles.SanitizeTitle(c.in); got != c.want {
			t.Errorf("SanitizeTitle(%q) = %q; want %q", c.in, got, c.want)
		}
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
	sessions := helpers.NewRunSessions(t)
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
	helpers.WaitSettled(t, hub)
	// Title generation runs off the critical path; poll briefly.
	deadline := time.Now().Add(5 * time.Second)
	for {
		loaded, err := sessions.Load(header.ID, 0, 0)
		if err != nil {
			t.Fatal(err)
		}
		if !titles.IsGenericTitle(loaded.Header.Title) {
			// The mock streams "done"; generation capitalizes it, so the
			// stored title is "Done" — the rule holds end to end.
			if loaded.Header.Title != "Done" {
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
