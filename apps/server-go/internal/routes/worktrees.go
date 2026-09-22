// Worktree routes (/api/worktrees/*). Orphan recovery + inventory for
// one-worktree-per-session; session create/delete wiring lives in the
// session routes + SessionService facade.
package routes

import (
	"github.com/gofiber/fiber/v2"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/services"
)

func registerWorktreeRoutes(app *fiber.App, sessions *services.SessionService, wt *services.WorktreeService) {
	h := app.Group("/api/worktrees")

	ok := func(c *fiber.Ctx, data any) error {
		return c.JSON(fiber.Map{"success": true, "data": data})
	}
	fail := func(c *fiber.Ctx, code int, err error) error {
		return c.Status(code).JSON(fiber.Map{"success": false, "error": err.Error()})
	}

	// GET /api/worktrees/ — owned (from session rows) + orphans.
	h.Get("/", func(c *fiber.Ctx) error {
		owned, orphans, err := worktreeInventory(sessions, wt)
		if err != nil {
			return fail(c, fiber.StatusInternalServerError, err)
		}
		return ok(c, fiber.Map{"owned": owned, "orphans": orphans})
	})

	// GET /api/worktrees/orphans — worktree dirs no session owns.
	h.Get("/orphans", func(c *fiber.Ctx) error {
		_, orphans, err := worktreeInventory(sessions, wt)
		if err != nil {
			return fail(c, fiber.StatusInternalServerError, err)
		}
		return ok(c, orphans)
	})

	// DELETE /api/worktrees/orphans — {path, force?}. Dirty blocks
	// without force; path must sit under the worktree root.
	h.Delete("/orphans", func(c *fiber.Ctx) error {
		var req struct {
			Path  string `json:"path"`
			Force bool   `json:"force"`
		}
		if err := c.BodyParser(&req); err != nil {
			return fail(c, fiber.StatusBadRequest, err)
		}
		if req.Path == "" {
			return fail(c, fiber.StatusBadRequest, fiber.NewError(fiber.StatusBadRequest, "Invalid request body: 'path' is required."))
		}
		root, err := services.DefaultRoot()
		if err != nil {
			return fail(c, fiber.StatusInternalServerError, err)
		}
		if err := wt.RemoveOrphan(root, req.Path, req.Force); err != nil {
			if err == services.ErrWorktreeDirty {
				return fail(c, fiber.StatusConflict, err)
			}
			return fail(c, fiber.StatusBadRequest, err)
		}
		return ok(c, fiber.Map{"path": req.Path, "removed": true})
	})
}

func worktreeInventory(sessions *services.SessionService, wt *services.WorktreeService) ([]services.WorktreeEntry, []services.WorktreeEntry, error) {
	owned := make([]services.WorktreeEntry, 0)
	orphans := make([]services.WorktreeEntry, 0)
	root, err := services.DefaultRoot()
	if err != nil {
		return owned, orphans, err
	}
	paths, err := sessions.OwnedWorktreePaths()
	if err != nil {
		return owned, orphans, err
	}
	for _, p := range paths {
		entry := services.WorktreeEntry{Path: p, Owned: true}
		if branch, err := wt.BranchOf(p); err == nil {
			entry.Branch = branch
		}
		if dirty, err := wt.IsDirty(p); err == nil {
			entry.Dirty = dirty
		}
		owned = append(owned, entry)
	}
	orphans, err = wt.ScanOrphans(root, paths)
	if err != nil {
		return owned, orphans, err
	}
	return owned, orphans, nil
}
