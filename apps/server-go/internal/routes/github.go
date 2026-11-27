// GitHub routes (/api/github/*). Today: the repository list the mobile and
// desktop project pickers show in their GitHub section. Read-only over the
// stored PAT — the same credential git clone uses.
package routes

import (
	"errors"

	"github.com/gofiber/fiber/v2"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/providers/github"
)

func RegisterGitHubRoutes(app *fiber.App) {
	h := app.Group("/api/github")

	// GET /api/github/repos — the connected account's repositories, most
	// recently pushed first. 409 with ErrNotConnected/ErrInvalidToken lets
	// the picker point at Accounts (the same mapping failProjectBootstrap
	// uses for clones).
	h.Get("/repos", func(c *fiber.Ctx) error {
		// A nil client lets ListRepos apply its default timeout; tests point
		// APIBase at an httptest server, so no injection is needed here.
		repos, err := github.ListRepos(c.Context(), nil)
		switch {
		case err == nil:
		case errors.Is(err, github.ErrNotConnected), errors.Is(err, github.ErrInvalidToken):
			return c.Status(fiber.StatusConflict).JSON(fiber.Map{"success": false, "error": err.Error()})
		default:
			return c.Status(fiber.StatusBadGateway).JSON(fiber.Map{"success": false, "error": err.Error()})
		}
		return c.JSON(fiber.Map{"success": true, "data": repos})
	})
}
