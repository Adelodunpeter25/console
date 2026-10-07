// Project management routes (/api/projects/*).
//
// Third domain on the shared protobuf schema. Response payloads are built
// from console.v1 generated types; the {success, data} envelope is
// unchanged. Timestamps now encode as protojson strings (see
// proto/console/v1/project.proto) — the first wire change, migrated on all
// three clients in the same step.
package routes

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/gofiber/fiber/v2"

	consolev1 "github.com/Adelodunpeter25/console/apps/server-go/internal/gen/console/v1"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/services"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/types"
)

// projectToProto converts a service row to the canonical wire type.
func projectToProto(p types.ProjectInfo) *consolev1.ProjectInfo {
	return &consolev1.ProjectInfo{
		Id: p.ID, Name: p.Name, Path: p.Path,
		CreatedAt: p.CreatedAt, UpdatedAt: p.UpdatedAt,
	}
}

func projectsToProto(list []types.ProjectInfo) []*consolev1.ProjectInfo {
	out := make([]*consolev1.ProjectInfo, 0, len(list))
	for _, p := range list {
		out = append(out, projectToProto(p))
	}
	return out
}

func sendProject(c *fiber.Ctx, p types.ProjectInfo) error {
	raw, err := protoMarshal.Marshal(projectToProto(p))
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"success": false, "error": "encode failed"})
	}
	return c.JSON(fiber.Map{"success": true, "data": json.RawMessage(raw)})
}

func RegisterProjectRoutes(app *fiber.App, projects *services.ProjectService) {
	app.Get("/api/projects", func(c *fiber.Ctx) error {
		list, err := projects.List()
		if err != nil {
			return fail400(c, err)
		}
		data, err := marshalProtoList(projectsToProto(list))
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"success": false, "error": "encode failed"})
		}
		return c.JSON(fiber.Map{"success": true, "data": data})
	})

	app.Post("/api/projects", func(c *fiber.Ctx) error {
		var body consolev1.CreateProjectRequest
		if err := protoUnmarshal.Unmarshal(c.Body(), &body); err != nil || body.Path == "" {
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
		return sendProject(c, project)
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
		raw, err := protoMarshal.Marshal(&consolev1.DeleteProjectResponse{Id: c.Params("id"), Deleted: true})
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"success": false, "error": "encode failed"})
		}
		return c.JSON(fiber.Map{"success": true, "data": json.RawMessage(raw)})
	})
}
