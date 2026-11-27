// A compaction checkpoint keeps the compacted prefix stable across turns, so
// the provider's cache breaks once at compaction instead of on every turn.
package tests

import (
	"reflect"
	"testing"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/compaction"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/loop"
)

func ckUser(text string) loop.UserMessage {
	return loop.UserMessage{Role: loop.RoleUser, Content: text}
}

func ckAssistant(text string) loop.AssistantMessage {
	return loop.AssistantMessage{Role: loop.RoleAssistant, Content: []any{loop.TextPart{Type: "text", Text: text}}}
}

func TestCheckpointReusesPrefixAndAppendsNewMessages(t *testing.T) {
	var cp compaction.Checkpoint
	history := []any{ckUser("a"), ckAssistant("b"), ckUser("c"), ckAssistant("d")}
	summary := []any{ckUser("summary"), ckUser("c"), ckAssistant("d")}
	if _, ok := cp.Apply(history); ok {
		t.Fatal("no checkpoint yet")
	}
	cp.Set(history, summary)

	next := append(append([]any{}, history...), ckUser("e"), ckAssistant("f"))
	got, ok := cp.Apply(next)
	if !ok {
		t.Fatal("expected the checkpoint to apply to a grown history")
	}
	want := append(append([]any{}, summary...), ckUser("e"), ckAssistant("f"))
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v, want the summary prefix plus new messages", got)
	}
	again, _ := cp.Apply(append(next, ckUser("g")))
	if !reflect.DeepEqual(again[:len(summary)], summary) {
		t.Fatal("the compacted prefix must be identical on every later turn")
	}
}

func TestCheckpointDropsWhenHistoryNoLongerMatches(t *testing.T) {
	var cp compaction.Checkpoint
	history := []any{ckUser("a"), ckAssistant("b"), ckUser("c")}
	cp.Set(history, []any{ckUser("summary")})

	if _, ok := cp.Apply(history[:2]); ok {
		t.Fatal("a rewound history must not reuse the checkpoint")
	}
	if _, ok := cp.Apply(history); ok {
		t.Fatal("a dropped checkpoint stays dropped")
	}

	cp.Set(history, []any{ckUser("summary")})
	edited := []any{ckUser("a"), ckAssistant("b"), ckUser("different")}
	if _, ok := cp.Apply(edited); ok {
		t.Fatal("an edited message must not reuse the checkpoint")
	}
}
