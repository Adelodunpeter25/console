// Provider & model catalog routes: list providers and fetch dynamic
// models. Only implemented providers serve live data; the rest answer 501.
package routes

import (
	"encoding/json"

	"github.com/gofiber/fiber/v2"

	consolev1 "github.com/Adelodunpeter25/console/apps/server-go/internal/gen/console/v1"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/providers"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/services"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/types"
)

// modelToProto converts a catalog model to the canonical wire type. Counts
// narrow to uint32 and stay JSON numbers; thinking levels stay plain strings
// and omit when empty.
func modelToProto(m types.Model) *consolev1.Model {
	out := &consolev1.Model{
		Id: m.ID, Provider: m.Provider, ContextWindow: uint32(m.ContextWindow),
		SupportsImages: m.SupportsImages,
	}
	out.SupportedThinkingLevels = append(out.SupportedThinkingLevels, m.ThinkingLevels...)
	if m.DefaultThinking != "" {
		out.DefaultThinkingLevel = &m.DefaultThinking
	}
	return out
}

func modelsToProto(models []types.Model) []*consolev1.Model {
	out := make([]*consolev1.Model, 0, len(models))
	for _, m := range models {
		out = append(out, modelToProto(m))
	}
	return out
}

// providerEntryToProto converts one catalog entry. An empty model list
// omits the key instead of marshalling null.
func providerEntryToProto(e types.ProviderEntry) *consolev1.ProviderCatalogEntry {
	return &consolev1.ProviderCatalogEntry{
		Name: e.Name, DisplayName: e.DisplayName, Description: e.Description,
		Models: modelsToProto(e.Models), AuthMethod: e.AuthMethod,
	}
}

// encodeModelsResponse serves the {provider, models} payload shared by
// every dynamic provider route.
func encodeModelsResponse(c *fiber.Ctx, provider string, models []types.Model) error {
	raw, err := protoMarshal.Marshal(&consolev1.ProviderModelsResponse{
		Provider: provider, Models: modelsToProto(models),
	})
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"success": false, "error": "encode failed"})
	}
	return c.JSON(fiber.Map{"success": true, "data": json.RawMessage(raw)})
}

func registerProviderRoutes(app *fiber.App, favorites *services.FavoriteService) {
	app.Get("/api/providers", func(c *fiber.Ctx) error {
		entries := providers.ListProviders()
		favs, err := favorites.List()
		if err == nil {
			for i, entry := range entries {
				entries[i].Models = providers.SortModelsByFavorites(entry.Models, favs)
			}
		}
		items := make([]*consolev1.ProviderCatalogEntry, 0, len(entries))
		for _, entry := range entries {
			items = append(items, providerEntryToProto(entry))
		}
		data, err := marshalProtoList(items)
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"success": false, "error": "encode failed"})
		}
		return c.JSON(fiber.Map{"success": true, "data": data})
	})

	app.Get("/api/providers/:id/models", func(c *fiber.Ctx) error {
		id := c.Params("id")
		switch id {
		case "codex":
			return encodeModelsResponse(c, id, withFavorites(providers.CodexModels(c.Context()), favorites))
		case "claude":
			return encodeModelsResponse(c, id, withFavorites(providers.ClaudeModels(c.Context()), favorites))
		case "antigravity":
			return encodeModelsResponse(c, id, withFavorites(providers.AntigravityModels(c.Context()), favorites))
		case "opencode":
			return encodeModelsResponse(c, id, withFavorites(providers.OpenCodeModels(c.Context()), favorites))
		default:
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"success": false, "error": "Invalid provider '" + id + "'."})
		}
	})
}

// withFavorites orders favorited models first when the favorite list
// loads; a favorites read failure leaves the provider order untouched.
func withFavorites(models []types.Model, favorites *services.FavoriteService) []types.Model {
	favs, err := favorites.List()
	if err != nil {
		return models
	}
	return providers.SortModelsByFavorites(models, favs)
}
