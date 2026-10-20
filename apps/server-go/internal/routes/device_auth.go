package routes

import (
	"encoding/json"
	"strings"

	"github.com/gofiber/fiber/v2"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/auth"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/db"
	consolev1 "github.com/Adelodunpeter25/console/apps/server-go/internal/gen/console/v1"
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

// RegisterPairingRoutes wires the public pairing exchange. The grant is the
// only thing that authorizes a device, and it is consumed on use.
func RegisterPairingRoutes(app *fiber.App, manager authManager) {
	h := app.Group("/api/auth")

	h.Post("/pair", func(c *fiber.Ctx) error {
		var req consolev1.PairDeviceRequest
		if err := protoUnmarshal.Unmarshal(c.Body(), &req); err != nil {
			return c.Status(400).JSON(fiber.Map{"success": false, "error": "Invalid request body."})
		}
		device, err := manager.Pair(req.GetGrant(), req.GetDeviceName(), req.GetPlatform())
		if err != nil {
			return c.Status(401).JSON(fiber.Map{"success": false, "error": "Pairing grant is invalid or expired."})
		}
		raw, err := protoMarshal.Marshal(&consolev1.PairDeviceResponse{
			Device: &consolev1.PairedDevice{
				Id:       device.ID,
				Name:     device.Name,
				Platform: device.Platform,
			},
			Token:     device.Token,
			ExpiresAt: device.ExpiresAt,
		})
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"success": false, "error": "encode failed"})
		}
		return c.JSON(fiber.Map{"success": true, "data": json.RawMessage(raw)})
	})
}

// RegisterDeviceRoutes wires the authenticated device-management routes.
func RegisterDeviceRoutes(app *fiber.App, manager authManager) {
	h := app.Group("/api/auth")

	h.Post("/pairing-grants", func(c *fiber.Ctx) error {
		var req consolev1.PairingGrantRequest
		if err := protoUnmarshal.Unmarshal(c.Body(), &req); err != nil {
			return c.Status(400).JSON(fiber.Map{"success": false, "error": "Invalid request body."})
		}
		creator, _ := c.Locals("authenticatedDeviceID").(string)
		grant, exp, err := manager.CreateDeviceGrant(creator, req.GetPlatform(), req.GetDeviceName())
		if err != nil {
			return c.Status(400).JSON(fiber.Map{"success": false, "error": err.Error()})
		}
		raw, err := protoMarshal.Marshal(&consolev1.PairingGrantResponse{
			Grant:     grant,
			ExpiresAt: exp,
		})
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"success": false, "error": "encode failed"})
		}
		return c.JSON(fiber.Map{"success": true, "data": json.RawMessage(raw)})
	})

	h.Get("/devices", func(c *fiber.Ctx) error {
		devices, err := manager.Devices()
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"success": false, "error": "Could not list devices."})
		}
		msg := &consolev1.ServerDevicesResponse{Devices: make([]*consolev1.ServerDevice, 0, len(devices))}
		for _, device := range devices {
			msg.Devices = append(msg.Devices, &consolev1.ServerDevice{
				Id:         device.ID,
				Name:       device.Name,
				Platform:   device.Platform,
				CreatedAt:  device.CreatedAt,
				LastSeenAt: device.LastSeenAt,
				RevokedAt:  device.RevokedAt,
			})
		}
		raw, err := protoMarshal.Marshal(msg)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"success": false, "error": "encode failed"})
		}
		return c.JSON(fiber.Map{"success": true, "data": json.RawMessage(raw)})
	})

	h.Delete("/devices/:id", func(c *fiber.Ctx) error {
		if err := manager.RevokeDevice(c.Params("id")); err != nil {
			return c.Status(500).JSON(fiber.Map{"success": false, "error": "Could not revoke device."})
		}
		return c.JSON(fiber.Map{"success": true})
	})
}
