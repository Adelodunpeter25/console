package tests

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/codemode"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/loop"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/permissions"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/tools"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/services/mcp"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

type mcpBridgeIn struct {
	Text string `json:"text"`
}

func startBridgeMCP(t *testing.T) *httptest.Server {
	t.Helper()
	srv := sdk.NewServer(&sdk.Implementation{Name: "jira", Version: "1"}, nil)
	sdk.AddTool(srv, &sdk.Tool{Name: "search_issues", Description: "Search Jira issues."},
		func(ctx context.Context, req *sdk.CallToolRequest, in mcpBridgeIn) (*sdk.CallToolResult, *struct{}, error) {
			return &sdk.CallToolResult{Content: []sdk.Content{&sdk.TextContent{Text: "issue:" + in.Text}}}, nil, nil
		})
	sdk.AddTool(srv, &sdk.Tool{Name: "delete_issue", Description: "Dangerous."},
		func(ctx context.Context, req *sdk.CallToolRequest, in mcpBridgeIn) (*sdk.CallToolResult, *struct{}, error) {
			return &sdk.CallToolResult{Content: []sdk.Content{&sdk.TextContent{Text: "deleted:" + in.Text}}}, nil, nil
		})
	ts := httptest.NewServer(sdk.NewStreamableHTTPHandler(func(*http.Request) *sdk.Server { return srv }, nil))
	t.Cleanup(ts.Close)
	return ts
}

func bridgeManager(t *testing.T) *mcp.Manager {
	t.Helper()
	dir := t.TempDir()
	m := mcp.NewManager(mcp.NewConfigStore(dir), mcp.NewCredentialStore(dir), nil)
	ts := startBridgeMCP(t)
	t.Cleanup(m.CloseAll)
	if _, err := m.Config.Save(mcp.ServerConfig{
		ID: "jira", Label: "Jira", Transport: mcp.TransportHTTP, URL: ts.URL, Enabled: true,
	}); err != nil {
		t.Fatal(err)
	}
	return m
}

func newBridgeRuntime(t *testing.T, m *mcp.Manager, mode permissions.Mode) *codemode.Runtime {
	t.Helper()
	reg := tools.NewRegistry(tools.DefaultTools()...)
	rt, err := codemode.NewRuntime(loop.NewExecutor(reg, mode), codemode.Options{MCP: m, Mode: func() permissions.Mode { return mode }})
	if err != nil {
		t.Fatal(err)
	}
	return rt
}

func evalBridge(t *testing.T, rt *codemode.Runtime, script string) string {
	t.Helper()
	res, err := rt.Eval(context.Background(), script, 15*time.Second)
	if err != nil {
		t.Fatalf("eval: %v", err)
	}
	env := res.(tools.Envelope)
	if env.IsError {
		t.Fatalf("eval failed: %v", env.Content)
	}
	return env.Content.(string)
}

func TestCodemodeMCPNamespaceDiscoveryAndCall(t *testing.T) {
	rt := newBridgeRuntime(t, bridgeManager(t), permissions.FullAccess)

	servers := evalBridge(t, rt, `(await mcp.servers()).map(s => [s.id, s.label, s.status]);`)
	if !strings.Contains(servers, "jira") || !strings.Contains(servers, "Jira") {
		t.Fatalf("mcp.servers: %s", servers)
	}

	listed := evalBridge(t, rt, `(await mcp.tools({id:"jira"})).tools.map(t => t.name);`)
	if !strings.Contains(listed, "search_issues") || !strings.Contains(listed, "delete_issue") {
		t.Fatalf("mcp.tools: %s", listed)
	}

	out := evalBridge(t, rt, `(await mcp.call({id:"jira", tool:"search_issues", args:{text:"abc"}})).content;`)
	if !strings.Contains(out, "issue:abc") {
		t.Fatalf("mcp.call: %s", out)
	}
}

func TestCodemodeMCPNamespaceTierDeniedInPlanMode(t *testing.T) {
	rt := newBridgeRuntime(t, bridgeManager(t), permissions.PlanMode)
	// A write-tier MCP tool must be denied in plan mode. The JS bridge throws
	// on method failure, which surfaces as an error envelope from Eval.
	res, _ := rt.Eval(context.Background(), `await mcp.call({id:"jira", tool:"delete_issue", args:{text:"x"}});`, 15*time.Second)
	env := res.(tools.Envelope)
	if !env.IsError || !strings.Contains(env.Content.(string), "denied") {
		t.Fatalf("expected tier denial, got: isError=%v %v", env.IsError, env.Content)
	}
	// Read-tier calls still work in plan mode.
	out := evalBridge(t, rt, `(await mcp.call({id:"jira", tool:"search_issues", args:{text:"plan"}})).content;`)
	if !strings.Contains(out, "issue:plan") {
		t.Fatalf("plan-mode read call: %s", out)
	}
}

func TestCodemodeSetupHintMentionsMCP(t *testing.T) {
	// The hint is a const on the codemode package; assert through the exposed
	// tool description so this stays valid if the hint moves.
	reg := tools.NewRegistry()
	rt, err := codemode.NewRuntime(loop.NewExecutor(reg, permissions.FullAccess), codemode.Options{})
	if err != nil {
		t.Fatal(err)
	}
	_ = rt
	if !strings.Contains(codemode.SetupHint, "mcp.call") || !strings.Contains(codemode.SetupHint, "Do NOT use fetch/webSearch") {
		t.Fatalf("setup hint missing MCP guidance: %s", codemode.SetupHint)
	}
}