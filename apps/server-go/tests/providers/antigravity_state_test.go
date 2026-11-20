// Antigravity conversation identity survives a server restart, so the
// endpoint keeps recognising the conversation and its cache.
package tests

import (
	"testing"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/providers/antigravity"
)

func TestAntigravityIdentitySurvivesRestart(t *testing.T) {
	t.Setenv("CONSOLE_STORAGE_DIR", t.TempDir())
	antigravity.ResetStateStore()

	state := antigravity.StateForConversation("conv-1")
	antigravity.BuildEnvelope(state, "m")
	antigravity.UpdateLastExecutionID(state, "exec-1")
	antigravity.SaveState("conv-1", state)

	antigravity.ResetStateStore() // restart
	restored := antigravity.StateForConversation("conv-1")
	if restored.SessionID != state.SessionID || restored.TrajectoryID != state.TrajectoryID || restored.AgentID != state.AgentID {
		t.Fatalf("identity changed across restart: %+v vs %+v", restored, state)
	}
	if restored.StepIndex != 1 || restored.LastExecutionID != "exec-1" {
		t.Fatalf("progress lost: step=%d exec=%q", restored.StepIndex, restored.LastExecutionID)
	}
	if other := antigravity.StateForConversation("conv-2"); other.SessionID == state.SessionID {
		t.Fatal("conversations must not share an identity")
	}
}
