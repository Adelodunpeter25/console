// The console.mcp.* methods: the model's control surface over MCP servers.
// Discovery is read-tier and always available; the config-write methods
// (add, update, remove) change user-level state that outlives the session and
// every project, so they demand the run's ceiling to be full-access.
package console

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/permissions"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/services/jskernel"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/services/mcp"
)

// mcpPollInterval and mcpConnectWait bound how long mcp.connect waits for a
// connect attempt to settle before reporting back as still connecting.
const (
	mcpPollInterval = 250 * time.Millisecond
	mcpConnectWait  = 15 * time.Second
)

// requireFullAccess gates the config-write methods. An unknown or empty
// ceiling fails closed, matching clampApproval.
func (r *Runtime) requireFullAccess(action string) error {
	if approvalRank[permissions.Mode(r.ceiling)] < approvalRank[permissions.FullAccess] {
		return fmt.Errorf("console.mcp.%s changes user-level MCP server config that affects every session and project, so it needs this run to be at full-access; ask the user to switch the approval mode or to edit the server in Settings → MCP", action)
	}
	return nil
}

// mcpServer finds one server in the live status list.
func (r *Runtime) mcpServer(id string) (mcp.ServerStatus, error) {
	statuses, err := r.mcp.Status()
	if err != nil {
		return mcp.ServerStatus{}, err
	}
	for _, st := range statuses {
		if st.ID == id {
			return st, nil
		}
	}
	return mcp.ServerStatus{}, fmt.Errorf("mcp server %q not found; see console.mcp.list()", id)
}

func (r *Runtime) mcpListImpl(_ context.Context, _ map[string]any) (*jskernel.Result, error) {
	if r.mcp == nil {
		return nil, fmt.Errorf("mcp service is unavailable")
	}
	statuses, err := r.mcp.Status()
	if err != nil {
		return nil, err
	}
	return marshalResult(statuses)
}

func (r *Runtime) mcpToolsImpl(_ context.Context, args map[string]any) (*jskernel.Result, error) {
	if r.mcp == nil {
		return nil, fmt.Errorf("mcp service is unavailable")
	}
	id, err := stringArg(args, "id")
	if err != nil {
		return nil, err
	}
	st, err := r.mcpServer(id)
	if err != nil {
		return nil, err
	}
	if st.Status != mcp.StatusConnected {
		return marshalResult(map[string]any{"id": id, "status": st.Status, "tools": []any{}})
	}
	return marshalResult(map[string]any{"id": id, "status": st.Status, "tools": st.Tools})
}

func (r *Runtime) mcpAddImpl(_ context.Context, args map[string]any) (*jskernel.Result, error) {
	if r.mcp == nil {
		return nil, fmt.Errorf("mcp service is unavailable")
	}
	if err := r.requireFullAccess("add"); err != nil {
		return nil, err
	}
	cfg, err := serverConfigFromArgs(args, nil)
	if err != nil {
		return nil, err
	}
	if _, exists, err := r.mcp.Config.Get(cfg.ID); err != nil {
		return nil, err
	} else if exists {
		return nil, fmt.Errorf("mcp server %q already exists; use console.mcp.update or console.mcp.remove", cfg.ID)
	}
	saved, err := r.mcp.Config.Save(*cfg)
	if err != nil {
		return nil, err
	}
	return marshalResult(saved)
}

func (r *Runtime) mcpUpdateImpl(_ context.Context, args map[string]any) (*jskernel.Result, error) {
	if r.mcp == nil {
		return nil, fmt.Errorf("mcp service is unavailable")
	}
	if err := r.requireFullAccess("update"); err != nil {
		return nil, err
	}
	id, err := stringArg(args, "id")
	if err != nil {
		return nil, err
	}
	prev, exists, err := r.mcp.Config.Get(id)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, fmt.Errorf("mcp server %q not found; see console.mcp.list()", id)
	}
	cfg, err := serverConfigFromArgs(args, &prev)
	if err != nil {
		return nil, err
	}
	saved, err := r.mcp.Config.Save(*cfg)
	if err != nil {
		return nil, err
	}
	// A server disabled through the kernel must not keep a live session; an
	// enabled one connects on the next console.mcp.connect call.
	if !saved.Enabled {
		r.mcp.Disconnect(id)
	}
	return marshalResult(saved)
}

