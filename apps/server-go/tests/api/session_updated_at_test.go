// Sidebar ordering: updated_at moves on a user prompt and on run finish,
// but streamed assistant messages and reads must not reorder the list.
package tests

import (
	"testing"
	"time"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/types"
	"github.com/Adelodunpeter25/console/apps/server-go/tests/helpers"
)

func headerUpdatedAt(t *testing.T, sessions interface {
	Header(string) (*types.SessionHeader, error)
}, id string) int64 {
	t.Helper()
	h, err := sessions.Header(id)
	if err != nil {
		t.Fatal(err)
	}
	return h.UpdatedAt
}

func TestUpdatedAtOnlyMovesOnUserActivity(t *testing.T) {
	_, _, sessions := settleRoutesApp(t)
	header := helpers.CreateRunSession(t, sessions)
	initial := headerUpdatedAt(t, sessions, header.ID)

	// Simulate stream latency so a bump would be observable.
	time.Sleep(5 * time.Millisecond)

	// Streamed assistant/tool messages must not bump updated_at.
	if err := sessions.AppendMessages(header.ID, []types.AgentMessage{
		{ID: "a1", Role: "assistant", Data: []byte(`{"assistant":{}}`)},
	}); err != nil {
		t.Fatal(err)
	}
	if got := headerUpdatedAt(t, sessions, header.ID); got != initial {
		t.Fatalf("streamed message moved updated_at: %d -> %d", initial, got)
	}

	// A user prompt does.
	if err := sessions.AppendMessage(header.ID, types.AgentMessage{
		ID: "u1", Role: "user", Data: []byte(`{"user":{"content":"hi"}}`),
	}); err != nil {
		t.Fatal(err)
	}
	afterPrompt := headerUpdatedAt(t, sessions, header.ID)
	if afterPrompt <= initial {
		t.Fatalf("user prompt did not move updated_at: %d -> %d", initial, afterPrompt)
	}

	// Run finish does too.
	time.Sleep(5 * time.Millisecond)
	if err := sessions.FinishRun(header.ID, "done"); err != nil {
		t.Fatal(err)
	}
	if got := headerUpdatedAt(t, sessions, header.ID); got <= afterPrompt {
		t.Fatalf("run finish did not move updated_at: %d -> %d", afterPrompt, got)
	}
}
