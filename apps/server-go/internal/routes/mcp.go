// MCP server routes (/api/mcp/*): config CRUD, connection control, tokens.
package routes

import (
	"fmt"
	"strings"

	"github.com/gofiber/fiber/v2"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/services/mcp"
)

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
		return ok(c, list)
	})

	h.Get("/servers/:id", func(c *fiber.Ctx) error {
		cfg, found, err := m.Config.Get(c.Params("id"))
		if err != nil {
			return fail400(c, err)
		}
		if !found {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"success": false, "error": "MCP server not found."})
		}
		return ok(c, fiber.Map{"config": cfg, "tools": m.Tools(cfg.ID)})
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
		return ok(c, saved)
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
		if err := m.Connect(c.Params("id")); err != nil {
			return fail400(c, err)
		}
		return ok(c, fiber.Map{"started": true}, fiber.StatusAccepted)
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
