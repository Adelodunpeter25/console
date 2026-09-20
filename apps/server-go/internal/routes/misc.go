// Config + notifications routes. Ports of apps/server/api/src/routes/
// config.ts and notifications.ts.
package routes

import (
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/services"
)

var approvalModes = []fiber.Map{
	{"value": "always-ask", "label": "Normal", "description": "Ask for every action"},
	{"value": "accept-edits", "label": "Accept Edits", "description": "Auto-approve file edits"},
	{"value": "plan-mode", "label": "Plan Mode", "description": "Plan only, no execution"},
	{"value": "full-access", "label": "Bypass Permissions", "description": "Run everything without asking"},
}

func registerMiscRoutes(app *fiber.App, notifications *services.NotificationService) {
	app.Get("/api/config/approval-modes", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{"success": true, "data": approvalModes})
	})

	app.Get("/api/notifications/stream", func(c *fiber.Ctx) error {
		return streamSSE(c, func(sse *sseStream) {
			events := notifications.Subscribe()
			defer notifications.Unsubscribe(events)
			ticker := time.NewTicker(15 * time.Second)
			defer ticker.Stop()
			for {
				select {
				case event := <-events:
					if err := sse.Send("notification", mustJSON(event)); err != nil {
						return
					}
				case <-ticker.C:
					if err := sse.Send("ping", ""); err != nil {
						return
					}
				}
			}
		})
	})
}
