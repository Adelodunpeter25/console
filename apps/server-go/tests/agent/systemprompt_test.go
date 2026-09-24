// Coverage for system-prompt discovery + assembly.
package tests

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/systemprompt"
)

func TestSystemPromptDiscoveryAndBuild(t *testing.T) {
	dir := t.TempDir()
	write := func(path, content string) {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	// AGENTS.md at project root.
	write(filepath.Join(dir, "AGENTS.md"), "Follow the style guide.")
	// A skill and an always-apply rule under .agents/.
	write(filepath.Join(dir, ".agents", "skills", "deploy.md"),
		"---\ndescription: Deploy the app\n---\nRun deploy.sh")
	write(filepath.Join(dir, ".agents", "rules", "style.md"),
		"---\nalwaysApply: true\ndescription: Style rules\n---\nUse tabs.")
	// A user-defined slash command.
	write(filepath.Join(dir, ".agents", "commands", "release.md"),
		"---\ndescription: Cut a release\n---\nRun the release checklist.")

	result := systemprompt.BuildSystemPrompt(systemprompt.BuildOptions{
		Cwd:               dir,
		Home:              t.TempDir(), // isolate from the real user's home config
		SkipWorkspaceTree: true,
	})

	prompt := result.SystemPrompt
	for _, want := range []string{
		"Follow the style guide.", // repo context file
		"deploy: Deploy the app",  // skill inventory
		"Use tabs.",               // always-apply rule content
		"/release: Cut a release", // slash command
		"/init:",                  // built-in init command
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("prompt missing %q:\n%s", want, prompt)
		}
	}

	if len(result.Context.Skills) != 1 || result.Context.Skills[0].Name != "deploy" {
		t.Fatalf("skills: %+v", result.Context.Skills)
	}
	if len(result.Context.Rules) != 1 || !result.Context.Rules[0].AlwaysApply {
		t.Fatalf("rules: %+v", result.Context.Rules)
	}
}

func TestSystemPromptApprovalModeInstructions(t *testing.T) {
	dir := t.TempDir()
	result := systemprompt.BuildSystemPrompt(systemprompt.BuildOptions{
		Cwd: dir, Home: t.TempDir(), SkipWorkspaceTree: true,
		ApprovalMode: systemprompt.PlanMode,
	})
	if !strings.Contains(result.SystemPrompt, "Plan mode is ACTIVE") {
		t.Fatalf("expected plan-mode instructions:\n%s", result.SystemPrompt)
	}
}

func TestSystemPromptStableSplitsFromSetup(t *testing.T) {
	build := func(dir string) systemprompt.Result {
		if err := os.WriteFile(filepath.Join(dir, "AGENTS.md"), []byte("Follow the style guide."), 0o644); err != nil {
			t.Fatal(err)
		}
		return systemprompt.BuildSystemPrompt(systemprompt.BuildOptions{
			Cwd: dir, Home: t.TempDir(), SkipWorkspaceTree: true,
			Model: "m", ApprovalMode: systemprompt.FullAccess,
		})
	}
	dirA, dirB := t.TempDir(), t.TempDir()
	a, b := build(dirA), build(dirB)

	// Stable part: no per-session values, identical across cwds.
	for _, volatile := range []string{dirA, "Today is", "Git branch:", "Follow the style guide."} {
		if strings.Contains(a.StableSystem, volatile) {
			t.Fatalf("stable system contains %q:\n%s", volatile, a.StableSystem)
		}
	}
	if a.StableSystem != b.StableSystem {
		t.Fatalf("stable system differs across sessions:\n%s\n---\n%s", a.StableSystem, b.StableSystem)
	}
	for _, want := range []string{"You are a coding agent.", "Bypass Permissions", "# Tool inventory", "<critical>"} {
		if !strings.Contains(a.StableSystem, want) {
			t.Fatalf("stable system missing %q", want)
		}
	}
	// Setup: per-session context.
	for _, want := range []string{dirA, "Today is", "Follow the style guide.", "/init:"} {
		if !strings.Contains(a.Setup, want) {
			t.Fatalf("setup missing %q:\n%s", want, a.Setup)
		}
	}
	// The joined prompt keeps everything for callers that don't split.
	if !strings.Contains(a.SystemPrompt, a.StableSystem) || !strings.Contains(a.SystemPrompt, a.Setup) {
		t.Fatal("SystemPrompt must join stable and setup")
	}
}
