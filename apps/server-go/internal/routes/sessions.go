package routes

import (
	"github.com/gofiber/fiber/v2"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/services"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/types"
)

// First slice of session routes (Phase 1): list, create, get, delete.
func registerSessionRoutes(app *fiber.App, sessions *services.SessionService) {
	h := app.Group("/api/sessions")

	h.Get("/", func(c *fiber.Ctx) error {
		list, err := sessions.List(0)
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		return c.JSON(list)
	})

	h.Post("/", func(c *fiber.Ctx) error {
		var req types.CreateSessionOptions
		if err := c.BodyParser(&req); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid body")
		}
		if req.Cwd == "" || req.ModelID == "" || req.Provider == "" {
			return fiber.NewError(fiber.StatusBadRequest, "cwd, modelId and provider are required")
		}
		header, err := sessions.Create(req)
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		return c.Status(fiber.StatusCreated).JSON(header)
	})

	h.Get("/:id", func(c *fiber.Ctx) error {
		result, err := sessions.Load(c.Params("id"), 0, 0)
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		if result == nil {
			return fiber.NewError(fiber.StatusNotFound, "session not found")
		}
		return c.JSON(result)
	})

	h.Delete("/:id", func(c *fiber.Ctx) error {
		ok, err := sessions.SoftDelete(c.Params("id"))
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		if !ok {
			return fiber.NewError(fiber.StatusNotFound, "session not found")
		}
		return c.SendStatus(fiber.StatusNoContent)
	})
}