// serverConfigFromArgs builds a validated ServerConfig from the model's
// arguments. When prev is set, absent keys keep the stored value (update);
// when it is nil, transport plus url or command are required (add). Secrets
// are never accepted here: the model cannot pass tokens, only an auth type
// whose credential is managed through Settings or the OAuth sign-in flow.
func serverConfigFromArgs(args map[string]any, prev *mcp.ServerConfig) (*mcp.ServerConfig, error) {
	cfg := &mcp.ServerConfig{Enabled: true}
	if prev != nil {
		cfg = &mcp.ServerConfig{
			ID: prev.ID, Label: prev.Label, Transport: prev.Transport,
			URL: prev.URL, Command: prev.Command, Args: prev.Args, Env: prev.Env,
			Auth: prev.Auth, TierOverrides: prev.TierOverrides, Enabled: prev.Enabled,
		}
	} else {
		id, err := stringArg(args, "id")
		if err != nil {
			return nil, err
		}
		cfg.ID = id
		transport, err := stringArg(args, "transport")
		if err != nil {
			return nil, err
		}
		cfg.Transport = transport
	}
	if label, err := optionalString(args, "label"); err != nil {
		return nil, err
	} else if label != "" {
		cfg.Label = label
	} else if prev == nil && cfg.Label == "" {
		cfg.Label = cfg.ID
	}
	if value, present := args["url"]; present {
		text, ok := value.(string)
		if !ok {
			return nil, fmt.Errorf("url must be a string, got %v", value)
		}
		cfg.URL = text
	}
	if value, present := args["command"]; present {
		text, ok := value.(string)
		if !ok {
			return nil, fmt.Errorf("command must be a string, got %v", value)
		}
		cfg.Command = text
	}
	if value, present := args["args"]; present {
		list, err := stringListArg(value, "args")
		if err != nil {
			return nil, err
		}
		cfg.Args = list
	}
	if value, present := args["env"]; present {
		obj, ok := value.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("env must be an object of strings, got %v", value)
		}
		env := mcp.EnvMap{}
		for k, v := range obj {
			text, ok := v.(string)
			if !ok {
				return nil, fmt.Errorf("env.%s must be a string, got %v", k, v)
			}
			env[k] = text
		}
		cfg.Env = env
	}
	if value, present := args["auth"]; present {
		text, _ := value.(string)
		switch text {
		case "", mcp.AuthNone:
			cfg.Auth = nil
		case mcp.AuthStatic, mcp.AuthOAuth2:
			cfg.Auth = &mcp.AuthConfig{Type: text, TokenRef: cfg.ID}
		default:
			return nil, fmt.Errorf("auth must be %q, %q or %q; tokens themselves are managed in Settings → MCP", mcp.AuthNone, mcp.AuthStatic, mcp.AuthOAuth2)
		}
	}
	if value, present := args["tier_overrides"]; present {
		obj, ok := value.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("tier_overrides must be an object mapping tool names to tiers, got %v", value)
		}
		overrides := map[string]string{}
		for k, v := range obj {
			text, ok := v.(string)
			if !ok {
				return nil, fmt.Errorf("tier_overrides[%s] must be a string, got %v", k, v)
			}
			overrides[k] = text
		}
		cfg.TierOverrides = overrides
	}
	if value, present := args["enabled"]; present {
		enabled, ok := value.(bool)
		if !ok {
			return nil, fmt.Errorf("enabled must be true or false, got %v", value)
		}
		cfg.Enabled = enabled
	}
	if err := mcp.ValidateServer(*cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}

func stringListArg(value any, name string) ([]string, error) {
	list, ok := value.([]any)
	if !ok {
		return nil, fmt.Errorf("%s must be an array of strings, got %v", name, value)
	}
	out := make([]string, 0, len(list))
	for _, item := range list {
		text, ok := item.(string)
		if !ok {
			return nil, fmt.Errorf("%s must be an array of strings, got element %v", name, item)
		}
		out = append(out, text)
	}
	return out, nil
}

