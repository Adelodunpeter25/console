// Usage routes (/api/usage, /api/providers/:id/usage). Port of
// apps/server/api/src/routes/usage.ts.
//
// Fourth domain on the shared protobuf schema, and the first trimmed one:
// response payloads are the canonical proto reports (see
// providers/usage/proto.go), not the full internal shapes. The provider map
// keeps JSON null entries for logged-out providers; timestamps encode as
// protojson strings.
package routes

import (
	"encoding/json"

	"github.com/gofiber/fiber/v2"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/providers/usage"
)

func RegisterUsageRoutes(app *fiber.App, svc *usage.Service) {
	app.Get("/api/usage", func(c *fiber.Ctx) error {
		all := svc.GetAllUsage(c.Context())
		data := make(map[string]json.RawMessage, len(all))
		for provider, v := range all {
			raw, err := usage.EncodeReportValue(v)
			if err != nil {
				return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"success": false, "error": "encode failed"})
			}
			data[provider] = raw
		}
		return c.JSON(fiber.Map{"success": true, "data": data})
	})

	app.Get("/api/providers/:id/usage", func(c *fiber.Ctx) error {
		report, err := svc.GetUsage(c.Context(), c.Params("id"))
		if err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"success": false, "error": err.Error()})
		}
		raw, err := usage.EncodeReportValue(report)
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"success": false, "error": "encode failed"})
		}
		return c.JSON(fiber.Map{"success": true, "data": raw})
	})
}
