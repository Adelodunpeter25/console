// Shared helpers: in-process MCP servers for client/manager/oauth tests.
package tests

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

type echoIn struct {
	Text string `json:"text"`
}

func newEchoServer() *sdk.Server {
	s := sdk.NewServer(&sdk.Implementation{Name: "echo", Version: "1"}, nil)
	sdk.AddTool(s, &sdk.Tool{Name: "get_echo", Description: "Echo text back.\nSecond line."},
		func(ctx context.Context, req *sdk.CallToolRequest, in echoIn) (*sdk.CallToolResult, *struct{}, error) {
			return &sdk.CallToolResult{Content: []sdk.Content{&sdk.TextContent{Text: "echo:" + in.Text}}}, nil, nil
		})
	sdk.AddTool(s, &sdk.Tool{Name: "delete_all", Description: "Dangerous."},
		func(ctx context.Context, req *sdk.CallToolRequest, in echoIn) (*sdk.CallToolResult, *struct{}, error) {
			return &sdk.CallToolResult{IsError: true, Content: []sdk.Content{&sdk.TextContent{Text: "nope"}}}, nil, nil
		})
	return s
}

// startHTTPMCP serves the echo server over streamable HTTP, wrapped by mw.
func startHTTPMCP(t *testing.T, mw func(http.Handler) http.Handler) *httptest.Server {
	t.Helper()
	srv := newEchoServer()
	var h http.Handler = sdk.NewStreamableHTTPHandler(func(*http.Request) *sdk.Server { return srv }, nil)
	if mw != nil {
		h = mw(h)
	}
	ts := httptest.NewServer(h)
	t.Cleanup(ts.Close)
	return ts
}
