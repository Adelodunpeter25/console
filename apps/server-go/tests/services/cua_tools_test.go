// The two computer tools: identity, tiers and session binding. The 56 driver
// methods reach the model as callable JavaScript through these two schemas,
// so what matters here is that exactly two tools load, that they are bound to
// one session, and that running code needs no library beyond construction.
package tests

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/tools"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/services/cua"
)

func TestComputerToolsHaveStableIdentity(t *testing.T) {
	manager := cua.NewManager()
	computer := cua.NewComputerTool(manager, "session-a")
	reset := cua.NewComputerResetTool(manager, "session-a")

	if computer.Name() != "computer" {
		t.Errorf("got %q", computer.Name())
	}
	if reset.Name() != "computer_reset" {
		t.Errorf("got %q", reset.Name())
	}
	// Running code can do anything the driver allows, so it is exec-tier;
	// clearing variables touches nothing on the machine, so it is a read.
	if computer.Tier() != tools.TierExec {
		t.Errorf("computer tier = %s, want exec", computer.Tier())
	}
	if reset.Tier() != tools.TierRead {
		t.Errorf("computer_reset tier = %s, want read", reset.Tier())
	}
	if strings.TrimSpace(computer.Description()) == "" || strings.TrimSpace(reset.Description()) == "" {
		t.Error("tools must describe themselves")
	}
}

func TestComputerToolSchemaTakesCode(t *testing.T) {
	computer := cua.NewComputerTool(cua.NewManager(), "session-a")
	schema, err := tools.SchemaMap(computer)
	if err != nil {
		t.Fatalf("schema: %v", err)
	}
	props, _ := schema["properties"].(map[string]any)
	if props == nil {
		t.Fatalf("schema has no properties: %v", schema)
	}
	if _, ok := props["code"]; !ok {
		t.Errorf("schema must take code, has %v", keysOf(props))
	}
}

// TestLoaderRefusesWithoutDriver keeps /computer-use honest: with no runtime
// it must report the reason rather than silently loading an empty group.
func TestLoaderRefusesWithoutDriver(t *testing.T) {
	t.Setenv("CUA_DRIVER_LIB_PATH", t.TempDir()+"/absent")
	// Without a library the load must fail honestly rather than install an
	// empty group the model would call into the void.
	registry := tools.NewRegistry()
	loader := cua.NewLoader(cua.NewManager(), "session-a")
	if loader.Loaded() {
		t.Error("a fresh loader must not report itself loaded")
	}
	if _, err := loader.Load(registry); err == nil {
		t.Fatal("Load succeeded with no driver")
	}
	if loader.Loaded() {
		t.Error("a failed load must not mark the group loaded")
	}
	if len(registry.Names()) != 0 {
		t.Errorf("registry gained %v from a failed load", registry.Names())
	}
}

// TestComputerResetClearsSessionState proves reset works without a driver:
// dropping a kernel touches no native code, so it must succeed even when the
// library is absent.
func TestComputerResetClearsSessionState(t *testing.T) {
	t.Setenv("CUA_DRIVER_LIB_PATH", t.TempDir()+"/absent")
	reset := cua.NewComputerResetTool(cua.NewManager(), "session-a")
	out, err := reset.Execute(context.Background(), json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("reset: %v", err)
	}
	parts, ok := out.([]map[string]any)
	if !ok || len(parts) != 1 || parts[0]["type"] != "text" {
		t.Fatalf("reset returned %v", out)
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
	if got := m.SessionRuntimeCount(); got != 0 {
		t.Errorf("no kernels should exist, got %d", got)
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

func keysOf(m map[string]any) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

// TestComputerExecuteWithoutDriverIsInformationNotAbort proves a missing
// library reads as a message to the user, not a dead turn: the harness
// surfaces ToolError to the model, which reports unavailability.
func TestComputerExecuteWithoutDriverIsInformationNotAbort(t *testing.T) {
	t.Setenv("CUA_DRIVER_LIB_PATH", t.TempDir()+"/absent")
	tool := cua.NewComputerTool(cua.NewManager(), "session-a")
	_, err := tool.Execute(context.Background(), json.RawMessage(`{"code": "1+1"}`))
	if err == nil {
		t.Fatal("expected unavailability, got success")
	}
	if !strings.Contains(err.Error(), "unavailable") {
		t.Fatalf("got %v, want an unavailability message", err)
	}
}
