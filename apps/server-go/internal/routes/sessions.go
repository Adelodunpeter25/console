package routes

import (
	"github.com/gofiber/fiber/v2"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/services"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/types"
)

// Session routes with envelope responses matching desktop expectations.
func registerSessionRoutes(app *fiber.App, sessions *services.SessionService) {
	h := app.Group("/api/sessions")

	h.Get("/", func(c *fiber.Ctx) error {
		minUpdatedAt := int64(0)
		if onlyDeleted := c.Query("onlyDeleted"); onlyDeleted == "true" {
			// For trash view, we'd need to implement this filter
			// For now, return empty list for deleted-only requests
			return c.JSON(fiber.Map{"success": true, "data": []types.SessionHeader{}})
		}
		// Support cwd and projectId filters (basic implementation)
		list, err := sessions.List(minUpdatedAt)
		if err != nil {
			return c.JSON(fiber.Map{"success": false, "error": err.Error()})
		}
		return c.JSON(fiber.Map{"success": true, "data": list})
	})

	h.Post("/", func(c *fiber.Ctx) error {
		var req types.CreateSessionOptions
		if err := c.BodyParser(&req); err != nil {
			return c.JSON(fiber.Map{"success": false, "error": "invalid body"})
		}
		// Make fields optional - use defaults if not provided
		if req.Cwd == "" {
			req.Cwd = "." // Default to current directory
		}
		if req.ModelID == "" {
			req.ModelID = "default" // Default model
		}
		if req.Provider == "" {
			req.Provider = "anthropic" // Default provider
		}
		header, err := sessions.Create(req)
		if err != nil {
			return c.JSON(fiber.Map{"success": false, "error": err.Error()})
		}
		return c.Status(fiber.StatusOK).JSON(fiber.Map{"success": true, "data": header})
	})

	h.Get("/:id", func(c *fiber.Ctx) error {
		result, err := sessions.Load(c.Params("id"), 0, 0)
		if err != nil {
			return c.JSON(fiber.Map{"success": false, "error": err.Error()})
		}
		if result == nil {
			return c.JSON(fiber.Map{"success": false, "error": "session not found"})
		}
		return c.JSON(fiber.Map{"success": true, "data": result})
	})

	h.Patch("/:id", func(c *fiber.Ctx) error {
		// Rename/model switch support
		var req struct {
			Title    *string `json:"title,omitempty"`
			ModelID  *string `json:"modelId,omitempty"`
			Provider *string `json:"provider,omitempty"`
		}
		if err := c.BodyParser(&req); err != nil {
			return c.JSON(fiber.Map{"success": false, "error": "invalid body"})
		}
		if req.Title != nil {
			if err := sessions.UpdateTitle(c.Params("id"), *req.Title); err != nil {
				return c.JSON(fiber.Map{"success": false, "error": err.Error()})
			}
		}
		// Model/provider switching would need additional service methods
		return c.JSON(fiber.Map{"success": true, "data": fiber.Map{}})
	})

	h.Post("/:id/restore", func(c *fiber.Ctx) error {
		// Restore from trash - would need service method
		return c.JSON(fiber.Map{"success": false, "error": "not implemented"})
	})

	h.Delete("/:id", func(c *fiber.Ctx) error {
		ok, err := sessions.SoftDelete(c.Params("id"))
		if err != nil {
			return c.JSON(fiber.Map{"success": false, "error": err.Error()})
		}
		if !ok {
			return c.JSON(fiber.Map{"success": false, "error": "session not found"})
		}
		return c.JSON(fiber.Map{"success": true, "data": fiber.Map{}})
	})

	h.Delete("/:id/permanent", func(c *fiber.Ctx) error {
		// Permanent delete - would need service method
		return c.JSON(fiber.Map{"success": false, "error": "not implemented"})
	})

	h.Get("/:id/subagents", func(c *fiber.Ctx) error {
		// Subagents endpoint - would need service method
		return c.JSON(fiber.Map{"success": true, "data": []any{}})
	})

	// GET /api/sessions/:id/todos — persisted todos for a session.
	h.Get("/:id/todos", func(c *fiber.Ctx) error {
		todos, err := sessions.GetSessionTodos(c.Params("id"))
		if err != nil {
			return c.JSON(fiber.Map{"success": false, "error": err.Error()})
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
			return c.JSON(fiber.Map{"success": false, "error": err.Error()})
		}
		return c.JSON(fiber.Map{"success": true, "data": changes})
	})

	// GET /api/sessions/:id/changes/diff — raw diff text for a specific file change.
	h.Get("/:id/changes/diff", func(c *fiber.Ctx) error {
		sessionID := c.Params("id")
		path := c.Query("path")
		if path == "" {
			return c.JSON(fiber.Map{"success": false, "error": "path query parameter is required"})
		}
		turnIndex := -1
		if turnStr := c.Query("turnIndex"); turnStr != "" {
			turnIndex = c.QueryInt("turnIndex", 0)
		}
		changes, err := sessions.GetSessionFileChanges(sessionID, turnIndex)
		if err != nil {
			return c.JSON(fiber.Map{"success": false, "error": err.Error()})
		}
		for _, change := range changes {
			if change.Path == path && change.DiffText != nil {
				return c.JSON(fiber.Map{"success": true, "data": fiber.Map{"diffText": *change.DiffText}})
			}
		}
		return c.JSON(fiber.Map{"success": false, "error": "file change not found"})
	})
}
