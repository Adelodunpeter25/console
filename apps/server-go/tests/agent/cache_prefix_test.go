// The cached head of a request (system prompt, setup, earlier messages) must
// be byte-identical between turns; mode and other per-turn context ride in the
// tail.
package tests

import (
	"reflect"
	"strings"
	"testing"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/loop"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/run"
	"github.com/Adelodunpeter25/console/apps/server-go/tests/helpers"
)

func TestModeChangeKeepsCachedPrefixIdentical(t *testing.T) {
	sessions := helpers.NewRunSessions(t)
	svc := run.NewService(sessions)
	provider := &setupRecorder{}
	svc.Lookup = func(string) (loop.Provider, error) { return provider, nil }
	header := helpers.CreateRunSession(t, sessions)

	for _, mode := range []string{"plan-mode", "full-access"} {
		hub, err := svc.StartRun(header.ID, run.Prompt{Text: "hi " + mode, Provider: "mock", ModelID: "m", ApprovalMode: mode})
		if err != nil {
			t.Fatal(err)
		}
		<-hub.Done()
	}

	provider.mu.Lock()
	defer provider.mu.Unlock()
	if len(provider.reqs) != 2 {
		t.Fatalf("provider calls: %d", len(provider.reqs))
	}
	first, second := provider.reqs[0], provider.reqs[1]
	if first.SystemPrompt != second.SystemPrompt || first.Setup != second.Setup {
		t.Fatal("a mode change must not alter the system prompt or setup")
	}
	if !reflect.DeepEqual(first.Messages, second.Messages[:len(first.Messages)]) {
		t.Fatalf("earlier messages must be an unchanged prefix:\n%#v\n%#v", first.Messages, second.Messages)
	}
	lastUser := func(r loop.TurnRequest) string {
		u, _ := r.Messages[len(r.Messages)-1].(loop.UserMessage)
		return u.Content
	}
	if !strings.Contains(lastUser(first), "Plan mode") {
		t.Fatalf("the first turn must carry the plan-mode notice:\n%s", lastUser(first))
	}
	if !strings.Contains(lastUser(second), "Full Access") {
		t.Fatalf("the turn where the mode changes must carry the new notice:\n%s", lastUser(second))
	}
	if strings.Contains(first.Tail, "Plan mode") || strings.Contains(second.Tail, "Full Access") {
		t.Fatal("the mode notice must not ride in the per-request tail")
	}
}
