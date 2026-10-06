// Cua tool surface: name mapping, tiering from Cua's risk metadata, and the
// conversion of driver results into harness content parts. Tiering matters
// most here: Cua's classes do not follow the name prefixes Console's heuristic
// assumes, so a name-only classifier would wave through kill_app.
package tests

import (
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/tools"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/services/cua"
)

func TestToolNameIsPrefixedAndSanitized(t *testing.T) {
	if got := cua.ToolName("get_window_state"); got != "cua__get_window_state" {
		t.Fatalf("got %q", got)
	}
	if got := cua.ToolName("weird name.v2"); strings.ContainsAny(got, " .") {
		t.Fatalf("unsafe characters survived: %q", got)
	}
	long := cua.ToolName(strings.Repeat("a", 200))
	if len(long) > 64 {
		t.Fatalf("name too long: %d", len(long))
	}
}

// TestTierFollowsCuaRiskNotNamePrefixes is the regression guard. kill_app has
// no exec-ish prefix, so a name heuristic would call it a write; Cua classes it
// R3 because it terminates a process.
func TestTierFollowsCuaRiskNotNamePrefixes(t *testing.T) {
	yes := true
	cases := []struct {
		name string
		def  cua.ToolDef
		want tools.ToolTier
	}{
		{"r0 is a read", cua.ToolDef{Risk: cua.Risk{Class: "r0"}}, tools.TierRead},
		{"r1 is a write", cua.ToolDef{Risk: cua.Risk{Class: "r1"}}, tools.TierWrite},
		{"r2 is exec", cua.ToolDef{Risk: cua.Risk{Class: "r2"}}, tools.TierExec},
		{"r3 is exec", cua.ToolDef{Risk: cua.Risk{Class: "r3"}}, tools.TierExec},
		{"r4 is exec", cua.ToolDef{Risk: cua.Risk{Class: "r4"}}, tools.TierExec},
		{"unclassified fails closed", cua.ToolDef{Risk: cua.Risk{Class: "unclassified"}}, tools.TierExec},
		{"missing classification fails closed", cua.ToolDef{}, tools.TierExec},
		{"class case is ignored", cua.ToolDef{Risk: cua.Risk{Class: "R0"}}, tools.TierRead},
		// readOnlyHint is the driver's own statement and outranks the class:
		// get_window_state is r3 but observing a window is not an action.
		{"readOnlyHint wins", cua.ToolDef{
			Risk:        cua.Risk{Class: "r3"},
			Annotations: cua.Annotations{ReadOnlyHint: &yes},
		}, tools.TierRead},
	}
	for _, c := range cases {
		if got := cua.Tier(c.def); got != c.want {
			t.Errorf("%s: got %s want %s", c.name, got, c.want)
		}
	}
}

// TestObservationToolsAreReadsDespiteHighRisk guards the practical consequence
// of the mapping: if a screenshot were exec, always-ask would prompt on every
// single observation and the feature would be unusable.
func TestObservationToolsAreReadsDespiteHighRisk(t *testing.T) {
	yes := true
	for _, name := range []string{"get_window_state", "get_desktop_state", "list_apps"} {
		def := cua.ToolDef{Name: name, Risk: cua.Risk{Class: "r3"}}
		def.Annotations.ReadOnlyHint = &yes
		if got := cua.Tier(def); got != tools.TierRead {
			t.Errorf("%s tier = %s, want read", name, got)
		}
	}
}

func TestResultPartsCarryImagesAsNativeParts(t *testing.T) {
	png := []byte{0x89, 'P', 'N', 'G', 1, 2, 3}
	result := &cua.ToolResult{
		Content: []cua.ContentPart{
			{Type: "text", Text: "window 42"},
			{Type: "image", MIMEType: "image/png", Data: png},
		},
	}
	parts := toolParts(result)

	// Text is always first so the model reads it before the image.
	if parts[0]["type"] != "text" || !strings.Contains(parts[0]["text"].(string), "window 42") {
		t.Fatalf("text part wrong: %v", parts[0])
	}
	var img map[string]any
	for _, p := range parts {
		if p["type"] == "image" {
			img = p
		}
	}
	if img == nil {
		t.Fatalf("no image part in %v", parts)
	}
	if img["mimeType"] != "image/png" {
		t.Errorf("mimeType = %v", img["mimeType"])
	}
	if img["data"] != base64.StdEncoding.EncodeToString(png) {
		t.Errorf("image data was not base64 encoded: %v", img["data"])
	}
}

