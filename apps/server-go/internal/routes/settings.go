// Settings routes (/api/settings). Port of apps/server/api/src/routes/settings.ts.
package routes

import (
	"encoding/json"
	"fmt"

	"github.com/gofiber/fiber/v2"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/services"
)

func registerSettingsRoutes(app *fiber.App, settings *services.SettingsService) {
	app.Get("/api/settings", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{"success": true, "data": settings.Load()})
	})

	app.Patch("/api/settings", func(c *fiber.Ctx) error {
		var raw map[string]json.RawMessage
		if err := json.Unmarshal(c.Body(), &raw); err != nil {
			return fail400(c, fmt.Errorf("Invalid body."))
		}
		modelRolesRaw, ok := raw["modelRoles"]
		if !ok {
			return fail400(c, fmt.Errorf("modelRoles is required."))
		}
		var modelRoles map[string]json.RawMessage
		if err := json.Unmarshal(modelRolesRaw, &modelRoles); err != nil {
			return fail400(c, fmt.Errorf("modelRoles must be an object."))
		}
		patch := map[string]*string{}
		for role, value := range modelRoles {
			if !services.IsModelRole(role) {
				return fail400(c, fmt.Errorf("Invalid model role '%s'.", role))
			}
			var model *string
			if err := json.Unmarshal(value, &model); err != nil {
				return fail400(c, fmt.Errorf("Value for model role '%s' must be a string.", role))
			}
			patch[role] = model
		}
		result, err := settings.Patch(patch)
		if err != nil {
			return fail400(c, err)
		}
		return c.JSON(fiber.Map{"success": true, "data": result})
	})
}
