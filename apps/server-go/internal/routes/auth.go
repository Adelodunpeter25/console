// OAuth auth routes. Port of apps/server/api/src/routes/auth.ts (codex
// slice): status, login URL generation, and the code-exchange callback.
// Other providers answer 501 until their Go ports land.
package routes

import (
	"github.com/gofiber/fiber/v2"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/auth"
)

func registerAuthRoutes(app *fiber.App, authSvc *auth.AuthService) {
	h := app.Group("/api/auth")

	h.Get("/status", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{"success": true, "data": authSvc.GetStatus()})
	})

	h.Post("/login/url", func(c *fiber.Ctx) error {
		var req struct {
			Provider string `json:"provider"`
		}
		if err := c.BodyParser(&req); err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"success": false, "error": "Invalid request body."})
		}
		if req.Provider == "" {
			req.Provider = "codex"
		}
		switch req.Provider {
		case "codex":
			result, err := authSvc.GetLoginURL()
			if err != nil {
				return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"success": false, "error": err.Error()})
			}
			return c.JSON(fiber.Map{"success": true, "data": result})
		case "antigravity", "devin", "claude":
			return c.Status(fiber.StatusNotImplemented).JSON(fiber.Map{"success": false, "error": "OAuth login for '" + req.Provider + "' is not supported by the Go server yet."})
		default:
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"success": false, "error": "Invalid OAuth provider."})
		}
	})

	h.Post("/login/callback", func(c *fiber.Ctx) error {
		var req struct {
			Provider string `json:"provider"`
			Code     string `json:"code"`
			State    string `json:"state"`
		}
		if err := c.BodyParser(&req); err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"success": false, "error": "Invalid request body."})
		}
		if req.Provider == "" {
			req.Provider = "codex"
		}
		switch req.Provider {
		case "codex":
			if req.Code == "" {
				return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"success": false, "error": "Authorization 'code' is required."})
			}
			result, err := authSvc.HandleCallback(req.Code, req.State)
			if err != nil {
				return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"success": false, "error": err.Error()})
			}
			return c.JSON(fiber.Map{"success": true, "data": result})
		case "antigravity", "devin", "claude":
			return c.Status(fiber.StatusNotImplemented).JSON(fiber.Map{"success": false, "error": "OAuth login for '" + req.Provider + "' is not supported by the Go server yet."})
		default:
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"success": false, "error": "Invalid OAuth provider."})
		}
	})
}