func TestResultPartsFallBackToStructuredContent(t *testing.T) {
	// When a result has neither text nor an image, the driver's typed view is
	// the only description of what happened and must not be dropped.
	result := &cua.ToolResult{StructuredContent: json.RawMessage(`{"window_id":42}`)}
	parts := toolParts(result)
	if len(parts) != 1 || parts[0]["type"] != "text" {
		t.Fatalf("got %v", parts)
	}
	if !strings.Contains(parts[0]["text"].(string), "window_id") {
		t.Fatalf("structured content lost: %q", parts[0]["text"])
	}
}

func TestResultPartsEmptyIsStillAnswery(t *testing.T) {
	parts := toolParts(&cua.ToolResult{})
	if len(parts) != 1 || parts[0]["text"] != "(no output)" {
		t.Fatalf("got %v", parts)
	}
}

func TestResultPartsAudioStaysATextNote(t *testing.T) {
	parts := toolParts(&cua.ToolResult{Content: []cua.ContentPart{
		{Type: "audio", MIMEType: "audio/wav", Data: []byte{1}},
	}})
	for _, p := range parts {
		if p["type"] != "text" {
			t.Fatalf("unexpected part type %v", p["type"])
		}
	}
	if !strings.Contains(parts[0]["text"].(string), "audio content omitted") {
		t.Errorf("audio note missing: %q", parts[0]["text"])
	}
}

// TestManagerAbsentDriverIsUnavailableNotBroken proves the feature degrades:
// with no library the manager reports unavailable rather than panicking.
func TestManagerAbsentDriverIsUnavailableNotBroken(t *testing.T) {
	t.Setenv("CUA_DRIVER_LIB_PATH", t.TempDir()+"/absent")
	m := cua.NewManager()
	if m.Available() {
		t.Skip("a real Cua Driver library is installed and loaded first")
	}
	if _, err := m.Driver(); err == nil {
		t.Fatal("Driver() succeeded with no library")
	}
	// Stop and Resume must be safe before the driver exists.
	m.Stop()
	m.Resume()
	if m.Stopped() {
		t.Error("Resume must clear the stop flag")
	}
	// A second open must not re-pay the dlopen attempt.
	if _, err := m.Driver(); err == nil {
		t.Fatal("second Driver() unexpectedly succeeded")
	}
}

// TestManagerStatusWithoutLibrary proves Status reports the problem instead of
// returning a misleading "available".
func TestManagerStatusWithoutLibrary(t *testing.T) {
	t.Setenv("CUA_DRIVER_LIB_PATH", t.TempDir()+"/absent")
	status := cua.NewManager().Status()
	if status.PermissionMode != "unrestricted" {
		t.Errorf("permissionMode = %q, want unrestricted", status.PermissionMode)
	}
	if status.Error == "" {
		t.Error("Status must explain why the library is unavailable")
	}
	if status.Available {
		t.Error("Status must not claim availability without a library")
	}
}

// TestLoaderRefusesWithoutDriver keeps /computer-use honest: with no runtime
// it must report the reason rather than silently loading an empty group.
func TestLoaderRefusesWithoutDriver(t *testing.T) {
	t.Setenv("CUA_DRIVER_LIB_PATH", t.TempDir()+"/absent")
	registry := tools.NewRegistry()
	loader := cua.NewLoader(cua.NewManager())
	if loader.Loaded() {
		t.Error("a fresh loader must not report itself loaded")
	}
	_, err := loader.Load(registry)
	if err == nil {
		t.Fatal("Load succeeded with no driver")
	}
	if loader.Loaded() {
		t.Error("a failed load must not mark the group loaded")
	}
}

// toolParts converts a result the way the tool path does, so the test covers
// exactly what the model receives.
func toolParts(result *cua.ToolResult) []map[string]any {
	return cua.ToolResultParts(result)
}
