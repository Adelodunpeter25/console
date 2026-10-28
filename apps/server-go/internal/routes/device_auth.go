package routes

import (
	"encoding/json"
	"fmt"
	"html"
	"os"
	"path/filepath"
	"strings"

	"github.com/gofiber/fiber/v2"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/auth"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/db"
	consolev1 "github.com/Adelodunpeter25/console/apps/server-go/internal/gen/console/v1"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/tunnel"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/utils"
)

type authManager interface {
	Authenticate(string) (auth.Claims, error)
	Pair(string, string, string) (auth.PairedDevice, error)
	CreateDeviceGrant(string, string, string) (string, int64, error)
	RevokeDevice(string) error
	RevokeOtherDevices(string) (int64, error)
	RenameDevice(string, string) error
	Devices() ([]db.AuthDevice, error)
	ServerInfo() auth.ServerIdentity
}

func bearer(c *fiber.Ctx) string {
	scheme, value, ok := strings.Cut(strings.TrimSpace(c.Get("Authorization")), " ")
	if !ok || !strings.EqualFold(scheme, "bearer") {
		return ""
	}
	return strings.TrimSpace(value)
}

// deviceStreamPath reports whether path is `/api/devices/<id>/stream`, the
// websocket a player page opens on the device's behalf.
func deviceStreamPath(path string) bool {
	return strings.HasPrefix(path, "/api/devices/") && strings.HasSuffix(path, "/stream")
}

// isWebSocketUpgrade mirrors the handshake test the upgrader performs. Browser
// scripting cannot set an Authorization header on a WebSocket handshake.
func isWebSocketUpgrade(c *fiber.Ctx) bool {
	return strings.EqualFold(c.Get("Upgrade"), "websocket") &&
		strings.Contains(strings.ToLower(c.Get("Connection")), "upgrade")
}

func RequireDeviceToken(manager authManager) fiber.Handler {
	return func(c *fiber.Ctx) error {
		// The player page is inert HTML with no data or secrets, and a webview
		// navigation cannot carry an Authorization header.
		if c.Method() == fiber.MethodGet && c.Path() == "/api/devices/player" {
			return c.Next()
		}
		credential := bearer(c)
		// A player page opening the stream websocket cannot send the header
		// either, so the device stream accepts its credential in the query
		// string — the same carve-out the WebTransport handshake makes.
		// Ordinary HTTP stays header-only, keeping tokens out of access logs.
		if credential == "" && isWebSocketUpgrade(c) && deviceStreamPath(c.Path()) {
			credential = strings.TrimSpace(c.Query("token"))
		}
		claims, err := manager.Authenticate(credential)
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

	// The pairing URL's path, so opening it in a browser explains itself
	// instead of 404ing. It shows only the server's public identity — the
	// grant lives in the URL fragment, which the browser never sends, so
	// this endpoint cannot leak it.
	app.Get("/console/pair", func(c *fiber.Ctx) error {
		info := manager.ServerInfo()
		return c.Type("html").SendString(fmt.Sprintf(
			`<!doctype html><html><head><meta charset="utf-8">`+
				`<title>Pair with %s</title></head><body>`+
				`<h1>%s</h1>`+
				`<p>Console server <code>%s</code>, version %s.</p>`+
				`<p>Open the Console app on the device you want to connect and paste `+
				`the whole pairing URL into it.</p>`+
				`</body></html>`,
			html.EscapeString(info.Name), html.EscapeString(info.Name),
			html.EscapeString(info.ID), html.EscapeString(info.Version),
		))
	})

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
			Server:    serverIdentityProto(manager.ServerInfo()),
		})
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"success": false, "error": "encode failed"})
		}
		return c.JSON(fiber.Map{"success": true, "data": json.RawMessage(raw)})
	})
}

