// Usage routes (/api/usage, /api/providers/:id/usage). Port of
// apps/server/api/src/routes/usage.ts; reports are null until the provider
// quota fetchers land in Phase 3.
package routes

import (
	"fmt"

	"github.com/gofiber/fiber/v2"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/services"
)

func registerUsageRoutes(app *fiber.App, usage *services.UsageService) {
	app.Get("/api/usage", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{"success": true, "data": usage.GetAllUsage()})
	})

	app.Get("/api/providers/:id/usage", func(c *fiber.Ctx) error {
		report, err := usage.GetUsage(c.Params("id"))
		if err != nil {
			return fail400(c, fmt.Errorf("Invalid provider '%s' for usage.", c.Params("id")))
		}
		return c.JSON(fiber.Map{"success": true, "data": report})
	})
}
