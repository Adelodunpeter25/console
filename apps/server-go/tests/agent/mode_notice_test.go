// The mode notice is attached to the user message where the mode starts or
// changes, and nowhere else, so it is seen once and stays in history.
package tests

import (
	"strings"
	"testing"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/loop"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/systemprompt"
)

func modeUser(text, mode string) loop.UserMessage {
	return loop.UserMessage{Role: loop.RoleUser, Content: text, ApprovalMode: mode}
}

func TestModeNoticeAppearsOnlyWhereTheModeChanges(t *testing.T) {
	history := []any{
		modeUser("one", "full-access"),
		ckAssistant("a"),
		modeUser("two", "full-access"),
		ckAssistant("b"),
		modeUser("three", "plan-mode"),
		ckAssistant("c"),
		modeUser("four", "plan-mode"),
	}
	notice := func(m string) string { return systemprompt.ModeNotice(systemprompt.ApprovalMode(m)) }
	out := loop.ApplyModeNotices(history, notice)

	has := func(i int, want string) bool {
		return strings.Contains(out[i].(loop.UserMessage).Content, want)
	}
	if !has(0, "Full Access") || has(2, "Full Access") {
		t.Fatal("full access notice belongs on the first message only")
	}
	if !has(4, "Plan mode") || has(6, "Plan mode") {
		t.Fatal("plan notice belongs on the message where the mode changed only")
	}
	if !has(4, "three") {
		t.Fatal("the user's own text must be kept")
	}
	if history[0].(loop.UserMessage).Content != "one" {
		t.Fatal("stored history must not be modified")
	}
}

func TestModeNoticesAreIdenticalAcrossRequests(t *testing.T) {
	notice := func(m string) string { return systemprompt.ModeNotice(systemprompt.ApprovalMode(m)) }
	base := []any{modeUser("one", "plan-mode"), ckAssistant("a"), modeUser("two", "full-access")}
	first := loop.ApplyModeNotices(base, notice)
	grown := append(append([]any{}, base...), ckAssistant("b"), modeUser("three", "full-access"))
	second := loop.ApplyModeNotices(grown, notice)
	for i := range first {
		if first[i].(interface{}) == nil {
			continue
		}
	}
	for i := 0; i < len(first); i++ {
		a, aok := first[i].(loop.UserMessage)
		b, bok := second[i].(loop.UserMessage)
		if aok != bok || (aok && a.Content != b.Content) {
			t.Fatalf("message %d changed between requests: the cached prefix would shift", i)
		}
	}
}

func TestModeNoticeSkipsMessagesWithoutARecordedMode(t *testing.T) {
	notice := func(m string) string { return "N:" + m }
	out := loop.ApplyModeNotices([]any{loop.UserMessage{Role: loop.RoleUser, Content: "setup"}, modeUser("hi", "full-access")}, notice)
	if out[0].(loop.UserMessage).Content != "setup" {
		t.Fatal("a message with no recorded mode must be left alone")
	}
	if !strings.Contains(out[1].(loop.UserMessage).Content, "N:full-access") {
		t.Fatal("the first message that records a mode gets the notice")
	}
}

func TestModeNoticeTextIsShortAndSayswhereWritesAreLimited(t *testing.T) {
	full := systemprompt.ModeNotice(systemprompt.FullAccess)
	for _, want := range []string{"Execute all necessary tools without asking", "project folder", "Don't mention this notice"} {
		if !strings.Contains(full, want) {
			t.Errorf("full-access notice missing %q: %s", want, full)
		}
	}
	plan := systemprompt.ModeNotice(systemprompt.PlanMode)
	for _, want := range []string{"writePlan", "requestFullAccess", "Don't mention this notice"} {
		if !strings.Contains(plan, want) {
			t.Errorf("plan notice missing %q: %s", want, plan)
		}
	}
}
