// End-to-end manager coverage against in-process MCP servers: static token
// auth, tool listing/calling, lazy loadTools, reconnect and status.
package tests

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/tools"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/services/mcp"
)

func newManager(t *testing.T) *mcp.Manager {
	t.Helper()
	dir := t.TempDir()
	m := mcp.NewManager(mcp.NewConfigStore(dir), mcp.NewCredentialStore(dir), nil)
	t.Cleanup(m.CloseAll)
	return m
}

func ctx5(t *testing.T) context.Context {
	c, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	t.Cleanup(cancel)
	return c
}

func requireBearer(want string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("Authorization") != want {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func saveStatic(t *testing.T, m *mcp.Manager, url, token string) mcp.ServerConfig {
	t.Helper()
	cfg, err := m.Config.Save(mcp.ServerConfig{
		ID: "echo", Label: "Echo", Transport: mcp.TransportHTTP, URL: url, Enabled: true,
		Auth: &mcp.AuthConfig{Type: mcp.AuthStatic, TokenRef: "echo"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := m.Credentials.Set("echo", mcp.Credential{Kind: mcp.CredentialStatic, Header: token}); err != nil {
		t.Fatal(err)
	}
	return cfg
}

func TestManagerStaticAuthListAndCall(t *testing.T) {
	ts := startHTTPMCP(t, requireBearer("Bearer s3cret"))
	m := newManager(t)
	saveStatic(t, m, ts.URL, "Bearer s3cret")

	if err := m.Ensure(ctx5(t), "echo"); err != nil {
		t.Fatalf("ensure: %v", err)
	}
	remote := m.Tools("echo")
	if len(remote) != 2 {
		t.Fatalf("want 2 tools, got %+v", remote)
	}
	res, err := m.Call(ctx5(t), "echo", "get_echo", map[string]any{"text": "hi"})
	if err != nil || res.IsError {
		t.Fatalf("call: %v %+v", err, res)
	}
	st, _ := m.Status()
	if len(st) != 1 || st[0].Status != mcp.StatusConnected || st[0].ToolCount != 2 {
		t.Fatalf("status: %+v", st)
	}
}

func TestManagerWrongTokenReportsError(t *testing.T) {
	ts := startHTTPMCP(t, requireBearer("Bearer right"))
	m := newManager(t)
	saveStatic(t, m, ts.URL, "Bearer wrong")
	if err := m.Ensure(ctx5(t), "echo"); err == nil {
		t.Fatal("expected auth failure")
	}
	st, _ := m.Status()
	if st[0].Status != mcp.StatusError || st[0].Error == "" {
		t.Fatalf("status: %+v", st)
	}
}

func TestManagerRejectsDisabledAndUnknown(t *testing.T) {
	m := newManager(t)
	if err := m.Connect("nope"); err == nil {
		t.Fatal("unknown server should error")
	}
	cfg := httpServer("off")
	cfg.Enabled = false
	cfg.Auth = nil
	if _, err := m.Config.Save(cfg); err != nil {
		t.Fatal(err)
	}
	if err := m.Connect("off"); err == nil {
		t.Fatal("disabled server should error")
	}
}

func TestLoadToolsAddsToolsLazilyAndCalls(t *testing.T) {
	ts := startHTTPMCP(t, nil)
	m := newManager(t)
	if _, err := m.Config.Save(mcp.ServerConfig{ID: "echo", Label: "Echo", Transport: mcp.TransportHTTP, URL: ts.URL, Enabled: true}); err != nil {
		t.Fatal(err)
	}
	reg := tools.NewRegistry()
	lt := mcp.NewLoadToolsTool(m, reg)
	if lt == nil {
		t.Fatal("loadTools should exist with an enabled server")
	}
	if !strings.Contains(lt.Description(), "mcp:echo") {
		t.Fatalf("description should list group: %s", lt.Description())
	}
	if len(reg.Definitions()) != 0 {
		t.Fatal("no MCP tools before loadTools")
	}

	if _, err := lt.Execute(ctx5(t), json.RawMessage(`{"group":"mcp:nope"}`)); err == nil {
		t.Fatal("unknown group should error")
	}
	if _, err := lt.Execute(ctx5(t), json.RawMessage(`{"group":"mcp:echo"}`)); err != nil {
		t.Fatalf("load: %v", err)
	}
	defs := reg.Definitions()
	if len(defs) != 2 {
		t.Fatalf("defs: %+v", defs)
	}
	for _, d := range defs {
		if d.InputSchema["type"] != "object" || d.InputSchema["properties"] == nil {
			t.Fatalf("schema passthrough lost: %+v", d)
		}
	}

	echo, _ := reg.Get("mcp__echo__get_echo")
	if echo.Tier() != tools.TierRead {
		t.Fatalf("get_ prefix should be read tier, got %s", echo.Tier())
	}
	out, err := echo.Execute(ctx5(t), json.RawMessage(`{"text":"yo"}`))
	if err != nil {
		t.Fatal(err)
	}
	env := out.(tools.Envelope)
	if env.IsError || !strings.Contains(toJSON(env.Content), "echo:yo") {
		t.Fatalf("result: %+v", env)
	}
	del, _ := reg.Get("mcp__echo__delete_all")
	if del.Tier() != tools.TierExec {
		t.Fatalf("delete_ prefix should be exec tier, got %s", del.Tier())
	}
	out, _ = del.Execute(ctx5(t), json.RawMessage(`{"text":"x"}`))
	if !out.(tools.Envelope).IsError {
		t.Fatal("server isError must propagate")
	}
}

func TestLoadToolsNilWithoutServers(t *testing.T) {
	m := newManager(t)
	if mcp.NewLoadToolsTool(m, tools.NewRegistry()) != nil {
		t.Fatal("no servers -> no loadTools tool")
	}
}

func TestManagerReconnectsAfterDisconnect(t *testing.T) {
	ts := startHTTPMCP(t, nil)
	m := newManager(t)
	if _, err := m.Config.Save(mcp.ServerConfig{ID: "echo", Label: "Echo", Transport: mcp.TransportHTTP, URL: ts.URL, Enabled: true}); err != nil {
		t.Fatal(err)
	}
	if err := m.Ensure(ctx5(t), "echo"); err != nil {
		t.Fatal(err)
	}
	m.Disconnect("echo")
	if _, err := m.Call(ctx5(t), "echo", "get_echo", map[string]any{"text": "again"}); err != nil {
		t.Fatalf("call after disconnect should reconnect: %v", err)
	}
}

func toJSON(v any) string { b, _ := json.Marshal(v); return string(b) }
