// Follow-up suggestions: output parsing, the smol-role gate, the question
// skip, and delivery over the notification stream.
package tests

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/loop"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/stream"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/suggestions"
	consolev1 "github.com/Adelodunpeter25/console/apps/server-go/internal/gen/console/v1"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/run"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/services"
	"github.com/Adelodunpeter25/console/apps/server-go/tests/helpers"
)

func TestSuggestionsParse(t *testing.T) {
	cases := map[string][]string{
		`["Run the tests","Commit this"]`:       {"Run the tests", "Commit this"},
		"Sure:\n```json\n[\"Add a test\"]\n```": {"Add a test"},
		`["a","A","b","c"]`:                     {"a", "b"},
		`[]`:                                    nil,
		`not json`:                              nil,
		`["` + strings.Repeat("x", 80) + `","Short one"]`: {"Short one"},
		`["  ", ""]`: nil,
	}
	for in, want := range cases {
		if got := suggestions.Parse(in); !reflect.DeepEqual(got, want) {
			t.Errorf("Parse(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestSuggestionsEndsWithQuestion(t *testing.T) {
	if !suggestions.EndsWithQuestion("Which one do you prefer?  \n") {
		t.Fatal("trailing question must be detected")
	}
	if suggestions.EndsWithQuestion("All done.") {
		t.Fatal("statement is not a question")
	}
}

// suggestingProvider answers the suggestion call (recognised by its system
// prompt) with reply, and every other call with a plain assistant text.
type suggestingProvider struct {
	assistant string
	reply     string
	calls     chan struct{}
}

func (p *suggestingProvider) RunTurn(ctx context.Context, req loop.TurnRequest, s *stream.Stream[loop.Event]) error {
	text := p.assistant
	if strings.Contains(req.SystemPrompt, "what the user might say next") {
		text = p.reply
		select {
		case p.calls <- struct{}{}:
		default:
		}
	}
	s.Push(loop.Event{Kind: loop.EventText, Text: text})
	s.Complete()
	return nil
}

func setSmolRole(t *testing.T, ref string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "settings.json")
	body := `{"modelRoles":{}}`
	if ref != "" {
		body = `{"modelRoles":{"smol":"` + ref + `"}}`
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CONSOLE_SETTINGS_PATH", path)
}

func runForSuggestions(t *testing.T, assistant, reply string) (chan *consolev1.NotificationEvent, *suggestingProvider) {
	t.Helper()
	sessions := helpers.NewRunSessions(t)
	svc := run.NewService(sessions)
	bus := services.NewNotificationService()
	svc.SetNotifications(bus)
	stop := make(chan struct{})
	t.Cleanup(func() { close(stop) })
	notes := collectNotifications(t, bus, stop)
	prov := &suggestingProvider{assistant: assistant, reply: reply, calls: make(chan struct{}, 4)}
	svc.Lookup = func(id string) (loop.Provider, error) { return prov, nil }
	header := helpers.CreateRunSession(t, sessions)
	hub, err := svc.StartRun(header.ID, run.Prompt{Text: "fix it", Provider: "mock", ModelID: "m"})
	if err != nil {
		t.Fatal(err)
	}
	helpers.WaitSettled(t, hub)
	return notes, prov
}

func TestSuggestionsDeliveredWithSmolRole(t *testing.T) {
	setSmolRole(t, "codex/gpt-5.5")
	notes, _ := runForSuggestions(t, "Fixed the parser.", `["Run the tests","Commit this"]`)
	ev := waitNotification(t, notes, "suggestions")
	if !reflect.DeepEqual(ev.GetItems(), []string{"Run the tests", "Commit this"}) {
		t.Fatalf("items: %v", ev.GetItems())
	}
}

func TestSuggestionsOffWithoutSmolRole(t *testing.T) {
	setSmolRole(t, "")
	notes, prov := runForSuggestions(t, "Fixed the parser.", `["Run the tests"]`)
	expectNoSuggestions(t, notes, prov, false)
}

func TestSuggestionsSkippedAfterQuestion(t *testing.T) {
	setSmolRole(t, "codex/gpt-5.5")
	notes, prov := runForSuggestions(t, "Should I also update the docs?", `["Yes please"]`)
	expectNoSuggestions(t, notes, prov, false)
}

func TestSuggestionsDroppedWhenMalformed(t *testing.T) {
	setSmolRole(t, "codex/gpt-5.5")
	notes, prov := runForSuggestions(t, "Fixed the parser.", `sorry, no idea`)
	expectNoSuggestions(t, notes, prov, true)
}

func expectNoSuggestions(t *testing.T, notes chan *consolev1.NotificationEvent, prov *suggestingProvider, wantCalled bool) {
	t.Helper()
	deadline := time.After(700 * time.Millisecond)
	for {
		select {
		case ev := <-notes:
			if ev.GetKind() == "suggestions" {
				t.Fatalf("unexpected suggestions: %v", ev.GetItems())
			}
		case <-deadline:
			if called := len(prov.calls) > 0; called != wantCalled {
				t.Fatalf("suggestion model called = %v, want %v", called, wantCalled)
			}
			return
		}
	}
}

// gatedProvider holds the suggestion call until released, so a test can
// start a new run while a generation is in flight.
type gatedProvider struct {
	release chan struct{}
	started chan struct{}
}

func (p *gatedProvider) RunTurn(ctx context.Context, req loop.TurnRequest, s *stream.Stream[loop.Event]) error {
	text := "Fixed the parser."
	if strings.Contains(req.SystemPrompt, "what the user might say next") {
		select {
		case p.started <- struct{}{}:
		default:
		}
		<-p.release
		text = `["Run the tests"]`
	}
	s.Push(loop.Event{Kind: loop.EventText, Text: text})
	s.Complete()
	return nil
}

func TestSuggestionsDiscardedWhenNewRunStarts(t *testing.T) {
	setSmolRole(t, "codex/gpt-5.5")
	sessions := helpers.NewRunSessions(t)
	svc := run.NewService(sessions)
	bus := services.NewNotificationService()
	svc.SetNotifications(bus)
	stop := make(chan struct{})
	defer close(stop)
	notes := collectNotifications(t, bus, stop)
	prov := &gatedProvider{release: make(chan struct{}), started: make(chan struct{}, 4)}
	svc.Lookup = func(id string) (loop.Provider, error) { return prov, nil }
	header := helpers.CreateRunSession(t, sessions)

	hub, err := svc.StartRun(header.ID, run.Prompt{Text: "fix it", Provider: "mock", ModelID: "m"})
	if err != nil {
		t.Fatal(err)
	}
	helpers.WaitSettled(t, hub)
	select {
	case <-prov.started:
	case <-time.After(5 * time.Second):
		t.Fatal("suggestion generation never started")
	}
	// A new run makes the in-flight result stale.
	hub, err = svc.StartRun(header.ID, run.Prompt{Text: "and again", Provider: "mock", ModelID: "m"})
	if err != nil {
		t.Fatal(err)
	}
	close(prov.release)
	helpers.WaitSettled(t, hub)
	waitNotification(t, notes, "suggestions")
	select {
	case ev := <-notes:
		if ev.GetKind() == "suggestions" {
			t.Fatalf("stale suggestions delivered: %v", ev.GetItems())
		}
	case <-time.After(700 * time.Millisecond):
	}
}
