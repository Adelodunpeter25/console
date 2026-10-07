// Settings routes (/api/settings).
//
// Second domain on the shared protobuf schema. Response payloads are built
// from console.v1 generated types and encoded with protojson; the
// {success, data} envelope is unchanged and response bytes are identical.
//
// The PATCH request body is intentionally hand-rolled (see
// proto/console/v1/settings.proto): null clears a role while a missing key
// leaves it untouched, which no proto3 map value can express.
package routes

import (
	"encoding/json"
	"fmt"

	"github.com/gofiber/fiber/v2"

	consolev1 "github.com/Adelodunpeter25/console/apps/server-go/internal/gen/console/v1"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/services"
)

// settingsToProto converts the service result to the canonical wire type.
// Empty roles become "" and are omitted by protojson, so only set roles
// appear — exactly like the old map encoding. The message is always
// populated, so an empty mapping still encodes as {"modelRoles":{}}.
func settingsToProto(s services.Settings) *consolev1.ConsoleSettings {
	return &consolev1.ConsoleSettings{ModelRoles: &consolev1.ModelRoleMapping{
		Vision: strptrOrNil(s.ModelRoles["vision"]),
		Smol:   strptrOrNil(s.ModelRoles["smol"]),
	}}
}

func strptrOrNil(v string) *string {
	if v == "" {
		return nil
	}
	return &v
}

func sendSettings(c *fiber.Ctx, s services.Settings) error {
	raw, err := protoMarshal.Marshal(settingsToProto(s))
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"success": false, "error": "encode failed"})
	}
	return c.JSON(fiber.Map{"success": true, "data": json.RawMessage(raw)})
}

func RegisterSettingsRoutes(app *fiber.App, settings *services.SettingsService) {
	app.Get("/api/settings", func(c *fiber.Ctx) error {
		return sendSettings(c, settings.Load())
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
		return sendSettings(c, result)
	})
}
