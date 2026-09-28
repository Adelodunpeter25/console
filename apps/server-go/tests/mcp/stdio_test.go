// Stdio transport: the test binary re-executes itself as an MCP server.
package tests

import (
	"context"
	"os"
	"testing"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/services/mcp"
)

func TestMain(m *testing.M) {
	if os.Getenv("MCP_TEST_STDIO_SERVER") == "1" {
		_ = newEchoServer().Run(context.Background(), &sdk.StdioTransport{})
		return
	}
	os.Exit(m.Run())
}

func TestStdioServerConnectsWithEnv(t *testing.T) {
	m := newManager(t)
	if _, err := m.Config.Save(mcp.ServerConfig{
		ID: "local", Label: "Local", Transport: mcp.TransportStdio, Enabled: true,
		Command: os.Args[0], Env: map[string]string{"MCP_TEST_STDIO_SERVER": "1"},
	}); err != nil {
		t.Fatal(err)
	}
	if err := m.Ensure(ctx5(t), "local"); err != nil {
		t.Fatalf("ensure: %v", err)
	}
	res, err := m.Call(ctx5(t), "local", "get_echo", map[string]any{"text": "stdio"})
	if err != nil || res.IsError {
		t.Fatalf("call: %v %+v", err, res)
	}
	m.Disconnect("local") // must stop the subprocess without hanging
}

func TestStdioMissingCommandReportsError(t *testing.T) {
	m := newManager(t)
	m.Config.Save(mcp.ServerConfig{ID: "bad", Label: "Bad", Transport: mcp.TransportStdio, Enabled: true, Command: "/nonexistent/binary"})
	if err := m.Ensure(ctx5(t), "bad"); err == nil {
		t.Fatal("missing command should fail")
	}
}
