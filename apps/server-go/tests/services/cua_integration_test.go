// Cua Driver integration against a real library. Skipped unless
// CUA_DRIVER_LIB_PATH points at a real libcua_driver_sdk, so CI is unaffected.
//
// These are the assertions that only a real driver can satisfy: the ABI check,
// the tool inventory shape, a live tool call, image decoding, and the stop
// contract. Everything else in the package is covered by unit tests.
package tests

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/tools"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/services/cua"
)

// requireDriver skips unless a real library is configured.
func requireDriver(t *testing.T) *cua.Driver {
	t.Helper()
	path := os.Getenv("CUA_DRIVER_LIB_PATH")
	if path == "" {
		t.Skip("set CUA_DRIVER_LIB_PATH to a libcua_driver_sdk to run driver integration tests")
	}
	if !strings.HasSuffix(path, ".dylib") && !strings.HasSuffix(path, ".so") {
		t.Skipf("CUA_DRIVER_LIB_PATH does not look like a driver library: %q", path)
	}
	driver, err := cua.Open(cua.Options{})
	if err != nil {
		t.Fatalf("open driver: %v", err)
	}
	t.Cleanup(func() { _ = driver.Shutdown() })
	return driver
}

// driverList reads the advertised inventory, the same call /computer-use makes
// when it builds the tool group.
func driverList(t *testing.T) ([]cua.ToolDef, error) {
	t.Helper()
	driver, err := cua.Open(cua.Options{})
	if err != nil {
		t.Fatalf("open driver: %v", err)
	}
	t.Cleanup(func() { _ = driver.Shutdown() })
	return driver.ListTools()
}

func TestDriverABIMatches(t *testing.T) {
	requireDriver(t)
	major, minor, _, err := cua.ABIVersion()
	if err != nil {
		t.Fatalf("abi version: %v", err)
	}
	if major != 1 || minor != 1 {
		t.Fatalf("ABI %d.%d, want 1.1", major, minor)
	}
}

// TestDriverAdvertisesClassifiedTools is the assertion that matters for
// tiering: every advertised tool carries a risk classification, so Console never
// has to fall back to a name heuristic.
func TestDriverAdvertisesClassifiedTools(t *testing.T) {
	requireDriver(t)
	list, err := driverList(t)
	if err != nil {
		t.Fatalf("list tools: %v", err)
	}
	if len(list) < 10 {
		t.Fatalf("only %d tools advertised, expected the full inventory", len(list))
	}

	unclassified := 0
	for _, tool := range list {
		switch tool.RiskClass() {
		case "r0", "r1", "r2", "r3", "r4":
		case "unclassified", "":
			unclassified++
			t.Logf("tool %s has no reviewed risk classification", tool.Name)
		default:
			t.Errorf("tool %s has an unknown risk class %q", tool.Name, tool.RiskClass())
		}
		// A real schema is what lets the model pass correct arguments.
		if len(tool.InputSchema) == 0 {
			t.Errorf("tool %s advertises no input schema", tool.Name)
		}
	}
	if unclassified > 0 {
		t.Errorf("%d tools are unclassified; tiering would be guessing for those", unclassified)
	}
}

// TestDriverTiersDangerousToolsCorrectly pins the two classifications a name
// heuristic gets wrong or gets right only by luck.
func TestDriverTiersDangerousToolsCorrectly(t *testing.T) {
	requireDriver(t)
	list, err := driverList(t)
	if err != nil {
		t.Fatalf("list tools: %v", err)
	}
	byName := map[string]cua.ToolDef{}
	for _, tool := range list {
		byName[tool.Name] = tool
	}

	// kill_app terminates a process and has no exec-ish name prefix.
	if def, ok := byName["kill_app"]; ok {
		if got := cua.Tier(def); got != tools.TierExec {
			t.Errorf("kill_app tier = %s, want exec (risk %s)", got, def.RiskClass())
		}
	}
	// The input loop must be able to act.
	for _, name := range []string{"click", "type_text", "press_key"} {
		if def, ok := byName[name]; ok {
			if got := cua.Tier(def); got != tools.TierWrite {
				t.Errorf("%s tier = %s, want write (risk %s)", name, got, def.RiskClass())
			}
		}
	}
	// Observation must be a read, or the model prompts on every screenshot.
	for _, name := range []string{"get_window_state", "list_apps"} {
		if def, ok := byName[name]; ok {
			if got := cua.Tier(def); got != tools.TierRead {
				t.Errorf("%s tier = %s, want read (risk %s, readOnly=%v)",
					name, got, def.RiskClass(), def.ReadOnly())
			}
		}
	}
}

