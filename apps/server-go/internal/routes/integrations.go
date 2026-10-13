// Binary integration status (/api/integrations). Read-only by design: the
// page tells the user what to install and where, and the install itself
// happens on the box, outside this API.
package routes

import (
	"encoding/json"
	"errors"

	"github.com/gofiber/fiber/v2"

	consolev1 "github.com/Adelodunpeter25/console/apps/server-go/internal/gen/console/v1"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/services"
)

// RegisterIntegrationRoutes wires the /api/integrations routes onto an app.
// Exported so route tests can build a Fiber app with only these routes,
// mirroring RegisterAuthRoutes.
func RegisterIntegrationRoutes(app *fiber.App) {
	app.Get("/api/integrations", func(c *fiber.Ctx) error {
		raw, err := marshalIntegrations()
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"success": false, "error": "encode failed"})
		}
		return c.JSON(fiber.Map{"success": true, "data": json.RawMessage(raw)})
	})

	// Install the managed gh CLI into the storage bin dir. Shown only when
	// no `gh` resolves; the call itself re-checks and no-ops to current
	// status when one does. Answers the full list so clients just replace
	// state. Failures are 502 with the reason verbatim (unreachable
	// github.com, checksum mismatch) or 400 for an unsupported platform —
	// never silent, since a quiet button is worse than none.
	app.Post("/api/integrations/gh/install", func(c *fiber.Ctx) error {
		if _, err := services.InstallGhCLI(c.Context()); err != nil {
			status := fiber.StatusBadGateway
			if errors.Is(err, services.ErrGhPlatformUnsupported) {
				status = fiber.StatusBadRequest
			}
			return c.Status(status).JSON(fiber.Map{"success": false, "error": err.Error()})
		}
		raw, err := marshalIntegrations()
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"success": false, "error": "encode failed"})
		}
		return c.JSON(fiber.Map{"success": true, "data": json.RawMessage(raw)})
	})
}

func marshalIntegrations() (json.RawMessage, error) {
	found := services.ListIntegrations()
	items := make([]*consolev1.IntegrationStatus, 0, len(found))
	for _, info := range found {
		items = append(items, integrationToProto(info))
	}
	return protoMarshal.Marshal(&consolev1.IntegrationsResponse{Integrations: items})
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
