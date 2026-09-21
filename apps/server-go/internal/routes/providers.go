// Provider & model catalog routes. Port of
// apps/server/api/src/routes/providers.ts: list providers and fetch
// dynamic models. Only implemented providers serve live data; the rest
// answer 501 until their Go ports land.
package routes

import (
	"github.com/gofiber/fiber/v2"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/providers"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/services"
)

func registerProviderRoutes(app *fiber.App, favorites *services.FavoriteService) {
	app.Get("/api/providers", func(c *fiber.Ctx) error {
		entries := providers.ListProviders()
		favs, err := favorites.List()
		if err == nil {
			for i, entry := range entries {
				entries[i].Models = providers.SortModelsByFavorites(entry.Models, favs)
			}
		}
		return c.JSON(fiber.Map{"success": true, "data": entries})
	})

	app.Get("/api/providers/:id/models", func(c *fiber.Ctx) error {
		id := c.Params("id")
		switch id {
		case "codex":
			models := providers.CodexModels(c.Context())
			if favs, err := favorites.List(); err == nil {
				models = providers.SortModelsByFavorites(models, favs)
			}
			return c.JSON(fiber.Map{"success": true, "data": fiber.Map{"provider": id, "models": models}})
		case "claude":
			models := providers.ClaudeModels(c.Context())
			if favs, err := favorites.List(); err == nil {
				models = providers.SortModelsByFavorites(models, favs)
			}
			return c.JSON(fiber.Map{"success": true, "data": fiber.Map{"provider": id, "models": models}})
		case "antigravity":
			return c.Status(fiber.StatusNotImplemented).JSON(fiber.Map{"success": false, "error": "Model discovery for '" + id + "' is not supported by the Go server yet."})
		default:
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"success": false, "error": "Invalid provider '" + id + "'."})
		}
	})
}
