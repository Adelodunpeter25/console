// MCP server routes (/api/mcp/*): config CRUD, connection control, tokens.
package routes

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	"github.com/gofiber/fiber/v2"

	consolev1 "github.com/Adelodunpeter25/console/apps/server-go/internal/gen/console/v1"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/services/mcp"
)

// mcpAuthToProto converts the auth object. The token ref is server-side
// state and never round-trips, but the ref itself is harmless metadata.
func mcpAuthToProto(a *mcp.AuthConfig) *consolev1.McpAuth {
	if a == nil {
		return nil
	}
	out := &consolev1.McpAuth{Type: a.Type}
	if a.TokenRef != "" {
		out.TokenRef = &a.TokenRef
	}
	return out
}

func mcpToolToProto(t mcp.RemoteTool) *consolev1.McpTool {
	out := &consolev1.McpTool{Name: t.Name}
	if t.Description != "" {
		out.Description = &t.Description
	}
	return out
}

// mcpConfigToProto converts the stored config. transport and enabled are
// always set: the old shape never omitted them, and the desktop requires
// transport on decode.
func mcpConfigToProto(cfg mcp.ServerConfig) *consolev1.McpServerConfig {
	transport, enabled := cfg.Transport, cfg.Enabled
	return &consolev1.McpServerConfig{
		Id: cfg.ID, Label: cfg.Label, Transport: &transport,
		Url: strOrNil(cfg.URL), Command: strOrNil(cfg.Command),
		Args: append([]string(nil), cfg.Args...),
		Env:  cfg.Env, Auth: mcpAuthToProto(cfg.Auth),
		TierOverrides: cfg.TierOverrides, Enabled: &enabled,
		CreatedAt: cfg.CreatedAt, UpdatedAt: cfg.UpdatedAt,
	}
}

// mcpStatusToProto converts one list row. The row is flat — the old Go type
// embedded ServerConfig — so the config keys repeat here; proto has no
// embedding.
func mcpStatusToProto(st mcp.ServerStatus) *consolev1.McpServerStatus {
	transport, enabled := st.Transport, st.Enabled
	out := &consolev1.McpServerStatus{
		Id: st.ID, Label: st.Label, Transport: &transport,
		Url: strOrNil(st.URL), Command: strOrNil(st.Command),
		Args: append([]string(nil), st.Args...),
		Env:  st.Env, Auth: mcpAuthToProto(st.Auth),
		TierOverrides: st.TierOverrides, Enabled: &enabled,
		CreatedAt: st.CreatedAt, UpdatedAt: st.UpdatedAt,
		Status: st.Status, ToolCount: int32(st.ToolCount),
	}
	if st.Error != "" {
		out.Error = &st.Error
	}
	if st.AuthURL != "" {
		out.AuthUrl = &st.AuthURL
	}
	for _, t := range st.Tools {
		out.Tools = append(out.Tools, mcpToolToProto(t))
	}
	return out
}

func strOrNil(v string) *string {
	if v == "" {
		return nil
	}
	return &v
}

