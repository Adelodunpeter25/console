// Model favorites routes (/api/model-favorites). Port of
// apps/server/api/src/routes/model-favorites.ts. Provider-name validation
// against the registry lands in Phase 3; non-empty is enforced here.
//
// First domain on the shared protobuf schema: request/response payloads are
// built from console.v1 generated types and encoded with protojson. The
// {success, data} envelope is unchanged in this step, and protojson output
// for these flat messages is byte-identical to the old hand-shaped JSON.
package routes

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/gofiber/fiber/v2"
	"google.golang.org/protobuf/encoding/protojson"

	consolev1 "github.com/Adelodunpeter25/console/apps/server-go/internal/gen/console/v1"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/services"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/types"
)

var protoMarshal = protojson.MarshalOptions{}
var protoUnmarshal = protojson.UnmarshalOptions{DiscardUnknown: true}

// favoritesToProto converts service rows to the canonical wire type.
func favoritesToProto(list []types.ModelFavorite) []*consolev1.ModelFavorite {
	out := make([]*consolev1.ModelFavorite, 0, len(list))
	for _, f := range list {
		out = append(out, &consolev1.ModelFavorite{Provider: f.Provider, ModelId: f.ModelID})
	}
	return out
}

// marshalFavoriteList encodes the list payload exactly as before: a JSON
// array of {"provider","modelId"} objects. protojson emits the same bytes
// as encoding/json did for these non-empty flat messages.
func marshalFavoriteList(list []*consolev1.ModelFavorite) (json.RawMessage, error) {
	items := make([]json.RawMessage, 0, len(list))
	for _, f := range list {
		raw, err := protoMarshal.Marshal(f)
		if err != nil {
			return nil, err
		}
		items = append(items, raw)
	}
	return json.Marshal(items)
}

func RegisterFavoriteRoutes(app *fiber.App, favorites *services.FavoriteService) {
	app.Get("/api/model-favorites", func(c *fiber.Ctx) error {
		list, err := favorites.List()
		if err != nil {
			return fail400(c, err)
		}
		data, err := marshalFavoriteList(favoritesToProto(list))
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"success": false, "error": "encode failed"})
		}
		return c.JSON(fiber.Map{"success": true, "data": data})
	})

	app.Put("/api/model-favorites", func(c *fiber.Ctx) error {
		var body consolev1.SetFavoriteRequest
		if err := protoUnmarshal.Unmarshal(c.Body(), &body); err != nil {
			return fail400(c, err)
		}
		provider := strings.TrimSpace(body.Provider)
		modelID := strings.TrimSpace(body.GetModelId())
		if provider == "" || modelID == "" || body.Favorite == nil {
			return fail400(c, fmt.Errorf("provider, modelId, and favorite are required."))
		}
		if err := favorites.Set(types.ModelFavorite{Provider: provider, ModelID: modelID}, *body.Favorite); err != nil {
			return fail400(c, err)
		}
		data, err := protoMarshal.Marshal(&consolev1.SetFavoriteResponse{
			Provider: provider, ModelId: modelID, Favorite: *body.Favorite,
		})
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"success": false, "error": "encode failed"})
		}
		return c.JSON(fiber.Map{"success": true, "data": json.RawMessage(data)})
	})
}
