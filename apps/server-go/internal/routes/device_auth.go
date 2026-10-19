package routes

import (
	"strings"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/auth"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/db"
	"github.com/gofiber/fiber/v2"
)

type authManager interface {
	Authenticate(string) (auth.Claims, error)
	Pair(string, string, string) (auth.PairedDevice, error)
	CreateDeviceGrant(string, string, string) (string, int64, error)
	RevokeDevice(string) error
	Devices() ([]db.AuthDevice, error)
}

func bearer(c *fiber.Ctx) string {
	scheme, value, ok := strings.Cut(strings.TrimSpace(c.Get("Authorization")), " ")
	if !ok || !strings.EqualFold(scheme, "bearer") {
		return ""
	}
	return strings.TrimSpace(value)
}

func RequireDeviceToken(manager authManager) fiber.Handler {
	return func(c *fiber.Ctx) error {
		claims, err := manager.Authenticate(bearer(c))
		if err != nil {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"success": false, "error": "Authentication required."})
		}
		c.Locals("authenticatedDeviceID", claims.Device)
		return c.Next()
	}
}

func RegisterPairingRoutes(app *fiber.App, manager authManager) {
	h := app.Group("/api/auth")
	h.Post("/pair", func(c *fiber.Ctx) error {
		var req struct{ Grant, DeviceName, Platform string }
		if err := c.BodyParser(&req); err != nil {
			return c.Status(400).JSON(fiber.Map{"success": false, "error": "Invalid request body."})
		}
		device, err := manager.Pair(req.Grant, req.DeviceName, req.Platform)
		if err != nil {
			return c.Status(401).JSON(fiber.Map{"success": false, "error": "Pairing grant is invalid or expired."})
		}
		return c.JSON(fiber.Map{"success": true, "data": fiber.Map{"device": fiber.Map{"id": device.ID, "name": device.Name, "platform": device.Platform}, "token": device.Token, "expiresAt": device.ExpiresAt}})
	})
}

func RegisterDeviceRoutes(app *fiber.App, manager authManager) {
	h := app.Group("/api/auth")
	h.Post("/pairing-grants", func(c *fiber.Ctx) error {
		var req struct{ DeviceName, Platform string }
		if err := c.BodyParser(&req); err != nil {
			return c.Status(400).JSON(fiber.Map{"success": false, "error": "Invalid request body."})
		}
		grant, exp, err := manager.CreateDeviceGrant(c.Locals("authenticatedDeviceID").(string), req.Platform, req.DeviceName)
		if err != nil {
			return c.Status(400).JSON(fiber.Map{"success": false, "error": err.Error()})
		}
		return c.JSON(fiber.Map{"success": true, "data": fiber.Map{"grant": grant, "expiresAt": exp}})
	})
	h.Get("/devices", func(c *fiber.Ctx) error {
		devices, err := manager.Devices()
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"success": false, "error": "Could not list devices."})
		}
		items := make([]fiber.Map, 0, len(devices))
		for _, device := range devices {
			items = append(items, fiber.Map{"id": device.ID, "name": device.Name, "platform": device.Platform, "createdAt": device.CreatedAt, "lastSeenAt": device.LastSeenAt, "revokedAt": device.RevokedAt})
		}
		return c.JSON(fiber.Map{"success": true, "data": fiber.Map{"devices": items}})
	})
	h.Delete("/devices/:id", func(c *fiber.Ctx) error {
		if err := manager.RevokeDevice(c.Params("id")); err != nil {
			return c.Status(500).JSON(fiber.Map{"success": false, "error": "Could not revoke device."})
		}
		return c.JSON(fiber.Map{"success": true})
	})
}
