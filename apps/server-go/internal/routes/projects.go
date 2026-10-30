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
	"errors"
	"fmt"
	projectssvc "github.com/Adelodunpeter25/console/apps/server-go/internal/services/projects"
	"os"
	"path/filepath"
	"strings"

	"github.com/gofiber/fiber/v2"

	consolev1 "github.com/Adelodunpeter25/console/apps/server-go/internal/gen/console/v1"
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

// failProjectBootstrap maps a bootstrap failure to a status the dialog can
// act on. The split that matters is ErrGitHubNotConnected getting its own
// 409: the client shows "connect in Accounts" for it, whereas a 400 would be
// read as a typo in the URL field and the user would edit the wrong thing.
// Everything else is a plain 400 — the error text is already written to be
// displayed verbatim.
func failProjectBootstrap(c *fiber.Ctx, err error) error {
	if errors.Is(err, projectssvc.ErrGitHubNotConnected) {
		return c.Status(fiber.StatusConflict).JSON(fiber.Map{"success": false, "error": err.Error()})
	}
	return fail400(c, err)
}

func RegisterProjectRoutes(app *fiber.App, projects *projectssvc.ProjectService, bootstrap *projectssvc.ProjectBootstrapService) {
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
		project, err := projects.Create(projectssvc.CreateProjectOptions{
			Name: strings.TrimSpace(filepath.Base(abs)), Dir: abs,
		})
		if err != nil {
			return fail400(c, err)
		}
		return sendProject(c, project)
	})

	// POST /api/projects/init — create a blank project: the server makes
	// <parentDir>/<name>, runs `git init`, and registers the result.
	//
	// Bootstrap and registration are kept in this order deliberately: the
	// directory is real before the row exists, so a failure mid-way leaves an
	// unregistered folder (invisible, recoverable) rather than a DB row
	// pointing at a directory that was never created (broken, and it would
	// make the project unusable on next open).
	app.Post("/api/projects/init", func(c *fiber.Ctx) error {
		var body consolev1.InitProjectRequest
		if err := protoUnmarshal.Unmarshal(c.Body(), &body); err != nil {
			return fail400(c, fmt.Errorf("Invalid body."))
		}
		if strings.TrimSpace(body.Name) == "" {
			return fail400(c, fmt.Errorf("Field 'name' is required."))
		}
		if strings.TrimSpace(body.ParentDir) == "" {
			return fail400(c, fmt.Errorf("Field 'parentDir' is required."))
		}
		dir, err := bootstrap.InitBlank(body.ParentDir, body.Name)
		if err != nil {
			return fail400(c, err)
		}
		project, err := projects.Create(projectssvc.CreateProjectOptions{Name: body.Name, Dir: dir})
		if err != nil {
			return fail400(c, err)
		}
		return sendProject(c, project)
	})

	// POST /api/projects/clone — clone a remote repo, then register it.
	//
	// Synchronous by design (see docs/plan/new-project-dialog-plan.md): there
	// is no job handle to stream progress or cancel against, so the dialog
	// shows a busy state for the duration of the request.
	app.Post("/api/projects/clone", func(c *fiber.Ctx) error {
		var body consolev1.CloneProjectRequest
		if err := protoUnmarshal.Unmarshal(c.Body(), &body); err != nil {
			return fail400(c, fmt.Errorf("Invalid body."))
		}
		if strings.TrimSpace(body.Url) == "" {
			return fail400(c, fmt.Errorf("Field 'url' is required."))
		}
		if strings.TrimSpace(body.ParentDir) == "" {
			return fail400(c, fmt.Errorf("Field 'parentDir' is required."))
		}
		dir, err := bootstrap.Clone(body.Url, body.ParentDir, body.GetName())
		if err != nil {
			return failProjectBootstrap(c, err)
		}
		// An explicit name is the user's choice; without one the folder name
		// came from the URL, which is also the best display name.
		name := body.GetName()
		if strings.TrimSpace(name) == "" {
			name = filepath.Base(dir)
		}
		project, err := projects.Create(projectssvc.CreateProjectOptions{Name: name, Dir: dir})
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
