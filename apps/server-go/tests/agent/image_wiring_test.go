// read_file wiring: the per-run options pick direct image parts, a vision-role
// describer, or the placeholder based on provider delivery and the configured
// vision role.
package tests

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/tools"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/run"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/types"
	"github.com/Adelodunpeter25/console/apps/server-go/tests/helpers"
)

func TestReadFileImageWiring(t *testing.T) {
	for _, p := range []string{"claude", "codex", "antigravity"} {
		if !run.DeliversToolResultImages(p) {
			t.Fatalf("%s must deliver tool-result images", p)
		}
	}
	if run.DeliversToolResultImages("opencode") {
		t.Fatal("opencode must not deliver tool-result images")
	}

	// Isolate settings: an absent file means no vision role.
	dir := t.TempDir()
	settingsPath := filepath.Join(dir, "settings.json")
	t.Setenv("CONSOLE_SETTINGS_PATH", settingsPath)
	svc := run.NewService(helpers.NewRunSessions(t))

	// Delivering providers with a capable model: direct image parts, no
	// describer.
	for _, provider := range []string{"claude", "codex", "antigravity"} {
		opts := svc.ReadFileToolOptions(types.Model{ID: "m", Provider: provider, SupportsImages: true}, provider, "m", nil)
		if !opts.CanSeeImages || opts.Describer != nil {
			t.Fatalf("%s capable: %+v", provider, opts)
		}
	}
	// A non-delivering provider with a capable model: the adapter redacts, so
	// no direct part and (with no vision role) no describer.
	nonDelivering := svc.ReadFileToolOptions(types.Model{ID: "gpt", Provider: "opencode", SupportsImages: true}, "opencode", "gpt", nil)
	if nonDelivering.CanSeeImages || nonDelivering.Describer != nil {
		t.Fatalf("opencode capable: %+v", nonDelivering)
	}

	// Configure a vision role: a blind model now gets a describer bound to the
	// role, and labels for the handoff text.
	if err := os.WriteFile(settingsPath, []byte(`{"modelRoles":{"vision":"claude/haiku"}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	blind := svc.ReadFileToolOptions(types.Model{ID: "gpt-5", Provider: "codex"}, "codex", "gpt-5", nil)
	if blind.Describer == nil || blind.VisionModel != "claude/haiku" || blind.RunModel != "gpt-5" {
		t.Fatalf("blind with vision role: %+v", blind)
	}
	// A capable delivering model needs no describer even with a role set.
	capable := svc.ReadFileToolOptions(types.Model{ID: "gpt-5", Provider: "codex", SupportsImages: true}, "codex", "gpt-5", nil)
	if capable.Describer != nil || !capable.CanSeeImages {
		t.Fatalf("capable delivering model must not describe: %+v", capable)
	}
}

func TestVisionDescribeDisabled(t *testing.T) {
	dir := t.TempDir()
	settingsPath := filepath.Join(dir, "settings.json")
	body := `{"modelRoles":{"vision":"claude/haiku"},"vision":{"describeImages":false}}`
	if err := os.WriteFile(settingsPath, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CONSOLE_SETTINGS_PATH", settingsPath)
	svc := run.NewService(helpers.NewRunSessions(t))

	opts := svc.ReadFileToolOptions(types.Model{ID: "gpt-5", Provider: "codex"}, "codex", "gpt-5", nil)
	if opts.Describer != nil {
		t.Fatalf("kill switch must disable the describer: %+v", opts)
	}

	// With the describer disabled, read_file falls back to the placeholder.
	img := filepath.Join(dir, "shot.png")
	if err := os.WriteFile(img, []byte{0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a, 'x'}, 0o644); err != nil {
		t.Fatal(err)
	}
	out, err := tools.NewReadFileTool(opts).Execute(context.Background(), json.RawMessage(`{"path":"`+img+`"}`))
	if err != nil {
		t.Fatal(err)
	}
	if text := out.([]map[string]any)[0]["text"].(string); !strings.Contains(text, "Cannot view") {
		t.Fatalf("kill switch must fall back to the placeholder: %q", text)
	}
}
