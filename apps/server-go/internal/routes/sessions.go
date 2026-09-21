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

	// GET /api/sessions/:id/todos — persisted todos for a session.
	h.Get("/:id/todos", func(c *fiber.Ctx) error {
		todos, err := sessions.GetSessionTodos(c.Params("id"))
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		return c.JSON(fiber.Map{"success": true, "data": todos})
	})

	// GET /api/sessions/:id/changes — session file changes with optional turn filter.
	h.Get("/:id/changes", func(c *fiber.Ctx) error {
		sessionID := c.Params("id")
		turnIndex := -1 // -1 means all turns
		if turnStr := c.Query("turnIndex"); turnStr != "" {
			turnIndex = c.QueryInt("turnIndex", 0)
		}
		changes, err := sessions.GetSessionFileChanges(sessionID, turnIndex)
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		return c.JSON(fiber.Map{"success": true, "data": changes})
	})

	// GET /api/sessions/:id/changes/diff — raw diff text for a specific file change.
	h.Get("/:id/changes/diff", func(c *fiber.Ctx) error {
		sessionID := c.Params("id")
		path := c.Query("path")
		if path == "" {
			return fiber.NewError(fiber.StatusBadRequest, "path query parameter is required")
		}
		turnIndex := -1
		if turnStr := c.Query("turnIndex"); turnStr != "" {
			turnIndex = c.QueryInt("turnIndex", 0)
		}
		changes, err := sessions.GetSessionFileChanges(sessionID, turnIndex)
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		for _, change := range changes {
			if change.Path == path && change.DiffText != nil {
				return c.JSON(fiber.Map{"success": true, "data": fiber.Map{"diffText": *change.DiffText}})
			}
		}
		return fiber.NewError(fiber.StatusNotFound, "file change not found")
	})
}
