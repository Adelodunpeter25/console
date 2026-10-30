package run

import (
	"strings"
	"testing"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/tools"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/db"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/services"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/types"
)

func newCapService(t *testing.T) (*Service, string) {
	t.Helper()
	manager, err := db.Open(db.OpenOptions{Path: ":memory:"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(manager.Close)
	sessions := services.NewSessionService(manager)
	header, err := sessions.Create(types.CreateSessionOptions{Cwd: t.TempDir(), ModelID: "m", Provider: "mock"})
	if err != nil {
		t.Fatal(err)
	}
	return NewService(sessions), header.ID
}

func TestTypedCommandSwitchesOnAndPersists(t *testing.T) {
	svc, id := newCapService(t)
	if on, _ := svc.sessions.Capabilities(id); len(on) != 0 {
		t.Fatalf("a fresh session must have nothing on: %v", on)
	}
	svc.activateInvoked(id, "/visualize show me a chart")
	svc.activateInvoked(id, "just a normal message")
	on, _ := svc.sessions.Capabilities(id)
	if len(on) != 1 || on[0] != "visualize" {
		t.Fatalf("on = %v, want [visualize]", on)
	}
	// Idempotent.
	svc.activateInvoked(id, "/visualize again")
	if on, _ := svc.sessions.Capabilities(id); len(on) != 1 {
		t.Fatalf("activation must be idempotent: %v", on)
	}
	// Survives a restart: a new service over the same store still knows.
	restarted := NewService(svc.sessions)
	if on, _ := restarted.sessions.Capabilities(id); len(on) != 1 {
		t.Fatalf("activation lost across a restart: %v", on)
	}
}

func TestEveryCommandIsDetected(t *testing.T) {
	for _, c := range Capabilities {
		if _, ok := c.Split("/" + c.Name); !ok {
			t.Errorf("/%s is not recognised by its own Split", c.Name)
		}
		if _, ok := c.Split("not a command"); ok {
			t.Errorf("%s matched plain text", c.Name)
		}
	}
}

func TestSetupCarriesOnlyCapabilitiesThatAreOn(t *testing.T) {
	svc, id := newCapService(t)
	if refs := svc.capabilitySetup(id); len(refs) != 0 {
		t.Fatalf("nothing on, setup must carry nothing: %d refs", len(refs))
	}
	svc.activateInvoked(id, "/console")
	refs := svc.capabilitySetup(id)
	if len(refs) != 1 || !strings.Contains(refs[0], "console") {
		t.Fatalf("refs = %d", len(refs))
	}
}

func TestReadSkillSwitchesOnWhatTheAgentMayEnable(t *testing.T) {
	svc, id := newCapService(t)
	registry := tools.NewRegistry()
	lc := &loadContext{sessionID: id, cwd: t.TempDir(), mode: "always-ask"}
	tool := tools.NewReadSkillTool(svc.readSkillCapabilities(lc, registry))
	read := func(name string) string {
		out, err := tool.Execute(t.Context(), []byte(`{"name":"`+name+`"}`))
		if err != nil {
			t.Fatalf("readSkill %s: %v", name, err)
		}
		return textOf(out)
	}

	// The agent forgot to ask the user to type /visualize: reading it switches
	// it on, loads its tool into this run, and persists the choice.
	text := read("visualize")
	if !strings.Contains(text, "now switched on") {
		t.Fatalf("expected a switched-on note, got: %.200s", text)
	}
	if _, err := registry.Get("visualize"); err != nil {
		t.Fatalf("visualize tool was not loaded into the run: %v", err)
	}
	if on, _ := svc.sessions.Capabilities(id); !contains(on, "visualize") {
		t.Fatalf("not persisted: %v", on)
	}

	// Computer use needs the user: the agent is told to ask, and nothing turns on.
	text = read("computer-use")
	if !strings.Contains(text, "only the user can switch it on") || !strings.Contains(text, "/computer-use") {
		t.Fatalf("expected an ask-the-user note, got: %.200s", text)
	}
	if on, _ := svc.sessions.Capabilities(id); contains(on, "computer-use") {
		t.Fatal("the agent must not be able to switch on computer use")
	}
	if _, err := registry.Get("computer-use"); err == nil {
		t.Fatal("computer-use tools must not be loaded")
	}
}

func TestReadSkillListsCapabilitiesWithTheirState(t *testing.T) {
	svc, id := newCapService(t)
	svc.activateInvoked(id, "/visualize")
	lc := &loadContext{sessionID: id}
	tool := tools.NewReadSkillTool(svc.readSkillCapabilities(lc, tools.NewRegistry()))
	out, err := tool.Execute(t.Context(), []byte(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	text := textOf(out)
	for _, want := range []string{"visualize", "(built-in, on)", "computer-use", "(built-in, off)", "console"} {
		if !strings.Contains(text, want) {
			t.Errorf("listing missing %q:\n%s", want, text)
		}
	}
}

func TestReadSkillUnknownNameListsCapabilities(t *testing.T) {
	svc, id := newCapService(t)
	tool := tools.NewReadSkillTool(svc.readSkillCapabilities(&loadContext{sessionID: id}, tools.NewRegistry()))
	_, err := tool.Execute(t.Context(), []byte(`{"name":"nope"}`))
	if err == nil || !strings.Contains(err.Error(), "visualize") {
		t.Fatalf("err = %v, want the built-ins listed", err)
	}
}

func TestPopupListComesFromTheTable(t *testing.T) {
	got := CapabilityDescriptions()
	if len(got) != len(Capabilities) {
		t.Fatalf("popup has %d entries for %d capabilities", len(got), len(Capabilities))
	}
	for i, c := range Capabilities {
		if got[i][0] != c.Name || got[i][1] == "" {
			t.Errorf("entry %d = %v", i, got[i])
		}
	}
}

// textOf extracts the text of a tool result.
func textOf(out any) string {
	switch v := out.(type) {
	case string:
		return v
	case []map[string]any:
		var b strings.Builder
		for _, part := range v {
			if s, ok := part["text"].(string); ok {
				b.WriteString(s)
			}
		}
		return b.String()
	}
	return ""
}
