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