func (r *Runtime) mcpRemoveImpl(_ context.Context, args map[string]any) (*jskernel.Result, error) {
	if r.mcp == nil {
		return nil, fmt.Errorf("mcp service is unavailable")
	}
	if err := r.requireFullAccess("remove"); err != nil {
		return nil, err
	}
	id, err := stringArg(args, "id")
	if err != nil {
		return nil, err
	}
	if _, exists, err := r.mcp.Config.Get(id); err != nil {
		return nil, err
	} else if !exists {
		return nil, fmt.Errorf("mcp server %q not found; see console.mcp.list()", id)
	}
	r.mcp.Disconnect(id)
	deleted, err := r.mcp.Config.Delete(id)
	if err != nil {
		return nil, err
	}
	if !deleted {
		return nil, fmt.Errorf("mcp server %q not found", id)
	}
	return &jskernel.Result{StructuredContent: json.RawMessage(`{"removed":true}`)}, nil
}

// mcpConnectImpl starts a connect and waits briefly for it to settle. An
// OAuth server cannot finish on its own — sign-in happens in the user's
// browser — so it reports needs_auth with the URL and the model relays that
// to the user instead of waiting out the whole login.
func (r *Runtime) mcpConnectImpl(ctx context.Context, args map[string]any) (*jskernel.Result, error) {
	if r.mcp == nil {
		return nil, fmt.Errorf("mcp service is unavailable")
	}
	id, err := stringArg(args, "id")
	if err != nil {
		return nil, err
	}
	if err := r.mcp.Connect(id); err != nil {
		return nil, err
	}
	deadline := time.Now().Add(mcpConnectWait)
	for {
		st, err := r.mcpServer(id)
		if err != nil {
			return nil, err
		}
		switch st.Status {
		case mcp.StatusConnected:
			return marshalResult(map[string]any{"id": id, "connected": true, "tool_count": st.ToolCount})
		case mcp.StatusNeedsAuth:
			return marshalResult(map[string]any{
				"id": id, "needs_auth": true, "auth_url": st.AuthURL,
				"note": "sign-in is waiting on the user: ask them to finish it in Settings → MCP (or open the auth_url); call console.mcp.connect again once they have",
			})
		case mcp.StatusError:
			return nil, fmt.Errorf("mcp server %q failed to connect: %s", id, st.Error)
		}
		if time.Now().After(deadline) {
			return marshalResult(map[string]any{
				"id": id, "connected": false, "status": st.Status,
				"note": "still " + st.Status + "; check console.mcp.list() for progress",
			})
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(mcpPollInterval):
		}
	}
}

func (r *Runtime) mcpDisconnectImpl(_ context.Context, args map[string]any) (*jskernel.Result, error) {
	if r.mcp == nil {
		return nil, fmt.Errorf("mcp service is unavailable")
	}
	id, err := stringArg(args, "id")
	if err != nil {
		return nil, err
	}
	if _, err := r.mcpServer(id); err != nil {
		return nil, err
	}
	r.mcp.Disconnect(id)
	return &jskernel.Result{StructuredContent: json.RawMessage(`{"disconnected":true}`)}, nil
}

func (r *Runtime) mcpResetAuthImpl(_ context.Context, args map[string]any) (*jskernel.Result, error) {
	if r.mcp == nil {
		return nil, fmt.Errorf("mcp service is unavailable")
	}
	id, err := stringArg(args, "id")
	if err != nil {
		return nil, err
	}
	if err := r.mcp.ResetAuth(id); err != nil {
		return nil, err
	}
	return &jskernel.Result{StructuredContent: json.RawMessage(`{"reset":true}`)}, nil
}

// mcpCallImpl runs one remote tool, the same path the adapter tools use.
// Tiers are enforced on the loaded harness tools; a direct call inherits the
// run's own console authority.
func (r *Runtime) mcpCallImpl(ctx context.Context, args map[string]any) (*jskernel.Result, error) {
	if r.mcp == nil {
		return nil, fmt.Errorf("mcp service is unavailable")
	}
	id, err := stringArg(args, "id")
	if err != nil {
		return nil, err
	}
	tool, err := stringArg(args, "tool")
	if err != nil {
		return nil, err
	}
	callArgs, _ := args["args"].(map[string]any)
	if callArgs == nil {
		if _, present := args["args"]; present {
			return nil, fmt.Errorf("args must be an object, got %v", args["args"])
		}
		callArgs = map[string]any{}
	}
	ctx, cancel := context.WithTimeout(ctx, mcp.CallTimeout)
	defer cancel()
	res, err := r.mcp.Call(ctx, id, tool, callArgs)
	if err != nil {
		return nil, err
	}
	return marshalResult(map[string]any{"id": id, "tool": tool, "content": mcp.ResultText(res), "is_error": res.IsError})
}
