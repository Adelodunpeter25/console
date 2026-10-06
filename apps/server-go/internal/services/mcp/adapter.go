// Adapter: exposes MCP server tools as harness tools.Tool values, and the
// lazy `loadTools` tool that brings a server's tools into a run on demand.
package mcp

import (
	"context"
	"encoding/base64"
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

// maxResultImages caps how many image parts survive one tool result. Screenshots
// are megabytes each, and a result that inlines many of them blows up both the
// model's context and the session file.
const maxResultImages = 4

// renderResult flattens MCP content into the harness content-part shape. Text is
// joined into a single part; images are passed through as native image parts so
// the provider converters can turn them into real image blocks (Claude) rather
// than a placeholder. Audio is not a shape the converters accept, so it stays a
// text note.
func renderResult(res *sdk.CallToolResult) []map[string]any {
	var parts []string
	var out []map[string]any
	images := 0
	droppedImages := 0
	for _, c := range res.Content {
		switch v := c.(type) {
		case *sdk.TextContent:
			parts = append(parts, v.Text)
		case *sdk.ImageContent:
			if images >= maxResultImages {
				droppedImages++
				continue
			}
			images++
			out = append(out, imagePart(v))
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
	if len(parts) == 0 && len(out) == 0 && res.StructuredContent != nil {
		if raw, err := json.Marshal(res.StructuredContent); err == nil {
			parts = append(parts, string(raw))
		}
	}
	if droppedImages > 0 {
		parts = append(parts, fmt.Sprintf("[%d more image(s) omitted]", droppedImages))
	}
	text := strings.Join(parts, "\n")
	if text != "" || len(out) == 0 {
		if text == "" {
			text = "(no output)"
		}
		if len(text) > maxResultText {
			text = text[:maxResultText] + "\n… [truncated]"
		}
		out = append([]map[string]any{{"type": "text", "text": text}}, out...)
	}
	return out
}

// imagePart renders one MCP image as the harness image part shape, matching the
// browser tool. The SDK gives raw bytes, so it is base64-encoded here; an empty or
// unusable payload degrades to nothing rather than to a block the provider will
// reject.
func imagePart(v *sdk.ImageContent) map[string]any {
	mimeType := strings.TrimSpace(v.MIMEType)
	if mimeType == "" {
		mimeType = "image/png"
	}
	return map[string]any{
		"type":     "image",
		"data":     base64.StdEncoding.EncodeToString(v.Data),
		"mimeType": mimeType,
	}
}

type loadToolsInput struct {
	Group string `json:"group" jsonschema:"description=Tool group to load, e.g. mcp:<server-id>."`
}

// maxGroupToolNames caps how many tool names a group line previews.
const maxGroupToolNames = 8

// LoadToolsOptions tunes NewLoadToolsTool.
type LoadToolsOptions struct {
	// OnLoad is called with the group id after a group loads successfully,
	// so the caller can remember it (e.g. per session) and restore it later.
	OnLoad func(group string)
}

// GroupID is the loadTools group name for a server.
func GroupID(serverID string) string { return groupPrefix + serverID }

// LoadGroup connects the server behind group and appends its tools to
// registry. It returns the server config and the adapters that were added.
func LoadGroup(ctx context.Context, m *Manager, registry *tools.Registry, group string) (ServerConfig, []tools.Tool, error) {
	id := strings.TrimPrefix(strings.TrimSpace(group), groupPrefix)
	cfg, ok, err := m.Config.Get(id)
	if err != nil {
		return ServerConfig{}, nil, err
	}
	if !ok || !cfg.Enabled || !strings.HasPrefix(strings.TrimSpace(group), groupPrefix) {
		return ServerConfig{}, nil, fmt.Errorf("unknown tool group %q", group)
	}
	if err := m.Ensure(ctx, cfg.ID); err != nil {
		return cfg, nil, fmt.Errorf("could not connect to %s: %w", cfg.Label, err)
	}
	remote := m.Tools(cfg.ID)
	adapters := make([]tools.Tool, 0, len(remote))
	for _, rt := range remote {
		adapters = append(adapters, NewAdapter(m, cfg, rt))
	}
	registry.Add(adapters...)
	return cfg, adapters, nil
}

// groupLine describes one group: its label, plus a preview of tool names
// when the server is already connected (unconnected servers show only the
// label; their tools are unknown until first load).
func groupLine(m *Manager, cfg ServerConfig) string {
	line := fmt.Sprintf("- %s: %s", GroupID(cfg.ID), cfg.Label)
	remote := m.Tools(cfg.ID)
	if len(remote) == 0 {
		return line
	}
	names := make([]string, 0, maxGroupToolNames)
	for i, rt := range remote {
		if i == maxGroupToolNames {
			break
		}
		names = append(names, rt.Name)
	}
	more := ""
	if len(remote) > maxGroupToolNames {
		more = ", …"
	}
	return fmt.Sprintf("%s (%d tools: %s%s)", line, len(remote), strings.Join(names, ", "), more)
}

// NewLoadToolsTool builds the lazy loader. Calling it connects the server
// (opening the browser for OAuth servers on first use) and appends its tools
// to registry; they appear in the model's tool list from the next turn on.
// The description is built once here, so it stays byte-identical for the
// whole run. It returns nil when no enabled servers are configured.
func NewLoadToolsTool(m *Manager, registry *tools.Registry, opts ...LoadToolsOptions) tools.Tool {
	var opt LoadToolsOptions
	if len(opts) > 0 {
		opt = opts[0]
	}
	enabled := EnabledServers(m)
	if len(enabled) == 0 {
		return nil
	}
	var b strings.Builder
	b.WriteString("Load an external tool group so its tools become callable on the next step. Available groups:")
	for _, s := range enabled {
		b.WriteString("\n" + groupLine(m, s))
	}
	return tools.NewTool("loadTools", b.String(), tools.TierRead,
		func(ctx context.Context, in loadToolsInput) (any, error) {
			cfg, adapters, err := LoadGroup(ctx, m, registry, in.Group)
			if err != nil {
				return nil, tools.NewToolError("%s", capitalize(err.Error()))
			}
			if opt.OnLoad != nil {
				opt.OnLoad(GroupID(cfg.ID))
			}
			lines := make([]string, 0, len(adapters))
			for _, ad := range adapters {
				lines = append(lines, fmt.Sprintf("- %s: %s", ad.Name(), firstLine(ad.Description())))
			}
			return []map[string]any{{"type": "text", "text": fmt.Sprintf(
				"Loaded %d tools from %s. They are callable from your next step:\n%s",
				len(adapters), cfg.Label, strings.Join(lines, "\n"))}}, nil
		})
}

// EnabledServers lists enabled server configs (nil on error).
func EnabledServers(m *Manager) []ServerConfig {
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
	return enabled
}

func capitalize(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:] + "."
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
