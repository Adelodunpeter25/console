// Git routes (/api/git/*). Port of apps/server/api/src/routes/git.ts.
package routes

import (
	"fmt"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/services"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/types"
)

func registerGitRoutes(app *fiber.App, git *services.GitService, watch *services.FsWatchService) {
	h := app.Group("/api/git")

	ok := func(c *fiber.Ctx, data any) error {
		return c.JSON(fiber.Map{"success": true, "data": data})
	}
	fail := func(c *fiber.Ctx, err error) error {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"success": false, "error": err.Error()})
	}

	// GET /api/git/status
	h.Get("/status", func(c *fiber.Ctx) error {
		repoPath := c.Query("path")
		if repoPath == "" {
			return fail(c, fmt.Errorf("Query parameter 'path' is required."))
		}
		return ok(c, git.GetGitStatus(repoPath))
	})

	// GET /api/git/status/watch — SSE snapshots on subscribe + debounced fs changes.
	h.Get("/status/watch", func(c *fiber.Ctx) error {
		repoPath := c.Query("path")
		if repoPath == "" {
			return fail(c, fmt.Errorf("Query parameter 'path' is required."))
		}
		watch.Watch(repoPath)
		return streamSSE(c, func(sse *sseStream) {
			events := watch.Subscribe(repoPath)
			defer watch.Unsubscribe(events)

			sendStatus := func() {
				summary := git.GetGitStatus(repoPath)
				_ = sse.Send("gitStatus", mustJSON(summary))
			}
			sendStatus()

			debounce := time.NewTimer(400 * time.Millisecond)
			debounce.Stop()
			ticker := time.NewTicker(15 * time.Second)
			defer ticker.Stop()
			for {
				select {
				case <-events:
					debounce.Reset(400 * time.Millisecond)
				case <-debounce.C:
					sendStatus()
				case <-ticker.C:
					if err := sse.Send("ping", ""); err != nil {
						return
					}
				}
			}
		})
	})

	// GET /api/git/diff
	h.Get("/diff", func(c *fiber.Ctx) error {
		repoPath := c.Query("repoPath")
		if repoPath == "" {
			repoPath = c.Query("cwd")
		}
		filePath := c.Query("path")
		if repoPath == "" && filePath == "" {
			return fail(c, fmt.Errorf("Query parameter 'repoPath' (or 'path') is required."))
		}
		diff, err := git.GetDiff(repoPath, filePath)
		if err != nil {
			return fail(c, err)
		}
		return ok(c, fiber.Map{"path": filePath, "diff": diff})
	})

	// GET /api/git/branches
	h.Get("/branches", func(c *fiber.Ctx) error {
		repoPath := c.Query("path")
		if repoPath == "" {
			return fail(c, fmt.Errorf("Query parameter 'path' is required."))
		}
		return ok(c, git.ListBranches(repoPath))
	})

	// POST /api/git/checkout
	h.Post("/checkout", func(c *fiber.Ctx) error {
		var body struct {
			Path   string `json:"path"`
			Branch string `json:"branch"`
		}
		if err := c.BodyParser(&body); err != nil {
			return fail(c, fmt.Errorf("Invalid body."))
		}
		if body.Branch == "" {
			return fail(c, fmt.Errorf("Field 'branch' is required."))
		}
		if body.Path == "" {
			return fail(c, fmt.Errorf("Field 'path' is required."))
		}
		if err := git.CheckoutBranch(body.Path, body.Branch); err != nil {
			return fail(c, err)
		}
		return ok(c, fiber.Map{"branch": body.Branch})
	})
}

var _ = types.GitFileStatus("")
