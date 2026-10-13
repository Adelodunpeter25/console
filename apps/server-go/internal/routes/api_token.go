// API bearer token routes and middleware for the QUIC/WebTransport
// migration (Phase 0): minting over REST, plus the single validator shared
// by the future WT path and the legacy path.
//
// The middleware is NOT mounted on any route yet: mounting bearer enforcement
// on legacy REST today would break clients that send opportunistic tokens.
// The WT listener (Phase 2) is its first consumer.
package routes

import (
	"strings"

	"github.com/gofiber/fiber/v2"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/auth"
)

// DeviceIDKey is the c.Locals key RequireAPIToken stores the validated
// device id under for downstream handlers.
const DeviceIDKey = "apiTokenDeviceID"

// RegisterTokenRoutes wires POST /api/auth/token (mint). Exported so route
// tests can build a Fiber app with only these routes, mirroring
// RegisterAuthRoutes.
func RegisterTokenRoutes(app *fiber.App, issuer *auth.TokenIssuer) {
	h := app.Group("/api/auth")

	h.Post("/token", func(c *fiber.Ctx) error {
		var req struct {
			DeviceID string `json:"deviceId"`
		}
		if err := c.BodyParser(&req); err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"success": false, "error": "Invalid request body."})
		}
		if strings.TrimSpace(req.DeviceID) == "" {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"success": false, "error": "deviceId is required."})
		}
		token, expiresAt, err := issuer.Mint(req.DeviceID, 0)
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"success": false, "error": err.Error()})
		}
		// Hand-shaped like the other control responses (abort/approve):
		// expiresAt stays a JSON number rather than a protojson string.
		return c.JSON(fiber.Map{"success": true, "data": fiber.Map{
			"token":     token,
			"expiresAt": expiresAt,
		}})
	})
}

// RequireAPIToken enforces a valid API token from either the
// `Authorization: Bearer` header or the `?token=` query parameter, so the WT
// handshake path and the legacy path share one validator. The validated
// device id lands in c.Locals(DeviceIDKey) for the handler (or, in Phase 2,
// for binding to the QUIC connection). Failures are a single 401 that does
// not distinguish expired from invalid.
func RequireAPIToken(issuer *auth.TokenIssuer) fiber.Handler {
	return func(c *fiber.Ctx) error {
		token := bearerToken(c.Get("Authorization"))
		if token == "" {
			token = strings.TrimSpace(c.Query("token"))
		}
		deviceID, err := issuer.Validate(token)
		if err != nil {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"success": false, "error": "A valid API token is required."})
		}
		c.Locals(DeviceIDKey, deviceID)
		return c.Next()
	}
}

// DeviceIDFromContext reads the device id RequireAPIToken stored.
func DeviceIDFromContext(c *fiber.Ctx) (string, bool) {
	deviceID, ok := c.Locals(DeviceIDKey).(string)
	return deviceID, ok && deviceID != ""
}

// bearerToken extracts a bearer credential. The scheme matches
// case-insensitively per RFC 9110; anything else (including a bare token
// with no scheme) is rejected so callers cannot accidentally send one form
// while the server reads another.
func bearerToken(header string) string {
	scheme, credentials, ok := strings.Cut(header, " ")
	if !ok || !strings.EqualFold(strings.TrimSpace(scheme), "bearer") {
		return ""
	}
	return strings.TrimSpace(credentials)
}
