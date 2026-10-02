// loadTools group descriptions, OnLoad tracking, and LoadGroup restore.
package tests

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/tools"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/services/mcp"
)

func TestLoadToolsDescribesConnectedGroupTools(t *testing.T) {
	ts := startHTTPMCP(t, nil)
	m := newManager(t)
	if _, err := m.Config.Save(mcp.ServerConfig{ID: "echo", Label: "Echo", Transport: mcp.TransportHTTP, URL: ts.URL, Enabled: true}); err != nil {
		t.Fatal(err)
	}
	// Unconnected: label only.
	lt := mcp.NewLoadToolsTool(m, tools.NewRegistry())
	if d := lt.Description(); !strings.Contains(d, "- mcp:echo: Echo") || strings.Contains(d, "tools:") {
		t.Fatalf("unconnected description: %s", d)
	}
	if strings.Contains(lt.Description(), "atlassian") {
		t.Fatal("description must not mention a specific server")
	}
	// Connected: tool names previewed.
	if err := m.Ensure(ctx5(t), "echo"); err != nil {
		t.Fatal(err)
	}
	lt = mcp.NewLoadToolsTool(m, tools.NewRegistry())
	if d := lt.Description(); !strings.Contains(d, "(2 tools: ") || !strings.Contains(d, "get_echo") || !strings.Contains(d, "delete_all") {
		t.Fatalf("connected description: %s", d)
	}
}

func TestLoadToolsOnLoadAndLoadGroup(t *testing.T) {
	ts := startHTTPMCP(t, nil)
	m := newManager(t)
	if _, err := m.Config.Save(mcp.ServerConfig{ID: "echo", Label: "Echo", Transport: mcp.TransportHTTP, URL: ts.URL, Enabled: true}); err != nil {
		t.Fatal(err)
	}
	var loaded []string
	reg := tools.NewRegistry()
	lt := mcp.NewLoadToolsTool(m, reg, mcp.LoadToolsOptions{OnLoad: func(g string) { loaded = append(loaded, g) }})
	if _, err := lt.Execute(ctx5(t), json.RawMessage(`{"group":"mcp:nope"}`)); err == nil {
		t.Fatal("unknown group should error")
	}
	if len(loaded) != 0 {
		t.Fatalf("failed load must not call OnLoad: %v", loaded)
	}
	if _, err := lt.Execute(ctx5(t), json.RawMessage(`{"group":"mcp:echo"}`)); err != nil {
		t.Fatal(err)
	}
	if len(loaded) != 1 || loaded[0] != mcp.GroupID("echo") {
		t.Fatalf("OnLoad: %v", loaded)
	}

	// Restore into a fresh registry, as the run service does per message.
	fresh := tools.NewRegistry()
	cfg, added, err := mcp.LoadGroup(ctx5(t), m, fresh, "mcp:echo")
	if err != nil || cfg.ID != "echo" || len(added) != 2 {
		t.Fatalf("LoadGroup: %v %v %d", err, cfg.ID, len(added))
	}
	if _, err := fresh.Get("mcp__echo__get_echo"); err != nil {
		t.Fatal(err)
	}
	// Disabled servers are not restorable.
	if _, err := m.Config.Save(mcp.ServerConfig{ID: "echo", Label: "Echo", Transport: mcp.TransportHTTP, URL: ts.URL, Enabled: false}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := mcp.LoadGroup(ctx5(t), m, tools.NewRegistry(), "mcp:echo"); err == nil {
		t.Fatal("disabled server should not load")
	}
}
