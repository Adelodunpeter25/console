package tests

import (
	"testing"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/services/background"
)

func TestBackgroundWorkCompletionIsClaimedOnce(t *testing.T) {
	manager := background.NewManager()
	work, err := manager.Register(background.Options{SessionID: "session-1", Kind: background.KindShell})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Complete(work.ID, "session-1", background.StatusCompleted, "done", "", nil); err != nil {
		t.Fatal(err)
	}
	first := manager.TakeCompletions("session-1")
	if len(first) != 1 || first[0].ID != work.ID {
		t.Fatalf("first delivery = %+v", first)
	}
	if second := manager.TakeCompletions("session-1"); len(second) != 0 {
		t.Fatalf("completion delivered twice: %+v", second)
	}
	if other := manager.TakeCompletions("session-2"); len(other) != 0 {
		t.Fatalf("completion crossed session boundary: %+v", other)
	}
}
