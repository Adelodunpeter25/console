// MCP client: transports (streamable HTTP / stdio), tool listing and calls
// on top of the official go-sdk.
package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"

	"github.com/modelcontextprotocol/go-sdk/auth"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// RemoteTool is one tool advertised by an MCP server.
type RemoteTool struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema"`
	ReadOnly    bool           `json:"readOnly"`
}

// Client is a connected MCP session.
type Client struct {
	session *sdk.ClientSession
}

func newSDKClient(onToolsChanged func()) *sdk.Client {
	opts := &sdk.ClientOptions{}
	if onToolsChanged != nil {
		opts.ToolListChangedHandler = func(context.Context, *sdk.ToolListChangedRequest) { onToolsChanged() }
	}
	return sdk.NewClient(&sdk.Implementation{Name: "console", Version: "1.0.0"}, opts)
}

// staticHeaderTransport adds a fixed Authorization header to every request.
type staticHeaderTransport struct {
	header string
	base   http.RoundTripper
}

func (t staticHeaderTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req = req.Clone(req.Context())
	req.Header.Set("Authorization", t.header)
	return t.base.RoundTrip(req)
}

// buildTransport turns a server config into an SDK transport. handler is only
// used for oauth2 http servers; creds resolves static credentials.
func buildTransport(cfg ServerConfig, creds *CredentialStore, handler auth.OAuthHandler) (sdk.Transport, error) {
	switch cfg.Transport {
	case TransportStdio:
		cmd := exec.Command(cfg.Command, cfg.Args...)
		cmd.Env = os.Environ()
		for k, v := range cfg.Env {
			cmd.Env = append(cmd.Env, k+"="+v)
		}
		return &sdk.CommandTransport{Command: cmd}, nil
	case TransportHTTP:
		t := &sdk.StreamableClientTransport{Endpoint: cfg.URL}
		if cfg.Auth != nil {
			switch cfg.Auth.Type {
			case AuthStatic:
				c, ok, err := creds.Get(cfg.Auth.TokenRef)
				if err != nil {
					return nil, err
				}
				if !ok || c.Header == "" {
					return nil, fmt.Errorf("no credential stored for %s; set a token first", cfg.ID)
				}
				t.HTTPClient = &http.Client{Transport: staticHeaderTransport{header: c.Header, base: http.DefaultTransport}}
			case AuthOAuth2:
				t.OAuthHandler = handler
			}
		}
		return t, nil
	}
	return nil, fmt.Errorf("unknown transport %q", cfg.Transport)
}

// ListTools returns every tool the server advertises.
func (c *Client) ListTools(ctx context.Context) ([]RemoteTool, error) {
	var out []RemoteTool
	for t, err := range c.session.Tools(ctx, nil) {
		if err != nil {
			return nil, err
		}
		rt := RemoteTool{Name: t.Name, Description: t.Description, InputSchema: schemaMap(t.InputSchema)}
		if t.Annotations != nil {
			rt.ReadOnly = t.Annotations.ReadOnlyHint
		}
		out = append(out, rt)
	}
	return out, nil
}

func schemaMap(v any) map[string]any {
	m, ok := v.(map[string]any)
	if !ok {
		raw, err := json.Marshal(v)
		if err == nil {
			_ = json.Unmarshal(raw, &m)
		}
	}
	if m == nil {
		m = map[string]any{}
	}
	if _, ok := m["type"]; !ok {
		m["type"] = "object"
	}
	return m
}

// Call invokes a tool on the server.
func (c *Client) Call(ctx context.Context, name string, args map[string]any) (*sdk.CallToolResult, error) {
	return c.session.CallTool(ctx, &sdk.CallToolParams{Name: name, Arguments: args})
}

// Wait blocks until the session ends.
func (c *Client) Wait() error { return c.session.Wait() }

// Close ends the session (and stops a stdio subprocess).
func (c *Client) Close() error { return c.session.Close() }
