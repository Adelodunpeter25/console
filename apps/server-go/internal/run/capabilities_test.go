package run

import (
	"context"
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
	svc.activateInvoked(id, "/computer-use click the button")
	svc.activateInvoked(id, "just a normal message")
	// Non-slash capabilities must not activate from /render or /console.
	svc.activateInvoked(id, "/render show me a chart")
	svc.activateInvoked(id, "/console run dev")
	on, _ := svc.sessions.Capabilities(id)
	if len(on) != 1 || on[0] != "computer-use" {
		t.Fatalf("on = %v, want [computer-use]", on)
	}
	// Idempotent.
	svc.activateInvoked(id, "/computer-use again")
	if on, _ := svc.sessions.Capabilities(id); len(on) != 1 {
		t.Fatalf("activation must be idempotent: %v", on)
	}
	// Survives a restart.
	restarted := NewService(svc.sessions)
	if on, _ := restarted.sessions.Capabilities(id); len(on) != 1 {
		t.Fatalf("activation lost across a restart: %v", on)
	}
}

func TestEverySlashCommandIsDetected(t *testing.T) {
	for _, c := range Capabilities {
		if !c.SlashCommand {
			if c.Split != nil {
				t.Errorf("%s is not a SlashCommand but still defines Split", c.Name)
			}
			continue
		}
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
	_ = svc.sessions.AddCapability(id, "console")
	refs := svc.capabilitySetup(id)
	if len(refs) != 1 || !strings.Contains(refs[0], "console") {
		t.Fatalf("refs = %d", len(refs))
	}
}

func TestReadSkillSwitchesOnWhatTheAgentMayEnable(t *testing.T) {
	svc, id := newCapService(t)
	registry := tools.NewRegistry()
	lc := &loadContext{sessionID: id, cwd: t.TempDir(), mode: "full-access"}
	tool := tools.NewReadSkillTool(svc.readSkillCapabilities(lc, registry))
	read := func(name string) string {
		out, err := tool.Execute(t.Context(), []byte(`{"name":"`+name+`"}`))
		if err != nil {
			t.Fatalf("readSkill %s: %v", name, err)
		}
		return textOf(out)
	}

	text := read("render")
	if !strings.Contains(text, "now switched on") {
		t.Fatalf("expected a switched-on note, got: %.200s", text)
	}
	if _, err := registry.Get("render"); err != nil {
		t.Fatalf("render tool was not loaded into the run: %v", err)
	}
	if on, _ := svc.sessions.Capabilities(id); !contains(on, "render") {
		t.Fatalf("not persisted: %v", on)
	}

}

func readComputerUse(t *testing.T, svc *Service, id string, ask tools.AskHandler) (string, error) {
	t.Helper()
	registry := tools.NewRegistry()
	lc := &loadContext{sessionID: id, cwd: t.TempDir(), mode: "full-access", askHandler: ask}
	tool := tools.NewReadSkillTool(svc.readSkillCapabilities(lc, registry))
	out, err := tool.Execute(t.Context(), []byte(`{"name":"computer-use"}`))
	if err != nil {
		return "", err
	}
	return textOf(out), nil
}

func TestComputerUseNeedsTheUsersConsent(t *testing.T) {
	var asked []string
	allow := func(answer string) tools.AskHandler {
		return func(_ context.Context, req tools.AskQuestionRequest) (tools.AskAnswer, error) {
			asked = append(asked, req.Question)
			return tools.AskAnswer{Text: answer}, nil
		}
	}

	t.Run("denied leaves it off", func(t *testing.T) {
		svc, id := newCapService(t)
		if _, err := readComputerUse(t, svc, id, allow("Deny")); err == nil {
			t.Fatal("a declined request must fail")
		}
		if on, _ := svc.sessions.Capabilities(id); contains(on, "computer-use") {
			t.Fatal("declined consent must not switch computer use on")
		}
	})

	t.Run("skipped counts as no", func(t *testing.T) {
		svc, id := newCapService(t)
		if _, err := readComputerUse(t, svc, id, allow("")); err == nil {
			t.Fatal("a skipped prompt must not enable computer use")
		}
		if on, _ := svc.sessions.Capabilities(id); contains(on, "computer-use") {
			t.Fatal("skipped consent must not switch computer use on")
		}
	})

	t.Run("nobody attached cannot approve", func(t *testing.T) {
		svc, id := newCapService(t)
		_, err := readComputerUse(t, svc, id, nil)
		if err == nil || !strings.Contains(err.Error(), "/computer-use") {
			t.Fatalf("want a hint to type the command, got %v", err)
		}
		if on, _ := svc.sessions.Capabilities(id); contains(on, "computer-use") {
			t.Fatal("no user means no consent")
		}
	})

	t.Run("allowed switches it on", func(t *testing.T) {
		svc, id := newCapService(t)
		text, err := readComputerUse(t, svc, id, allow("Allow"))
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(text, "now switched on") {
			t.Fatalf("expected a switched-on note, got: %.200s", text)
		}
		if on, _ := svc.sessions.Capabilities(id); !contains(on, "computer-use") {
			t.Fatalf("not persisted: %v", on)
		}
	})

	if len(asked) == 0 || !strings.Contains(asked[0], "computer use") {
		t.Fatalf("the user was never asked: %v", asked)
	}
}

func TestReadSkillListsCapabilitiesWithTheirState(t *testing.T) {
	svc, id := newCapService(t)
	_ = svc.sessions.AddCapability(id, "render")
	lc := &loadContext{sessionID: id, mode: "full-access"}
	tool := tools.NewReadSkillTool(svc.readSkillCapabilities(lc, tools.NewRegistry()))
	out, err := tool.Execute(t.Context(), []byte(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	text := textOf(out)
	for _, want := range []string{"render", "(built-in, on)", "computer-use", "(built-in, off)", "console"} {
		if !strings.Contains(text, want) {
			t.Errorf("listing missing %q:\n%s", want, text)
		}
	}
}

func TestReadSkillUnknownNameListsCapabilities(t *testing.T) {
	svc, id := newCapService(t)
	tool := tools.NewReadSkillTool(svc.readSkillCapabilities(&loadContext{sessionID: id, mode: "full-access"}, tools.NewRegistry()))
	_, err := tool.Execute(t.Context(), []byte(`{"name":"nope"}`))
	if err == nil || !strings.Contains(err.Error(), "render") {
		t.Fatalf("err = %v, want the built-ins listed", err)
	}
}

func TestBuiltinSkillsListFollowsTheMode(t *testing.T) {
	build := builtinSkillsSetup("full-access")
	for _, want := range []string{"browser-use", "console", "render", "computer-use", "/computer-use", "readSkill"} {
		if !strings.Contains(build, want) {
			t.Errorf("full-access list missing %q:\n%s", want, build)
		}
	}
	plan := builtinSkillsSetup("plan-mode")
	for _, want := range []string{"render", "browser-use", "console"} {
		if !strings.Contains(plan, want) {
			t.Errorf("plan mode keeps %q:\n%s", want, plan)
		}
	}
	for _, banned := range []string{"computer-use"} {
		if strings.Contains(plan, banned) {
			t.Errorf("plan mode must not list exec-tier %q:\n%s", banned, plan)
		}
	}
}

func TestToolsRefuseUntilTheirSkillIsRead(t *testing.T) {
	svc, id := newCapService(t)
	registry := tools.NewRegistry()
	svc.loadCapabilities(&loadContext{sessionID: id, cwd: t.TempDir(), mode: "full-access"}, registry)
	tool, err := registry.Get("render")
	if err != nil {
		t.Fatal(err)
	}
	args := []byte(`{"html":"<!doctype html><html></html>"}`)
	_, err = tool.Execute(t.Context(), args)
	if err == nil || !strings.Contains(err.Error(), "before reading the render skill") {
		t.Fatalf("first call must be refused with the reference, got %v", err)
	}
	if on, _ := svc.sessions.Capabilities(id); !contains(on, "render") {
		t.Fatalf("refusal must switch the skill on: %v", on)
	}
	if _, err = tool.Execute(t.Context(), args); err != nil && strings.Contains(err.Error(), "before reading") {
		t.Fatalf("retry must reach the real tool, got %v", err)
	}
}

func TestPopupListOnlyIncludesSlashCommands(t *testing.T) {
	got := CapabilityDescriptions()
	if len(got) != 1 || got[0][0] != "computer-use" {
		t.Fatalf("popup = %v, want only [computer-use]", got)
	}
}

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
