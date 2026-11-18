// Public webhook ingress (/api/webhooks/:topic and /api/webhooks/:topic/:id).
// Registered above RequireDeviceToken because external senders authenticate
// with the subscription's secret or HMAC signature rather than a device token.
package routes

import (
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/gofiber/fiber/v2"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/services/webhooks"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/tunnel"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/tunnel/opentunnel"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/utils"
)

// WebhookDeliverFunc matches and delivers an incoming webhook payload.
type WebhookDeliverFunc func(topic, subID string, headers map[string]string, queryToken string, body []byte) (int, error)

// RegisterWebhookRoutes wires the public webhook ingress endpoints.
func RegisterWebhookRoutes(app *fiber.App, deliver WebhookDeliverFunc) {
	h := app.Group("/api/webhooks")
	handler := func(c *fiber.Ctx) error {
		if deliver == nil {
			return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{"success": false, "error": "webhooks unavailable"})
		}
		topic := c.Params("topic")
		subID := c.Params("id")
		headers := make(map[string]string)
		c.Request().Header.VisitAll(func(k, v []byte) {
			headers[string(k)] = string(v)
		})
		queryToken := c.Query("token")
		matched, err := deliver(topic, subID, headers, queryToken, c.Body())
		switch {
		case err == nil:
			return c.JSON(fiber.Map{"success": true, "matched": matched})
		case errors.Is(err, webhooks.ErrNotFound):
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"success": false, "error": err.Error()})
		case errors.Is(err, webhooks.ErrUnauthorized):
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"success": false, "error": err.Error()})
		default:
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"success": false, "error": err.Error()})
		}
	}
	h.Post("/:topic", handler)
	h.Post("/:topic/:id", handler)
}

func defaultWebhookBaseURL() string {
	return webhookBaseURLFromDir(filepath.Join(utils.ConsoleStorageDir(), "tunnel"))
}

func webhookBaseURLFromDir(tunnelDir string) string {
	if explicit := strings.TrimSpace(os.Getenv("CONSOLE_PUBLIC_URL")); explicit != "" {
		return strings.TrimRight(explicit, "/")
	}
	if info, ok := opentunnel.ReadInfo(tunnelDir); ok && info.Ready && info.URL != "" {
		return strings.TrimRight(info.URL, "/")
	}
	if remote, ok := tunnel.ReadInfo(tunnelDir); ok && remote.Ready && remote.URL != "" {
		return strings.TrimRight(remote.URL, "/")
	}
	return ""
}
