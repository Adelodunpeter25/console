// /computer-use invocation: the command match and the skill text. Matching
// is deliberately narrow — the model has no prior knowledge of computer use,
// so a false positive would summon tools for a message that merely mentions
// them, and a false negative would ignore the user.
package tests

import (
	"strings"
	"testing"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/services/cua"
)

func TestSplitInvocation(t *testing.T) {
	cases := []struct {
		text string
		task string
		ok   bool
	}{
		{"/computer-use turn off the Tailscale login item", "turn off the Tailscale login item", true},
		{"/computer-use", "", true},
		{"/computer-use   ", "", true},
		{"  /computer-use calculate 7x8", "calculate 7x8", true},
		{"/computer-use\nmulti\nline", "multi\nline", true},
		{"/computer-useful", "", false},
		{"/computer-use2", "", false},
		{"please /computer-use this", "", false},
		{"computer-use without the slash", "", false},
		{"/computer", "", false},
		{"", "", false},
	}
	for _, c := range cases {
		task, ok := cua.SplitInvocation(c.text)
		if ok != c.ok || task != c.task {
			t.Errorf("%q: got (%q, %v), want (%q, %v)", c.text, task, ok, c.task, c.ok)
		}
	}
}

// TestSkillTextCarriesTheContract pins what the model is told on invocation:
// the two tool names, the calling convention, and the rules that keep it
// from misgrounding actions. An empty or gutted skill would leave the model
// with tools it cannot use correctly.
func TestSkillTextCarriesTheContract(t *testing.T) {
	skill := cua.SkillText()
	if strings.TrimSpace(skill) == "" {
		t.Fatal("skill text is empty")
	}
	for _, want := range []string{
		"computer_reset",
		"get_window_state",
		"click",
		"element_token",
		"element_id",
		"snapshot",
		"background",
		"verify_state",
		"label_contains",
		"value_equals",
		"set_value",
		"confirmation",
		"did not hold",
		"cua.list_apps",
	} {
		if !strings.Contains(skill, want) {
			t.Errorf("skill does not mention %q", want)
		}
	}
	if !strings.Contains(strings.ToLower(skill), "async") {
		t.Error("skill must state the sync-only rule")
	}
	// The model sees the raw "/computer-use <task>" message (never rewritten),
	// so the skill must explain the invocation form.
	if !strings.Contains(skill, "/computer-use <task>") {
		t.Error("skill must explain the invocation form")
	}
}
