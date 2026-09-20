// Project management routes (/api/projects/*). Port of
// apps/server/api/src/routes/projects.ts.
package routes

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/gofiber/fiber/v2"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/services"
)

func registerProjectRoutes(app *fiber.App, projects *services.ProjectService) {
	app.Get("/api/projects", func(c *fiber.Ctx) error {
		list, err := projects.List()
		if err != nil {
			return fail400(c, err)
		}
		return c.JSON(fiber.Map{"success": true, "data": list})
	})

	app.Post("/api/projects", func(c *fiber.Ctx) error {
		var body struct {
			Path string `json:"path"`
		}
		if err := c.BodyParser(&body); err != nil || body.Path == "" {
			return fail400(c, fmt.Errorf("Field 'path' is required."))
		}
		abs, err := filepath.Abs(body.Path)
		if err != nil {
			return fail400(c, err)
		}
		info, err := os.Stat(abs)
		if err != nil || !info.IsDir() {
			return fail400(c, fmt.Errorf("Directory does not exist: %s", abs))
		}
		project, err := projects.Create(services.CreateProjectOptions{
			Name: strings.TrimSpace(filepath.Base(abs)), Dir: abs,
		})
		if err != nil {
			return fail400(c, err)
		}
		return c.JSON(fiber.Map{"success": true, "data": project})
	})

	app.Delete("/api/projects/:id", func(c *fiber.Ctx) error {
		deleted, err := projects.Delete(c.Params("id"))
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"success": false, "error": err.Error()})
		}
		if !deleted {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
				"success": false, "error": fmt.Sprintf("Project '%s' not found.", c.Params("id")),
			})
		}
		return c.JSON(fiber.Map{"success": true, "data": fiber.Map{"id": c.Params("id"), "deleted": true}})
	})
}
