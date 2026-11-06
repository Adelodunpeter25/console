// Task-from-repo route (POST /api/sessions/from-repo).
//
// The one-tap path behind the mobile/desktop repo pickers: paste a GitHub
// URL, get a registered project and a session ready for a prompt — without
// the repo existing locally first. It composes the two bootstrap pieces the
// New-project dialog uses (ProjectBootstrapService.Clone + ProjectService.
// Create), reusing a project already registered at the target directory
// instead of cloning over it. The client sends its prompt to the normal
// POST /api/sessions/:id/run afterwards, which keeps run-start and SSE
// streaming in the request the UI already knows how to make.
package routes

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/gofiber/fiber/v2"

	projectssvc "github.com/Adelodunpeter25/console/apps/server-go/internal/services/projects"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/services"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/types"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/utils"
)

func RegisterRepoTaskRoutes(app *fiber.App, projects *projectssvc.ProjectService, bootstrap *projectssvc.ProjectBootstrapService, sessions *services.SessionService) {
	app.Post("/api/sessions/from-repo", func(c *fiber.Ctx) error {
		var body struct {
			Url       string `json:"url"`
			Name      string `json:"name"`
			ParentDir string `json:"parentDir"`
		}
		if err := c.BodyParser(&body); err != nil {
			return fail400(c, fmt.Errorf("Invalid body."))
		}
		url := strings.TrimSpace(body.Url)
		if url == "" {
			return fail400(c, fmt.Errorf("Field 'url' is required."))
		}
		name := strings.TrimSpace(body.Name)
		if name == "" {
			parsed, err := bootstrap.RepoName(url)
			if err != nil {
				return failProjectBootstrap(c, err)
			}
			name = parsed
		}

		// Where the clone would land — the same rule Clone applies. A
		// project already registered there is reused as-is: tapping the same
		// repo twice in the picker must not clone a second copy.
		parent := body.ParentDir
		if strings.TrimSpace(parent) == "" {
			parent = utils.ProjectsDir()
		}
		dir := filepath.Join(parent, name)

		project, err := projects.GetByDir(dir)
		if err != nil {
			cloneDir, err := bootstrap.Clone(url, body.ParentDir, name)
			if err != nil {
				return failProjectBootstrap(c, err)
			}
			project, err = projects.Create(projectssvc.CreateProjectOptions{Name: name, Dir: cloneDir})
			if err != nil {
				return fail400(c, err)
			}
		}

		header, err := sessions.Create(types.CreateSessionOptions{
			Title:     project.Name,
			Cwd:       project.Path,
			ProjectID: &project.ID,
		})
		if err != nil {
			return fail400(c, err)
		}

		projRaw, err := protoMarshal.Marshal(projectToProto(project))
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"success": false, "error": "encode failed"})
		}
		sessRaw, err := protoMarshal.Marshal(sessionHeaderToProto(header))
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"success": false, "error": "encode failed"})
		}
		return c.JSON(fiber.Map{"success": true, "data": fiber.Map{
			"project": json.RawMessage(projRaw),
			"session": json.RawMessage(sessRaw),
		}})
	})
}
