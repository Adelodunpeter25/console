// Binary integration status (/api/integrations). Read-only by design: the
// page tells the user what to install and where, and the install itself
// happens on the box, outside this API.
package routes

import (
	"encoding/json"

	"github.com/gofiber/fiber/v2"

	consolev1 "github.com/Adelodunpeter25/console/apps/server-go/internal/gen/console/v1"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/services"
)

// RegisterIntegrationRoutes wires the /api/integrations routes onto an app.
// Exported so route tests can build a Fiber app with only these routes,
// mirroring RegisterAuthRoutes.
func RegisterIntegrationRoutes(app *fiber.App) {
	app.Get("/api/integrations", func(c *fiber.Ctx) error {
		found := services.ListIntegrations()
		items := make([]*consolev1.IntegrationStatus, 0, len(found))
		for _, info := range found {
			items = append(items, integrationToProto(info))
		}
		raw, err := protoMarshal.Marshal(&consolev1.IntegrationsResponse{Integrations: items})
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"success": false, "error": "encode failed"})
		}
		return c.JSON(fiber.Map{"success": true, "data": json.RawMessage(raw)})
	})
}

// integrationToProto converts the detector's plain shape to the wire type.
// Version and path stay absent (not empty) when the probe could not supply
// them, so clients read "unknown" rather than "".
func integrationToProto(info services.IntegrationInfo) *consolev1.IntegrationStatus {
	// Local copy: the installed flag has explicit presence, and taking the
	// address of a range variable would alias every row to the last one.
	installed := info.Installed
	out := &consolev1.IntegrationStatus{
		Id:          info.ID,
		Name:        info.Name,
		Description: info.Description,
		Installed:   &installed,
		InstallUrl:  info.InstallURL,
		InstallHint: info.InstallHint,
	}
	if info.Version != "" {
		out.Version = &info.Version
	}
	if info.Path != "" {
		out.BinaryPath = &info.Path
	}
	return out
}
