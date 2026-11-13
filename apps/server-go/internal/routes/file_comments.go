// File comment routes: durable comments on lines of a project's files and diffs.
package routes

import (
	"encoding/json"

	"github.com/gofiber/fiber/v2"

	consolev1 "github.com/Adelodunpeter25/console/apps/server-go/internal/gen/console/v1"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/services"
)

func RegisterFileCommentRoutes(app *fiber.App, comments *services.FileCommentService) {
	encode := func(c *fiber.Ctx, msg *consolev1.FileComment) error {
		raw, err := protoMarshal.Marshal(msg)
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"success": false, "error": "encode failed"})
		}
		return c.JSON(fiber.Map{"success": true, "data": json.RawMessage(raw)})
	}
	failFor := func(c *fiber.Ctx, err error) error {
		if err == services.ErrFileCommentNotFound {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"success": false, "error": err.Error()})
		}
		return fail400(c, err)
	}

	// Optional ?path= narrows to one file.
	app.Get("/api/projects/:projectId/comments", func(c *fiber.Ctx) error {
		items, err := comments.List(c.Params("projectId"), c.Query("path"))
		if err != nil {
			return fail400(c, err)
		}
		raw, err := marshalProtoList(items)
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"success": false, "error": "encode failed"})
		}
		return c.JSON(fiber.Map{"success": true, "data": raw})
	})

	app.Post("/api/projects/:projectId/comments", func(c *fiber.Ctx) error {
		var body consolev1.CreateFileCommentRequest
		if err := protoUnmarshal.Unmarshal(c.Body(), &body); err != nil {
			return fail400(c, err)
		}
		created, err := comments.Create(c.Params("projectId"), &body)
		if err != nil {
			return failFor(c, err)
		}
		return encode(c, created)
	})

	app.Patch("/api/comments/:id", func(c *fiber.Ctx) error {
		var body consolev1.UpdateFileCommentRequest
		if err := protoUnmarshal.Unmarshal(c.Body(), &body); err != nil {
			return fail400(c, err)
		}
		updated, err := comments.UpdateText(c.Params("id"), body.Text)
		if err != nil {
			return failFor(c, err)
		}
		return encode(c, updated)
	})

	app.Delete("/api/comments/:id", func(c *fiber.Ctx) error {
		if err := comments.Delete(c.Params("id")); err != nil {
			return failFor(c, err)
		}
		return c.JSON(fiber.Map{"success": true, "data": fiber.Map{}})
	})
}
