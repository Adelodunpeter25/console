// Provider & model catalog routes: list providers and fetch dynamic
// models. Only implemented providers serve live data; the rest answer 501.
package routes

import (
	"encoding/json"
	"errors"
	"os"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"google.golang.org/protobuf/proto"

	consolev1 "github.com/Adelodunpeter25/console/apps/server-go/internal/gen/console/v1"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/providers"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/providers/apikeys"
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
	RegisterProviderCredentialRoutes(app)
	app.Get("/api/providers", func(c *fiber.Ctx) error {
		entries := providers.ListProvidersWithContext(c.Context())
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
			// Go has no offline fallback, so the error is real information:
			// most often it is ErrNoAPIKey, which tells the user exactly what
			// to do. Swallowing it would render an empty picker instead.
			models, err := providers.OpenCodeModels(c.Context())
			if err != nil {
				return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
					"success": false,
					"error":   err.Error(),
				})
			}
			return encodeModelsResponse(c, id, withFavorites(models, favorites))
		default:
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"success": false, "error": "Invalid provider '" + id + "'."})
		}
	})
}

// registerProviderCredentialRoutes wires the generic pasted-key endpoints.
//
// These live under /api/providers rather than /api/auth because a key is a
// property of a provider, not another session: the catalog already names the
// providers that accept one (auth_method "api-key"), and this reads that same
// list instead of duplicating it.
//
// Nothing here returns a key — not the value, not a prefix, not a length. A
// client that needs to know whether a credential exists asks status; a client
// that wants to change it sends a new one.
func RegisterProviderCredentialRoutes(app *fiber.App) {
	app.Get("/api/providers/credentials", func(c *fiber.Ctx) error {
		statuses := map[string]*consolev1.ProviderCredentialStatus{}
		for _, entry := range providers.ListProviders() {
			if entry.AuthMethod != providers.AuthMethodAPIKey {
				continue
			}
			statuses[entry.Name] = providerCredentialStatus(entry.Name)
		}
		raw, err := protoMarshal.Marshal(&consolev1.ProviderCredentialStatuses{Providers: statuses})
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"success": false, "error": "encode failed"})
		}
		return c.JSON(fiber.Map{"success": true, "data": json.RawMessage(raw)})
	})

	app.Put("/api/providers/:id/credential", func(c *fiber.Ctx) error {
		id := c.Params("id")
		if !providers.SupportsAPIKey(id) {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"success": false, "error": "Provider '" + id + "' does not accept an API key."})
		}
		var req struct {
			APIKey string `json:"apiKey"`
		}
		if err := c.BodyParser(&req); err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"success": false, "error": "Invalid request body."})
		}
		if err := apikeys.Set(id, req.APIKey); err != nil {
			// A blank key is a client mistake worth naming, not a 500.
			if errors.Is(err, apikeys.ErrEmptyKey) {
				return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"success": false, "error": err.Error()})
			}
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"success": false, "error": err.Error()})
		}
		return respondProviderCredential(c, id, false)
	})

	app.Delete("/api/providers/:id/credential", func(c *fiber.Ctx) error {
		id := c.Params("id")
		if !providers.SupportsAPIKey(id) {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"success": false, "error": "Provider '" + id + "' does not accept an API key."})
		}
		if err := apikeys.Clear(id); err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"success": false, "error": err.Error()})
		}
		return respondProviderCredential(c, id, true)
	})
}

// providerCredentialStatus reports one provider's key state without revealing
// it. `source` distinguishes an environment override from a stored key, which
// matters because clearing a stored key does not clear an env one — a client
// that hid its field after a "remove" would be misreporting which key is live.
func providerCredentialStatus(provider string) *consolev1.ProviderCredentialStatus {
	entry, stored := apikeys.EntryFor(provider)
	if !stored {
		// Both fields stay present even when there is no key: protojson drops
		// an implicit-presence false, and a client must be able to tell "no key
		// stored" from "the server did not report one".
		status := &consolev1.ProviderCredentialStatus{Configured: proto.Bool(false)}
		if src := envKeySource(provider); src != "" {
			status.Source = proto.String(src)
			status.Configured = proto.Bool(true)
		}
		return status
	}
	return &consolev1.ProviderCredentialStatus{
		Configured: proto.Bool(true),
		Source:     proto.String(apikeySourceStored),
		UpdatedAt:  proto.String(entry.UpdatedAt.UTC().Format(time.RFC3339)),
	}
}

// respondProviderCredential answers a write with the provider's new state.
func respondProviderCredential(c *fiber.Ctx, provider string, removed bool) error {
	status := providerCredentialStatus(provider)
	raw, err := protoMarshal.Marshal(&consolev1.SetProviderCredentialResponse{
		Provider:   provider,
		Configured: status.Configured,
		Source:     status.Source,
		Removed:    removed,
	})
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"success": false, "error": "encode failed"})
	}
	return c.JSON(fiber.Map{"success": true, "data": json.RawMessage(raw)})
}

const (
	apikeySourceStored = "stored"
	apikeySourceEnv    = "env"
)

// envKeySource returns "env" when an environment variable is supplying this
// provider's key, or "" when none is. Only OpenCode has one today; keeping the
// mapping here means the status route does not switch on provider ids.
func envKeySource(provider string) string {
	if provider == "opencode" && strings.TrimSpace(os.Getenv("OPENCODE_API_KEY")) != "" {
		return apikeySourceEnv
	}
	return ""
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
