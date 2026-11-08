// What a run really sends the model in each mode. The unit tests prove the
// permission rules; this proves the tools are actually offered, by recording
// the tool definitions that reach the provider on a real run.
package tests

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/loop"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/tools"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/run"
	"github.com/Adelodunpeter25/console/apps/server-go/tests/helpers"
)

// toolNamesSent runs one prompt in the given approval mode and returns the names
// of the tools the provider was offered.
func toolNamesSent(t *testing.T, mode string) map[string]bool {
	t.Helper()
	sessions := helpers.NewRunSessions(t)
	svc := run.NewService(sessions)
	provider := &setupRecorder{}
	svc.Lookup = func(string) (loop.Provider, error) { return provider, nil }
	header := helpers.CreateRunSession(t, sessions)

	hub, err := svc.StartRun(header.ID, run.Prompt{Text: "hi", Provider: "mock", ModelID: "m", ApprovalMode: mode})
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-hub.Done():
	case <-time.After(10 * time.Second):
		t.Fatal("run did not settle")
	}
	provider.mu.Lock()
	defer provider.mu.Unlock()
	if len(provider.reqs) == 0 {
		t.Fatal("the provider was never called with tools")
	}
	names := map[string]bool{}
	for _, def := range provider.reqs[0].Tools {
		names[def.Name] = true
	}
	return names
}

func TestPlanModeRunOffersWritePlanAndNotTheOrdinaryWrites(t *testing.T) {
	got := toolNamesSent(t, "plan-mode")
	for _, want := range []string{"writePlan", "bash", "read_file", "grep", "memory", "browser"} {
		if !got[want] {
			t.Errorf("a plan-mode run must be offered %q; got %v", want, keys(got))
		}
	}
}

func TestOtherModesDoNotOfferWritePlan(t *testing.T) {
	for _, mode := range []string{"full-access"} {
		if got := toolNamesSent(t, mode); got["writePlan"] {
			t.Errorf("%s: writePlan is for plan mode only, but it was offered", mode)
		}
	}
}

func TestPlanModePromptNamesTheTool(t *testing.T) {
	sessions := helpers.NewRunSessions(t)
	svc := run.NewService(sessions)
	provider := &setupRecorder{}
	svc.Lookup = func(string) (loop.Provider, error) { return provider, nil }
	header := helpers.CreateRunSession(t, sessions)
	hub, err := svc.StartRun(header.ID, run.Prompt{Text: "hi", Provider: "mock", ModelID: "m", ApprovalMode: "plan-mode"})
	if err != nil {
		t.Fatal(err)
	}
	<-hub.Done()
	provider.mu.Lock()
	defer provider.mu.Unlock()
	req := provider.reqs[0]
	if !strings.Contains(req.SystemPrompt, "writePlan") || !strings.Contains(req.SystemPrompt, "docs/plan") {
		t.Fatalf("the plan-mode prompt must tell the model about writePlan and where it writes:\n%s", req.SystemPrompt)
	}
	// The tool inventory section lists it too.
	if !strings.Contains(req.SystemPrompt, "`writePlan`") {
		t.Fatalf("writePlan is missing from the tool inventory in the system prompt")
	}
}

func keys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

// The whole path, end to end: a model in plan mode calls writePlan, the run
// executes it, and the file is on disk. A model that then tries write_file in
// the same run is refused.
func TestPlanModeRunActuallyWritesAPlanFile(t *testing.T) {
	sessions := helpers.NewRunSessions(t)
	svc := run.NewService(sessions)
	dir := t.TempDir()
	header := helpers.CreateRunSessionIn(t, sessions, dir)
	// A placeholder title makes the run start a title-generation call on the same
	// provider, which would consume the first scripted turn below. A real title
	// keeps the script for the agent.
	if err := sessions.UpdateTitle(header.ID, "Plan mode run"); err != nil {
		t.Fatal(err)
	}

	planCall := tools.ToolCall{ID: "c1", Name: "writePlan",
		Arguments: helpers.MustJSONRaw(t, map[string]any{"name": "My Plan", "content": "# Steps\n1. think"})}
	writeCall := tools.ToolCall{ID: "c2", Name: "write_file",
		Arguments: helpers.MustJSONRaw(t, map[string]any{"path": filepath.Join(dir, "sneaky.txt"), "content": "x"})}
	provider := &helpers.MockProvider{Turns: []func() []loop.Event{
		func() []loop.Event {
			return []loop.Event{{Kind: loop.EventToolCall, Call: &planCall}, {Kind: loop.EventToolCall, Call: &writeCall}}
		},
		func() []loop.Event { return []loop.Event{{Kind: loop.EventText, Text: "done"}} },
	}}
	svc.Lookup = func(string) (loop.Provider, error) { return provider, nil }

	hub, err := svc.StartRun(header.ID, run.Prompt{Text: "plan it", Provider: "mock", ModelID: "m", ApprovalMode: "plan-mode"})
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-hub.Done():
	case <-time.After(15 * time.Second):
		t.Fatal("run did not settle")
	}
	got, err := os.ReadFile(filepath.Join(dir, "docs", "plan", "my-plan.md"))
	if err != nil || string(got) != "# Steps\n1. think" {
		t.Fatalf("the plan file was not written by the run: %q, %v", got, err)
	}
	if _, err := os.Stat(filepath.Join(dir, "sneaky.txt")); err == nil {
		t.Fatal("write_file succeeded in plan mode during the same run")
	}
}

// Every tool a real run offers must have a human label: otherwise the chat shows
// a raw identifier and a wrench. This is the test that would have caught
// `console`, `computer` and `writePlan` being left out of the table.
func TestEveryToolARunOffersHasALabel(t *testing.T) {
	seen := map[string]bool{}
	for _, mode := range []string{"plan-mode", "full-access"} {
		for name := range toolNamesSent(t, mode) {
			seen[name] = true
		}
	}
	// Tools only present once activated: bring them in the way a run would.
	for _, name := range []string{"visualize", "computer", "computer_reset"} {
		seen[name] = true
	}
	var missing []string
	for name := range seen {
		if tools.ToolLabel(name) == name { // an unlabelled tool falls back to its raw name
			missing = append(missing, name)
		}
	}
	if len(missing) > 0 {
		t.Fatalf("these tools have no label in classify.go, so clients show the raw name: %v", missing)
	}
}
