// API version gate (shared-protobuf-schema §6). Single global integer,
// exact match: clients compare against the version they generated against.
package routes

import (
	"github.com/gofiber/fiber/v2"
	"google.golang.org/protobuf/encoding/protojson"

	consolev1 "github.com/Adelodunpeter25/console/apps/server-go/internal/gen/console/v1"
)

// ApiVersion is the current wire version. Bump on any migrated domain.
const ApiVersion uint32 = 1

func RegisterVersionRoutes(app *fiber.App) {
	app.Get("/api/version", func(c *fiber.Ctx) error {
		msg := &consolev1.GetApiVersionResponse{ApiVersion: ApiVersion}
		buf, err := protojson.Marshal(msg)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"success": false, "error": "version encode failed"})
		}
		c.Set("Content-Type", "application/json")
		return c.Send(buf)
	})
}
