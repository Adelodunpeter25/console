// Model favorites routes (/api/model-favorites). Port of
// apps/server/api/src/routes/model-favorites.ts. Provider-name validation
// against the registry lands in Phase 3; non-empty is enforced here.
package routes

import (
	"fmt"
	"strings"

	"github.com/gofiber/fiber/v2"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/services"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/types"
)

func registerFavoriteRoutes(app *fiber.App, favorites *services.FavoriteService) {
	app.Get("/api/model-favorites", func(c *fiber.Ctx) error {
		list, err := favorites.List()
		if err != nil {
			return fail400(c, err)
		}
		return c.JSON(fiber.Map{"success": true, "data": list})
	})

	app.Put("/api/model-favorites", func(c *fiber.Ctx) error {
		var body struct {
			Provider string `json:"provider"`
			ModelID  string `json:"modelId"`
			Favorite *bool  `json:"favorite"`
		}
		if err := c.BodyParser(&body); err != nil {
			return fail400(c, err)
		}
		provider := strings.TrimSpace(body.Provider)
		modelID := strings.TrimSpace(body.ModelID)
		if provider == "" || modelID == "" || body.Favorite == nil {
			return fail400(c, fmt.Errorf("provider, modelId, and favorite are required."))
		}
		if err := favorites.Set(types.ModelFavorite{Provider: provider, ModelID: modelID}, *body.Favorite); err != nil {
			return fail400(c, err)
		}
		return c.JSON(fiber.Map{"success": true, "data": fiber.Map{
			"provider": provider, "modelId": modelID, "favorite": *body.Favorite,
		}})
	})
}
