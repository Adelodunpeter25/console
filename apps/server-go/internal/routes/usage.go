// Usage routes (/api/usage, /api/providers/:id/usage). Port of
// apps/server/api/src/routes/usage.ts.
package routes

import (
	"github.com/gofiber/fiber/v2"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/providers/usage"
)

func registerUsageRoutes(app *fiber.App, svc *usage.Service) {
	app.Get("/api/usage", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{"success": true, "data": svc.GetAllUsage(c.Context())})
	})

	app.Get("/api/providers/:id/usage", func(c *fiber.Ctx) error {
		report, err := svc.GetUsage(c.Context(), c.Params("id"))
		if err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"success": false, "error": err.Error()})
		}
		return c.JSON(fiber.Map{"success": true, "data": report})
	})
}