func registerMCPRoutes(app *fiber.App, m *mcp.Manager) {
	ok := func(c *fiber.Ctx, data any, status ...int) error {
		code := fiber.StatusOK
		if len(status) > 0 {
			code = status[0]
		}
		return c.Status(code).JSON(fiber.Map{"success": true, "data": data})
	}
	h := app.Group("/api/mcp")

	h.Get("/servers", func(c *fiber.Ctx) error {
		list, err := m.Status()
		if err != nil {
			return fail400(c, err)
		}
		rows := make([]*consolev1.McpServerStatus, 0, len(list))
		for _, st := range list {
			rows = append(rows, mcpStatusToProto(st))
		}
		data, err := marshalProtoList(rows)
		if err != nil {
			return fail400(c, err)
		}
		return ok(c, json.RawMessage(data))
	})

	h.Get("/servers/:id", func(c *fiber.Ctx) error {
		cfg, found, err := m.Config.Get(c.Params("id"))
		if err != nil {
			return fail400(c, err)
		}
		if !found {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"success": false, "error": "MCP server not found."})
		}
		tools := m.Tools(cfg.ID)
		items := make([]*consolev1.McpTool, 0, len(tools))
		for _, t := range tools {
			items = append(items, mcpToolToProto(t))
		}
		configRaw, err := protoMarshal.Marshal(mcpConfigToProto(cfg))
		if err != nil {
			return fail400(c, err)
		}
		toolsRaw, err := marshalProtoList(items)
		if err != nil {
			return fail400(c, err)
		}
		return ok(c, fiber.Map{"config": json.RawMessage(configRaw), "tools": toolsRaw})
	})

	// Body: ServerConfig plus an optional "token" (static auth only), which
	// is stored in the credential file and never echoed back.
	//
	// The desktop client names two fields differently — `name` for the label
	// and `auth_type` for the auth type — and omits `enabled` entirely, so
	// both spellings are accepted and normalized here, at the wire boundary.
	// ServerConfig stays the one canonical shape that gets stored and
	// returned.
	save := func(c *fiber.Ctx) error {
		var body struct {
			mcp.ServerConfig
			Token    string `json:"token"`
			Name     string `json:"name"`
			AuthType string `json:"auth_type"`
			// Shadows ServerConfig.Enabled so an omitted field can be told
			// apart from an explicit false.
			Enabled *bool `json:"enabled"`
		}
		if err := c.BodyParser(&body); err != nil {
			return fail400(c, err)
		}
		cfg := body.ServerConfig
		if cfg.Label == "" {
			cfg.Label = body.Name
		}
		if cfg.Auth == nil && body.AuthType != "" {
			cfg.Auth = &mcp.AuthConfig{Type: body.AuthType}
		}
		// A client with no enable/disable control omits the field; a server
		// the user just finished configuring should be usable.
		if body.Enabled != nil {
			cfg.Enabled = *body.Enabled
		} else {
			cfg.Enabled = true
		}
		if id := c.Params("id"); id != "" {
			cfg.ID = id
		}
		if cfg.Auth != nil && cfg.Auth.TokenRef == "" && cfg.Auth.Type != mcp.AuthNone {
			cfg.Auth.TokenRef = cfg.ID
		}
		if body.Token != "" {
			if cfg.Auth == nil || cfg.Auth.Type != mcp.AuthStatic {
				return fail400(c, fmt.Errorf("token is only valid with static auth"))
			}
			header := body.Token
			if !strings.Contains(header, " ") {
				header = "Bearer " + header
			}
			if err := m.Credentials.Set(cfg.Auth.TokenRef, mcp.Credential{Kind: mcp.CredentialStatic, Header: header}); err != nil {
				return fail400(c, err)
			}
		}
		saved, err := m.Config.Save(cfg)
		if err != nil {
			return fail400(c, err)
		}
		// Config changed: drop any live session so it reconnects with the new one.
		m.Disconnect(saved.ID)
		raw, err := protoMarshal.Marshal(mcpStatusToProto(mcp.ServerStatus{ServerConfig: saved}))
		if err != nil {
			return fail400(c, err)
		}
		return ok(c, json.RawMessage(raw))
	}
	h.Post("/servers", func(c *fiber.Ctx) error { return save(c) })
	h.Put("/servers/:id", save)

	h.Delete("/servers/:id", func(c *fiber.Ctx) error {
		id := c.Params("id")
		if err := m.ResetAuth(id); err != nil {
			return fail400(c, err)
		}
		removed, err := m.Config.Delete(id)
		if err != nil {
			return fail400(c, err)
		}
		return ok(c, fiber.Map{"removed": removed})
	})

	// Connect starts in the background: for OAuth servers this opens the
	// browser; poll GET /servers for status / authUrl.
	h.Post("/servers/:id/connect", func(c *fiber.Ctx) error {
		var body struct {
			RedirectURI string `json:"redirectUri"`
		}
		// Body is optional: desktop sends none and keeps server-loopback.
		if len(c.Body()) > 0 {
			if err := c.BodyParser(&body); err != nil {
				return fail400(c, fmt.Errorf("invalid request body"))
			}
		}
		if body.RedirectURI != "" {
			u, err := url.Parse(body.RedirectURI)
			if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
				return fail400(c, fmt.Errorf("redirectUri must be an http(s) URL"))
			}
			if u.Scheme == "http" && u.Hostname() != "localhost" && u.Hostname() != "127.0.0.1" {
				return fail400(c, fmt.Errorf("redirectUri must target localhost for local loopback auth"))
			}
		}
		if err := m.Connect(c.Params("id"), body.RedirectURI); err != nil {
			return fail400(c, err)
		}
		return ok(c, fiber.Map{"started": true}, fiber.StatusAccepted)
	})

	// POST /servers/:id/oauth/callback — the mobile client forwards the
	// code+state it captured on its own loopback listener into the pending
	// server-initiated flow, mirroring the server's loopback handler.
	h.Post("/servers/:id/oauth/callback", func(c *fiber.Ctx) error {
		var body struct {
			State string `json:"state"`
			Code  string `json:"code"`
			Error string `json:"error"`
			Iss   string `json:"iss"`
		}
		if err := c.BodyParser(&body); err != nil || body.State == "" {
			return fail400(c, fmt.Errorf("state is required"))
		}
		if !m.OAuth.ResolveWaiter(body.State, body.Code, body.Error, body.Iss) {
			return c.Status(fiber.StatusConflict).JSON(fiber.Map{"success": false, "error": "unknown or stale state"})
		}
		return ok(c, fiber.Map{"forwarded": true})
	})

	h.Post("/servers/:id/disconnect", func(c *fiber.Ctx) error {
		m.Disconnect(c.Params("id"))
		return ok(c, fiber.Map{"disconnected": true})
	})

	h.Post("/servers/:id/reset-auth", func(c *fiber.Ctx) error {
		if err := m.ResetAuth(c.Params("id")); err != nil {
			return fail400(c, err)
		}
		return ok(c, fiber.Map{"reset": true})
	})
}

// RegisterMCPRoutes is the exported entry point (used by route tests).
func RegisterMCPRoutes(app *fiber.App, m *mcp.Manager) { registerMCPRoutes(app, m) }