// serverIdentityProto converts the server's identity for the wire.
//
// The base URL is omitted when empty: protojson drops an implicit empty
// string anyway, and a client that sees no base_url knows the server did
// not claim one rather than being handed a blank.
func serverIdentityProto(info auth.ServerIdentity) *consolev1.ServerIdentity {
	return &consolev1.ServerIdentity{
		Id:      info.ID,
		Name:    info.Name,
		Version: info.Version,
		BaseUrl: info.BaseURL,
	}
}

// qrBaseURL picks the address a minted QR should name.
//
// The caller reports the address it is talking to, but that is often the
// LAN address, useless to a phone on another network. The tunnel reaches
// the server from anywhere, so it is preferred whenever one exists and its
// certificate is issued — the same preference `console pair` applies. A
// CONSOLE_PUBLIC_URL override means the operator has already stated the
// address clients use, so the caller's answer stands.
func qrBaseURL(requested string) string {
	if strings.TrimSpace(os.Getenv("CONSOLE_PUBLIC_URL")) != "" {
		return requested
	}
	if remote, ok := tunnel.ReadInfo(filepath.Join(utils.ConsoleStorageDir(), "tunnel")); ok && remote.Ready {
		return remote.URL
	}
	return requested
}

// RegisterDeviceRoutes wires the authenticated device-management routes.
func RegisterDeviceRoutes(app *fiber.App, manager authManager) {	h := app.Group("/api/auth")

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
		info := manager.ServerInfo()
		// The QR carries the address the *other* device should use, which
		// only the caller knows, and the code, which only the server knows.
		payload := auth.PairingPayload(qrBaseURL(req.GetBaseUrl()), info.ID, grant)
		raw, err := protoMarshal.Marshal(&consolev1.PairingGrantResponse{
			Grant:     grant,
			ExpiresAt: exp,
			Server:    serverIdentityProto(info),
			QrPng:     auth.RenderPairingQR(payload),
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

	// Revoke every device except the caller's. The caller's own credential
	// keeps working — the point is to sign out everything else.
	h.Post("/devices/revoke-others", func(c *fiber.Ctx) error {
		caller, _ := c.Locals("authenticatedDeviceID").(string)
		count, err := manager.RevokeOtherDevices(caller)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"success": false, "error": "Could not revoke other devices."})
		}
		raw, err := protoMarshal.Marshal(&consolev1.RevokeOthersResponse{RevokedCount: count})
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"success": false, "error": "encode failed"})
		}
		return c.JSON(fiber.Map{"success": true, "data": json.RawMessage(raw)})
	})

	// Rename the caller's own device. There is no id in the request: the
	// server renames whoever is authenticated, so a device cannot rename
	// another one.
	h.Post("/devices/rename", func(c *fiber.Ctx) error {
		var req consolev1.RenameDeviceRequest
		if err := protoUnmarshal.Unmarshal(c.Body(), &req); err != nil {
			return c.Status(400).JSON(fiber.Map{"success": false, "error": "Invalid request body."})
		}
		caller, _ := c.Locals("authenticatedDeviceID").(string)
		if caller == "" {
			return c.Status(401).JSON(fiber.Map{"success": false, "error": "Authentication required."})
		}
		if err := manager.RenameDevice(caller, req.GetName()); err != nil {
			return c.Status(400).JSON(fiber.Map{"success": false, "error": err.Error()})
		}
		devices, err := manager.Devices()
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"success": false, "error": "Could not list devices."})
		}
		for _, device := range devices {
			if device.ID != caller {
				continue
			}
			raw, err := protoMarshal.Marshal(&consolev1.RenameDeviceResponse{
				Device: &consolev1.ServerDevice{
					Id:         device.ID,
					Name:       device.Name,
					Platform:   device.Platform,
					CreatedAt:  device.CreatedAt,
					LastSeenAt: device.LastSeenAt,
					RevokedAt:  device.RevokedAt,
				},
			})
			if err != nil {
				return c.Status(500).JSON(fiber.Map{"success": false, "error": "encode failed"})
			}
			return c.JSON(fiber.Map{"success": true, "data": json.RawMessage(raw)})
		}
		return c.Status(404).JSON(fiber.Map{"success": false, "error": "Device not found."})
	})
}
