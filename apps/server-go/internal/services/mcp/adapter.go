// Adapter: exposes MCP server tools as harness tools.Tool values, and the
// lazy `loadTools` tool that brings a server's tools into a run on demand.
package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/invopop/jsonschema"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/tools"
)

const (
	callTimeout   = 2 * time.Minute
	maxResultText = 100_000
	groupPrefix   = "mcp:"
)

var unsafeName = regexp.MustCompile(`[^A-Za-z0-9_-]`)

// ToolName is the harness-facing name: mcp__<server>__<tool>, restricted to
// the charset providers accept and capped at 64 characters.
func ToolName(serverID, tool string) string {
	name := "mcp__" + serverID + "__" + unsafeName.ReplaceAllString(tool, "_")
	if len(name) > 64 {
		name = name[:64]
	}
	return name
}

// ResolveTier picks the permission tier: explicit config override, then the
// tool's readOnly annotation, then naming convention, then TierWrite.
func ResolveTier(cfg ServerConfig, t RemoteTool) tools.ToolTier {
	if v, ok := cfg.TierOverrides[t.Name]; ok {
		return tools.ToolTier(v)
	}
	if t.ReadOnly {
		return tools.TierRead
	}
	n := strings.ToLower(t.Name)
	switch {
	case hasAnyPrefix(n, "read_", "get_", "list_", "search_", "find_", "fetch_", "lookup_"):
		return tools.TierRead
	case hasAnyPrefix(n, "delete_", "remove_", "destroy_"):
		return tools.TierExec
	}
	return tools.TierWrite
}

func hasAnyPrefix(s string, prefixes ...string) bool {
	for _, p := range prefixes {
		if strings.HasPrefix(s, p) {
			return true
		}
	}
	return false
}

// Caller invokes a remote tool (satisfied by *Manager).
type Caller interface {
	Call(ctx context.Context, id, tool string, args map[string]any) (*sdk.CallToolResult, error)
}

type adapterTool struct {
	caller   Caller
	serverID string
	remote   RemoteTool
	name     string
	tier     tools.ToolTier
}

// NewAdapter wraps one remote tool.
func NewAdapter(caller Caller, cfg ServerConfig, remote RemoteTool) tools.Tool {
	return &adapterTool{caller: caller, serverID: cfg.ID, remote: remote,
		name: ToolName(cfg.ID, remote.Name), tier: ResolveTier(cfg, remote)}
}

func (a *adapterTool) Name() string { return a.name }
func (a *adapterTool) Description() string {
	if a.remote.Description == "" {
		return "MCP tool " + a.remote.Name + " (" + a.serverID + ")"
	}
	return a.remote.Description
}
func (a *adapterTool) Tier() tools.ToolTier       { return a.tier }
func (a *adapterTool) Schema() *jsonschema.Schema { return &jsonschema.Schema{Type: "object"} }
func (a *adapterTool) RawSchema() map[string]any  { return a.remote.InputSchema }

func (a *adapterTool) Execute(ctx context.Context, arguments json.RawMessage) (any, error) {
	var args map[string]any
	if len(arguments) > 0 {
		if err := json.Unmarshal(arguments, &args); err != nil {
			return nil, tools.NewToolError("Invalid arguments for %s: %v", a.name, err)
		}
	}
	ctx, cancel := context.WithTimeout(ctx, callTimeout)
	defer cancel()
	res, err := a.caller.Call(ctx, a.serverID, a.remote.Name, args)
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return nil, tools.NewToolError("MCP tool %s timed out after %s.", a.remote.Name, callTimeout)
		}
		return nil, tools.NewToolError("MCP tool %s failed: %v", a.remote.Name, err)
	}
	return tools.Envelope{Content: renderResult(res), IsError: res.IsError}, nil
}

// renderResult flattens MCP content into the harness text-part shape.
func renderResult(res *sdk.CallToolResult) []map[string]any {
	var parts []string
	for _, c := range res.Content {
		switch v := c.(type) {
		case *sdk.TextContent:
			parts = append(parts, v.Text)
		case *sdk.ImageContent:
			parts = append(parts, fmt.Sprintf("[image %s, %d bytes omitted]", v.MIMEType, len(v.Data)))
		case *sdk.AudioContent:
			parts = append(parts, "[audio content omitted]")
		case *sdk.ResourceLink:
			parts = append(parts, fmt.Sprintf("[resource %s: %s]", v.Name, v.URI))
		case *sdk.EmbeddedResource:
			if v.Resource != nil && v.Resource.Text != "" {
				parts = append(parts, v.Resource.Text)
			} else if v.Resource != nil {
				parts = append(parts, "[embedded resource "+v.Resource.URI+"]")
			}
		}
	}
	if len(parts) == 0 && res.StructuredContent != nil {
		if raw, err := json.Marshal(res.StructuredContent); err == nil {
			parts = append(parts, string(raw))
		}
	}
	text := strings.Join(parts, "\n")
	if text == "" {
		text = "(no output)"
	}
	if len(text) > maxResultText {
		text = text[:maxResultText] + "\n… [truncated]"
	}
	return []map[string]any{{"type": "text", "text": text}}
}

type loadToolsInput struct {
	Group string `json:"group" jsonschema:"description=Tool group to load, e.g. mcp:atlassian."`
}

// NewLoadToolsTool builds the lazy loader. Calling it connects the server
// (opening the browser for OAuth servers on first use) and appends its tools
// to registry; they appear in the model's tool list from the next turn on.
// It returns nil when no enabled servers are configured.
func NewLoadToolsTool(m *Manager, registry *tools.Registry) tools.Tool {
	servers, err := m.Config.List()
	if err != nil {
		return nil
	}
	var enabled []ServerConfig
	for _, s := range servers {
		if s.Enabled {
			enabled = append(enabled, s)
		}
	}
	if len(enabled) == 0 {
		return nil
	}
	var b strings.Builder
	b.WriteString("Load an external tool group so its tools become callable on the next step. Available groups:")
	byGroup := map[string]ServerConfig{}
	for _, s := range enabled {
		byGroup[groupPrefix+s.ID] = s
		fmt.Fprintf(&b, "\n- %s%s: %s", groupPrefix, s.ID, s.Label)
	}
	return tools.NewTool("loadTools", b.String(), tools.TierRead,
		func(ctx context.Context, in loadToolsInput) (any, error) {
			cfg, ok := byGroup[strings.TrimSpace(in.Group)]
			if !ok {
				return nil, tools.NewToolError("Unknown tool group %q.", in.Group)
			}
			if err := m.Ensure(ctx, cfg.ID); err != nil {
				return nil, tools.NewToolError("Could not connect to %s: %v", cfg.Label, err)
			}
			remote := m.Tools(cfg.ID)
			adapters := make([]tools.Tool, 0, len(remote))
			var lines []string
			for _, rt := range remote {
				ad := NewAdapter(m, cfg, rt)
				adapters = append(adapters, ad)
				desc := firstLine(rt.Description)
				lines = append(lines, fmt.Sprintf("- %s: %s", ad.Name(), desc))
			}
			registry.Add(adapters...)
			return []map[string]any{{"type": "text", "text": fmt.Sprintf(
				"Loaded %d tools from %s. They are callable from your next step:\n%s",
				len(adapters), cfg.Label, strings.Join(lines, "\n"))}}, nil
		})
}

func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	if len(s) > 160 {
		s = s[:160] + "…"
	}
	return s
}
