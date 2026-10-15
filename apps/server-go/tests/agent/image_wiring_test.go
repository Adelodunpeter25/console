// read_file wiring: the per-run options pick direct image parts, a vision-role
// describer, or the placeholder based on provider delivery and the configured
// vision role.
package tests

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/run"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/types"
	"github.com/Adelodunpeter25/console/apps/server-go/tests/helpers"
)

func TestReadFileImageWiring(t *testing.T) {
	if !run.DeliversToolResultImages("claude") {
		t.Fatal("claude must deliver tool-result images")
	}
	for _, p := range []string{"codex", "opencode", "antigravity"} {
		if run.DeliversToolResultImages(p) {
			t.Fatalf("%s must not deliver tool-result images", p)
		}
	}

	// Isolate settings: an absent file means no vision role.
	dir := t.TempDir()
	settingsPath := filepath.Join(dir, "settings.json")
	t.Setenv("CONSOLE_SETTINGS_PATH", settingsPath)
	svc := run.NewService(helpers.NewRunSessions(t))

	// Claude + capable model: direct image parts, no describer.
	claude := svc.ReadFileToolOptions(types.Model{ID: "sonnet", Provider: "claude", SupportsImages: true}, "claude", "sonnet")
	if !claude.CanSeeImages || claude.Describer != nil {
		t.Fatalf("claude capable: %+v", claude)
	}
	// Codex + capable model: the adapter redacts tool-result images, so no
	// direct part and (with no vision role) no describer.
	codex := svc.ReadFileToolOptions(types.Model{ID: "gpt-5", Provider: "codex", SupportsImages: true}, "codex", "gpt-5")
	if codex.CanSeeImages || codex.Describer != nil {
		t.Fatalf("codex capable: %+v", codex)
	}

	// Configure a vision role: a blind model now gets a describer bound to the
	// role, and labels for the handoff text.
	if err := os.WriteFile(settingsPath, []byte(`{"modelRoles":{"vision":"claude/haiku"}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	blind := svc.ReadFileToolOptions(types.Model{ID: "gpt-5", Provider: "codex"}, "codex", "gpt-5")
	if blind.Describer == nil || blind.VisionModel != "claude/haiku" || blind.RunModel != "gpt-5" {
		t.Fatalf("blind with vision role: %+v", blind)
	}
	// A capable but redacting model also describes once a role is set.
	capable := svc.ReadFileToolOptions(types.Model{ID: "gpt-5", Provider: "codex", SupportsImages: true}, "codex", "gpt-5")
	if capable.Describer == nil {
		t.Fatal("capable redacting model must describe when a vision role is set")
	}
}