// TestDriverCallReturnsStructuredResult proves the async completion path works
// end to end, which no unit test can cover.
func TestDriverCallReturnsStructuredResult(t *testing.T) {
	driver := requireDriver(t)
	result, err := driver.Call(context.Background(), "get_screen_size", map[string]any{}, nil)
	if err != nil {
		t.Fatalf("get_screen_size: %v", err)
	}
	if result.IsError {
		t.Fatalf("get_screen_size reported an error: %+v", result.Content)
	}
	if len(result.Content) == 0 {
		t.Fatal("get_screen_state returned no content")
	}
	if result.Content[0].Type != "text" {
		t.Fatalf("first part is %q, want text", result.Content[0].Type)
	}
	if !strings.Contains(result.Content[0].Text, "display") {
		t.Errorf("unexpected text: %q", result.Content[0].Text)
	}
}

// TestDriverDecodesImageContent is the screenshot path. It needs a real
// permitted process, so it asserts only what holds either way: an image part
// arrives as raw bytes, never as a base64 string, and the harness shape carries
// a mime type the Claude converter accepts.
func TestDriverDecodesImageContent(t *testing.T) {
	driver := requireDriver(t)
	result, err := driver.Call(context.Background(), "get_desktop_state", map[string]any{}, nil)
	if err != nil {
		t.Skipf("capture refused in this environment: %v", err)
	}

	var image *cua.ContentPart
	for i := range result.Content {
		if result.Content[i].Type == "image" {
			image = &result.Content[i]
		}
	}
	if image == nil {
		// Without Screen Recording the driver returns a refusal instead of an
		// image; that is a legitimate outcome, not a binding failure.
		t.Skip("no image content: capture is not permitted for this process")
	}
	if len(image.Data) == 0 {
		t.Fatal("image part carries no bytes")
	}
	if !strings.Contains(string(image.Data[:8]), "PNG") {
		t.Errorf("image data is not a PNG: %q", image.Data[:8])
	}

	parts := cua.ToolResultParts(result)
	var harnessImage map[string]any
	for _, p := range parts {
		if p["type"] == "image" {
			harnessImage = p
		}
	}
	if harnessImage == nil {
		t.Fatal("no image part in the harness conversion")
	}
	if harnessImage["mimeType"] != "image/png" {
		t.Errorf("mimeType = %v, want image/png", harnessImage["mimeType"])
	}
	if _, ok := harnessImage["data"].(string); !ok {
		t.Errorf("harness image data must be a base64 string, got %T", harnessImage["data"])
	}
}

// TestDriverStopRefusesThenResumes covers the kill switch against the real
// runtime: a stopped driver must refuse new work with a *known* outcome, and
// must come back after Resume.
func TestDriverStopRefusesThenResumes(t *testing.T) {
	driver := requireDriver(t)
	ctx := context.Background()

	driver.Stop()
	_, err := driver.Call(ctx, "get_screen_size", map[string]any{}, nil)
	if err == nil {
		t.Fatal("a call succeeded after Stop")
	}
	var cancelled *cua.CancelledError
	if !errorsAsCua(err, &cancelled) {
		t.Fatalf("got %v, want a CancelledError", err)
	}
	if cancelled.Unknown {
		t.Error("a refused start has a known outcome and must not report unknown")
	}

	driver.Resume()
	if _, err := driver.Call(ctx, "get_screen_size", map[string]any{}, nil); err != nil {
		t.Fatalf("call after Resume failed: %v", err)
	}
}

func errorsAsCua(err error, target **cua.CancelledError) bool {
	for err != nil {
		if c, ok := err.(*cua.CancelledError); ok {
			*target = c
			return true
		}
		u, ok := err.(interface{ Unwrap() error })
		if !ok {
			return false
		}
		err = u.Unwrap()
	}
	return false
}
