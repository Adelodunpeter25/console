package codemode

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/permissions"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/services/jskernel"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/services/mcp"
)

// mcpBridge exposes discovery and calls of configured MCP servers inside the
// REPL. Config writes, auth resets and disconnects stay on console.mcp.*:
// those change user-level state that outlives the session.
type mcpBridge struct {
	manager *mcp.Manager
	mode    func() permissions.Mode
}

func (b *mcpBridge) methods() []jskernel.Method {
	return []jskernel.Method{
		{Name: "mcp.servers", Call: b.servers},
		{Name: "mcp.tools", Call: b.tools},
		{Name: "mcp.call", Call: b.call},
	}
}

func (b *mcpBridge) guard() error {
	if b == nil || b.manager == nil {
		return fmt.Errorf("MCP service is unavailable")
	}
	return nil
}

func (b *mcpBridge) servers(_ context.Context, _ map[string]any) (*jskernel.Result, error) {
	if err := b.guard(); err != nil {
		return nil, err
	}
	statuses, err := b.manager.Status()
	if err != nil {
		return nil, err
	}
	out := make([]map[string]any, 0, len(statuses))
	for _, st := range statuses {
		row := map[string]any{
			"id": st.ID, "label": st.Label, "status": st.Status,
			"error": st.Error, "toolCount": st.ToolCount,
		}
		if st.Status == mcp.StatusNeedsAuth && st.AuthURL != "" {
			row["authUrl"] = st.AuthURL
		}
		out = append(out, row)
	}
	return marshalResult(out)
}

func (b *mcpBridge) tools(ctx context.Context, args map[string]any) (*jskernel.Result, error) {
	if err := b.guard(); err != nil {
		return nil, err
	}
	id, err := stringArg(args, "id")
	if err != nil {
		return nil, err
	}
	statuses, err := b.manager.Status()
	if err != nil {
		return nil, err
	}
	found := false
	for _, st := range statuses {
		if st.ID == id {
			found = true
			break
		}
	}
	if !found {
		return nil, fmt.Errorf("MCP server %q not found; see mcp.servers()", id)
	}
	if err := b.manager.Ensure(ctx, id); err != nil {
		return nil, err
	}
	remote := b.manager.Tools(id)
	items := make([]map[string]any, 0, len(remote))
	for _, t := range remote {
		items = append(items, map[string]any{
			"name": t.Name, "description": t.Description, "inputSchema": t.InputSchema,
		})
	}
	return marshalResult(map[string]any{"id": id, "status": mcp.StatusConnected, "tools": items})
}

func (b *mcpBridge) call(ctx context.Context, args map[string]any) (*jskernel.Result, error) {
	if err := b.guard(); err != nil {
		return nil, err
	}
	id, err := stringArg(args, "id")
	if err != nil {
		return nil, err
	}
	name, err := stringArg(args, "tool")
	if err != nil {
		return nil, err
	}
	callArgs, _ := args["args"].(map[string]any)
	if callArgs == nil {
		if _, present := args["args"]; present {
			return nil, fmt.Errorf("args must be an object")
		}
		callArgs = map[string]any{}
	}
	statuses, err := b.manager.Status()
	if err != nil {
		return nil, err
	}
	var cfg mcp.ServerConfig
	for _, st := range statuses {
		if st.ID == id {
			cfg = st.ServerConfig
			break
		}
	}
	if cfg.ID == "" {
		return nil, fmt.Errorf("MCP server %q not found; see mcp.servers()", id)
	}
	if err := b.manager.Ensure(ctx, id); err != nil {
		return nil, err
	}
	var remote mcp.RemoteTool
	for _, t := range b.manager.Tools(id) {
		if t.Name == name {
			remote = t
			break
		}
	}
	tier := mcp.ResolveTier(cfg, remote)
	mode := permissions.PlanMode
	if b.mode != nil {
		mode = b.mode()
	}
	if permissions.Resolve(mode, tier) == permissions.Deny {
		return nil, fmt.Errorf("tool %s is denied in %s; switch to full access to run it", name, mode)
	}
	ctx, cancel := context.WithTimeout(ctx, mcp.CallTimeout)
	defer cancel()
	res, err := b.manager.Call(ctx, id, name, callArgs)
	if err != nil {
		return nil, err
	}
	return marshalResult(map[string]any{
		"id": id, "tool": name, "content": mcp.ResultText(res), "is_error": res.IsError,
	})
}

func stringArg(args map[string]any, key string) (string, error) {
	value, ok := args[key].(string)
	if !ok || value == "" {
		return "", fmt.Errorf("%s is required and must be a non-empty string", key)
	}
	return value, nil
}

func marshalResult(value any) (*jskernel.Result, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	return &jskernel.Result{StructuredContent: raw}, nil
}
