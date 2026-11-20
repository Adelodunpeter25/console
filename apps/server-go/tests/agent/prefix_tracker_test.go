// The prefix tracker names the part of a request head that changed since the
// previous request in the same conversation.
package tests

import (
	"testing"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/loop"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/tools"
)

func TestPrefixTrackerNamesWhatChanged(t *testing.T) {
	var p loop.PrefixTracker
	defs := []tools.Definition{{Name: "code"}}
	history := []any{ckUser("a"), ckAssistant("b")}

	if got := p.Observe("c", "sys", "setup", defs, history); got != "" {
		t.Fatalf("first request has nothing to compare, got %q", got)
	}
	grown := append(append([]any{}, history...), ckUser("c"))
	if got := p.Observe("c", "sys", "setup", defs, grown); got != "" {
		t.Fatalf("appending messages keeps the prefix, got %q", got)
	}
	if got := p.Observe("c", "sys2", "setup", defs, grown); got != "system" {
		t.Fatalf("got %q, want system", got)
	}
	if got := p.Observe("c", "sys2", "setup2", defs, grown); got != "setup" {
		t.Fatalf("got %q, want setup", got)
	}
	if got := p.Observe("c", "sys2", "setup2", []tools.Definition{{Name: "code"}, {Name: "x"}}, grown); got != "tools" {
		t.Fatalf("got %q, want tools", got)
	}
	edited := []any{ckUser("a"), ckAssistant("changed"), ckUser("c")}
	if got := p.Observe("c", "sys2", "setup2", []tools.Definition{{Name: "code"}, {Name: "x"}}, edited); got != "messages@1" {
		t.Fatalf("got %q, want messages@1", got)
	}
	if got := p.Observe("c", "sys2", "setup2", []tools.Definition{{Name: "code"}, {Name: "x"}}, edited[:1]); got != "messages shrank" {
		t.Fatalf("got %q, want messages shrank", got)
	}
	if got := p.Observe("other", "sys", "setup", defs, history); got != "" {
		t.Fatalf("conversations are tracked separately, got %q", got)
	}
}
