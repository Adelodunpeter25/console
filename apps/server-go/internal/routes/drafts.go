// Chat draft routes (/api/drafts/:sessionId): per-chat composer text,
// mentions and staged file comments, synced across devices. Clients pull on
// open and push on pause/blur; there is no live event.
package routes

import (
	"encoding/json"
	"time"

	"github.com/gofiber/fiber/v2"

	consolev1 "github.com/Adelodunpeter25/console/apps/server-go/internal/gen/console/v1"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/services"
)

func RegisterDraftRoutes(app *fiber.App, drafts *services.DraftService) {
	// GET returns {draft: null} when the chat has none.
	app.Get("/api/drafts/:sessionId", func(c *fiber.Ctx) error {
		draft, err := drafts.Get(c.Params("sessionId"))
		if err != nil {
			return fail400(c, err)
		}
		if draft == nil {
			return c.JSON(fiber.Map{"success": true, "data": fiber.Map{"draft": nil}})
		}
		raw, err := protoMarshal.Marshal(draft)
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"success": false, "error": "encode failed"})
		}
		return c.JSON(fiber.Map{"success": true, "data": fiber.Map{"draft": json.RawMessage(raw)}})
	})

	app.Put("/api/drafts/:sessionId", func(c *fiber.Ctx) error {
		var body consolev1.ChatDraft
		if err := protoUnmarshal.Unmarshal(c.Body(), &body); err != nil {
			return fail400(c, err)
		}
		body.SessionId = c.Params("sessionId")
		applied, draft, err := drafts.Put(&body)
		if err != nil {
			return fail400(c, err)
		}
		raw, err := protoMarshal.Marshal(&consolev1.PutDraftResponse{Applied: applied, Draft: draft})
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"success": false, "error": "encode failed"})
		}
		return c.JSON(fiber.Map{"success": true, "data": json.RawMessage(raw)})
	})

	app.Delete("/api/drafts/:sessionId", func(c *fiber.Ctx) error {
		if _, _, err := drafts.Put(&consolev1.ChatDraft{
			SessionId: c.Params("sessionId"),
			UpdatedAt: time.Now().UnixMilli(),
		}); err != nil {
			return fail400(c, err)
		}
		return c.JSON(fiber.Map{"success": true, "data": fiber.Map{}})
	})
}
