// The console.mcp.* methods against a real MCP manager backed by temp-dir
// stores. Connect-dependent paths are not exercised here (no live server);
// the argument, gating and config-lifecycle behaviour is.
package console

import (
	"context"
	"testing"
	"time"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/services/mcp"
)

func newMCPRuntime(t *testing.T, ceiling string) *Runtime {
	t.Helper()
	dir := t.TempDir()
	manager := mcp.NewManager(mcp.NewConfigStore(dir), mcp.NewCredentialStore(dir), nil)
	runtime, err := NewRuntime(nil, nil, nil, nil, manager, nil, nil, nil, nil, QueueControl{}, "", "")
	if err != nil {
		t.Fatal(err)
	}
	runtime.SetCeiling(ceiling)
	return runtime
}

const addHTTP = `console.mcp.add({id: "notes", transport: "http", url: "https://example.com/mcp", label: "Notes"})`

func TestMcpAddIsGatedToFullAccess(t *testing.T) {
	for _, ceiling := range []string{"", "plan-mode", "always-ask", "accept-edits"} {
		runtime := newMCPRuntime(t, ceiling)
		if _, err := runtime.Eval(context.Background(), addHTTP, time.Second); err == nil {
			t.Errorf("ceiling %q: expected mcp.add to be refused", ceiling)
		}
	}
}

func TestMcpAddListUpdateRemoveLifecycle(t *testing.T) {
	runtime := newMCPRuntime(t, "full-access")
	added := evalJSON(t, runtime, addHTTP).(map[string]any)
	if added["id"] != "notes" || added["enabled"] != true {
		t.Fatalf("added = %#v", added)
	}
	if auth, _ := added["auth"].(map[string]any); auth != nil {
		t.Fatalf("no auth requested but got %#v", added["auth"])
	}

	list := evalJSON(t, runtime, `console.mcp.list()`).([]any)
	if len(list) != 1 {
		t.Fatalf("list = %#v", list)
	}
	row := list[0].(map[string]any)
	if row["id"] != "notes" || row["status"] != mcp.StatusDisconnected {
		t.Fatalf("list row = %#v", row)
	}

	// Update changes only the keys given and keeps the rest.
	updated := evalJSON(t, runtime, `console.mcp.update({id: "notes", label: "Old notes", enabled: false})`).(map[string]any)
	if updated["label"] != "Old notes" {
		t.Fatalf("label = %v", updated["label"])
	}
	if updated["enabled"] != false {
		t.Fatalf("enabled = %v", updated["enabled"])
	}
	if updated["url"] != "https://example.com/mcp" {
		t.Fatalf("url = %v; update dropped it", updated["url"])
	}

	// Auth type changes to none, static or oauth2; the token never passes through here.
	oauth := evalJSON(t, runtime, `console.mcp.update({id: "notes", auth: "oauth2"})`).(map[string]any)
	auth, _ := oauth["auth"].(map[string]any)
	if auth == nil || auth["type"] != mcp.AuthOAuth2 || auth["tokenRef"] != "notes" {
		t.Fatalf("auth = %#v", oauth["auth"])
	}

	removed := evalJSON(t, runtime, `console.mcp.remove({id: "notes"})`).(map[string]any)
	if removed["removed"] != true {
		t.Fatalf("removed = %#v", removed)
	}
	// An empty list renders as "(no output)" in the kernel (same as an empty
	// sessions.list), so check the store for the deletion.
	if configs, err := runtime.mcp.Config.List(); err != nil || len(configs) != 0 {
		t.Fatalf("configs after remove = %#v, %v", configs, err)
	}
}

func TestMcpAddValidatesAndRefusesDuplicates(t *testing.T) {
	runtime := newMCPRuntime(t, "full-access")
	cases := map[string]string{
		"bad transport":  `console.mcp.add({id: "x", transport: "carrier-pigeon"})`,
		"http needs url": `console.mcp.add({id: "x", transport: "http"})`,
		"bad id":         `console.mcp.add({id: "not ok!", transport: "http", url: "https://example.com"})`,
		"bad auth":       `console.mcp.add({id: "x", transport: "http", url: "https://example.com", auth: "bearer"})`,
	}
	for name, code := range cases {
		if _, err := runtime.Eval(context.Background(), code, time.Second); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
	if _, err := runtime.Eval(context.Background(), addHTTP, time.Second); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.Eval(context.Background(), addHTTP, time.Second); err == nil {
		t.Fatal("duplicate add: expected an error naming update or remove")
	}
}

func TestMcpUnknownServerRefusals(t *testing.T) {
	runtime := newMCPRuntime(t, "full-access")
	for _, code := range []string{
		`console.mcp.tools({id: "ghost"})`,
		`console.mcp.update({id: "ghost", label: "x"})`,
		`console.mcp.remove({id: "ghost"})`,
		`console.mcp.disconnect({id: "ghost"})`,
		`console.mcp.reset_auth({id: "ghost"})`,
	} {
		if _, err := runtime.Eval(context.Background(), code, time.Second); err == nil {
			t.Errorf("%s: expected not-found error", code)
		}
	}
	// Connecting starts a background attempt against an unreachable URL and
	// must come back as an error, not hang past the kernel timeout.
	if _, err := runtime.Eval(context.Background(), `console.mcp.connect({id: "ghost"})`, 3*time.Second); err == nil {
		t.Error("connect of unknown server: expected an error")
	}
}

func TestMcpCallWithoutServerFails(t *testing.T) {
	runtime := newMCPRuntime(t, "plan-mode")
	if _, err := runtime.Eval(context.Background(), `console.mcp.call({id: "ghost", tool: "ping"})`, 3*time.Second); err == nil {
		t.Fatal("expected an error")
	}
}
