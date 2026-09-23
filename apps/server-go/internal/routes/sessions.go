package routes

import (
	"encoding/json"
	"errors"
	"strconv"

	"github.com/gofiber/fiber/v2"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/run"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/services"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/services/session"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/types"
)

// Session routes. Response shapes, status codes, and defaults mirror the TS
// routes/sessions.ts + SessionService so the desktop client works unchanged.
func registerSessionRoutes(app *fiber.App, sessions *services.SessionService, runs *run.Service) {
	h := app.Group("/api/sessions")

	// GET /api/sessions — list, optionally filtered by cwd/projectId, with
	// onlyDeleted=true selecting the trash view.
	h.Get("/", func(c *fiber.Ctx) error {
		list, err := sessions.ListFiltered(session.ListFilter{
			Cwd:         c.Query("cwd"),
			ProjectID:   c.Query("projectId"),
			OnlyDeleted: c.Query("onlyDeleted") == "true",
		})
		if err != nil {
			return sessionError(c, fiber.StatusInternalServerError, err.Error())
		}
		return c.JSON(fiber.Map{"success": true, "data": list})
	})

	// POST /api/sessions — create a new session. Field defaults mirror the
	// TS SessionService.createSession (fallback model/provider, cwd, project
	// inference, scratchpad for explicit-null projectId).
	h.Post("/", func(c *fiber.Ctx) error {
		var req types.CreateSessionOptions
		if err := json.Unmarshal(c.Body(), &req); err != nil {
			return sessionError(c, fiber.StatusBadRequest, "Invalid request body.")
		}
		req.ProjectNull = bodyFieldIsNull(c.Body(), "projectId")
		header, err := sessions.Create(req)
		if err != nil {
			// Worktree request errors are client errors, not 500s.
			if errors.Is(err, services.ErrWorktreeScratchpad) ||
				errors.Is(err, services.ErrWorktreeNeedsCwd) ||
				errors.Is(err, services.ErrNotGitRepo) ||
				errors.Is(err, services.ErrUnbornHEAD) {
				return sessionError(c, fiber.StatusBadRequest, err.Error())
			}
			return sessionError(c, fiber.StatusInternalServerError, err.Error())
		}
		return c.JSON(fiber.Map{"success": true, "data": header})
	})

	// GET /api/sessions/:id — header plus a page of message history.
	h.Get("/:id", func(c *fiber.Ctx) error {
		id := c.Params("id")
		limit, before, ok := parsePageParams(c)
		if !ok {
			return sessionError(c, fiber.StatusBadRequest, "'limit' and 'before' must be positive integers.")
		}
		result, err := sessions.Load(id, limit, before)
		if err != nil {
			return sessionError(c, fiber.StatusInternalServerError, err.Error())
		}
		if result == nil {
			return sessionError(c, fiber.StatusNotFound, "Session '"+id+"' not found.")
		}
		// Settle stale working state on read, mirroring getSession: a chat
		// whose run is gone is done, not stuck working.
		if !runs.IsActive(id) && (result.Header.Status == "working" || result.Header.Status == "needs_attention") {
			if err := sessions.UpdateStatus(id, "done"); err == nil {
				result.Header.Status = "done"
			}
		}
		return c.JSON(fiber.Map{"success": true, "data": result})
	})

	// PATCH /api/sessions/:id — update title, model/provider, approval mode,
	// or cwd/project (the latter only before the first message, mirroring
	// the TS cwd lock). Returns the refreshed header.
	h.Patch("/:id", func(c *fiber.Ctx) error {
		id := c.Params("id")
		var req struct {
			Title        *string `json:"title"`
			ModelID      *string `json:"modelId"`
			Provider     *string `json:"provider"`
			ApprovalMode *string `json:"approvalMode"`
			Cwd          *string `json:"cwd"`
		}
		if err := json.Unmarshal(c.Body(), &req); err != nil {
			return sessionError(c, fiber.StatusBadRequest, "Invalid request body.")
		}
		present := bodyFieldsPresent(c.Body(), "projectId", "cwd")

		header, err := sessions.Header(id)
		if err != nil {
			return sessionError(c, fiber.StatusInternalServerError, err.Error())
		}
		if header == nil {
			return sessionError(c, fiber.StatusNotFound, "Session '"+id+"' not found.")
		}

		if req.Title != nil && *req.Title != "" {
			if err := sessions.UpdateTitle(id, *req.Title); err != nil {
				return sessionError(c, fiber.StatusInternalServerError, err.Error())
			}
		}
		if present["cwd"] || present["projectId"] {
			// Cwd lock: silently ignore project/cwd moves once the chat has
			// messages (mirrors the TS updateSession no-op, which keeps the
			// client's header refresh running).
			if header.MessageCount == 0 {
				cwd := header.Cwd
				if req.Cwd != nil {
					cwd = *req.Cwd
				}
			var projectID *string
				if present["projectId"] {
					// Explicit value (or null) passes through as-is;
					// null/"scratch" become scratch in UpdateCwd.
					projectID = parseNullableString(c.Body(), "projectId")
				} else if cwd != "" {
					// Key absent: infer from cwd like the TS updateSession.
					if found, err := sessions.ProjectByDir(cwd); err == nil && found != "" {
						projectID = &found
					}
				} else {
					projectID = header.ProjectID
				}
				if err := sessions.UpdateCwd(id, cwd, projectID); err != nil {
					return sessionError(c, fiber.StatusInternalServerError, err.Error())
				}
			}
		}
		if req.ModelID != nil && *req.ModelID != "" {
			provider := ""
			if req.Provider != nil {
				provider = *req.Provider
			}
			if provider == "" {
				provider = header.Provider
			}
			if provider == "" {
				provider = session.DefaultFallbackProvider
			}
			if err := sessions.UpdateModel(id, *req.ModelID, provider); err != nil {
				return sessionError(c, fiber.StatusInternalServerError, err.Error())
			}
		}
		if req.ApprovalMode != nil && *req.ApprovalMode != "" {
			if err := sessions.UpdateApprovalMode(id, *req.ApprovalMode); err != nil {
				return sessionError(c, fiber.StatusInternalServerError, err.Error())
			}
		}

		updated, err := sessions.Header(id)
		if err != nil {
			return sessionError(c, fiber.StatusInternalServerError, err.Error())
		}
		if updated == nil {
			return sessionError(c, fiber.StatusNotFound, "Session '"+id+"' not found.")
		}
		return c.JSON(fiber.Map{"success": true, "data": updated})
	})

	// POST /api/sessions/:id/worktree — convert an existing, message-less
	// session in place into a worktree session (branch off its current cwd,
	// re-point cwd at the new worktree). Never creates a new session row —
	// distinct from POST /api/sessions with a worktree spec, which always
	// mints a fresh session.
	h.Post("/:id/worktree", func(c *fiber.Ctx) error {
		id := c.Params("id")
		var spec types.CreateWorktreeSpec
		if len(c.Body()) > 0 {
			if err := json.Unmarshal(c.Body(), &spec); err != nil {
				return sessionError(c, fiber.StatusBadRequest, "Invalid request body.")
			}
		}
		header, err := sessions.AttachWorktree(id, &spec)
		if err != nil {
			switch {
			case errors.Is(err, services.ErrSessionNotFound):
				return sessionError(c, fiber.StatusNotFound, "Session '"+id+"' not found.")
			case errors.Is(err, services.ErrWorktreeSessionHasMessages),
				errors.Is(err, services.ErrWorktreeAlreadyOwned),
				errors.Is(err, services.ErrWorktreeScratchpad),
				errors.Is(err, services.ErrWorktreeNeedsCwd),
				errors.Is(err, services.ErrNotGitRepo),
				errors.Is(err, services.ErrUnbornHEAD):
				return sessionError(c, fiber.StatusBadRequest, err.Error())
			default:
				return sessionError(c, fiber.StatusInternalServerError, err.Error())
			}
		}
		return c.JSON(fiber.Map{"success": true, "data": header})
	})

	// DELETE /api/sessions/:id — soft delete.
	h.Delete("/:id", func(c *fiber.Ctx) error {
		id := c.Params("id")
		deleted, err := sessions.SoftDelete(id)
		if err != nil {
			return sessionError(c, fiber.StatusInternalServerError, err.Error())
		}
		if !deleted {
			return sessionError(c, fiber.StatusNotFound, "Session '"+id+"' not found.")
		}
		return c.JSON(fiber.Map{"success": true, "data": fiber.Map{"id": id, "deleted": true}})
	})

	// POST /api/sessions/:id/restore — restore a soft-deleted session.
	h.Post("/:id/restore", func(c *fiber.Ctx) error {
		id := c.Params("id")
		restored, err := sessions.Restore(id)
		if err != nil {
			return sessionError(c, fiber.StatusInternalServerError, err.Error())
		}
		if !restored {
			return sessionError(c, fiber.StatusNotFound, "Session '"+id+"' not found.")
		}
		return c.JSON(fiber.Map{"success": true, "data": fiber.Map{"id": id, "restored": true}})
	})

	// DELETE /api/sessions/:id/permanent — irreversibly delete a
	// soft-deleted session.
	h.Delete("/:id/permanent", func(c *fiber.Ctx) error {
		id := c.Params("id")
		deleted, err := sessions.PermanentDelete(id)
		if err != nil {
			// Dirty worktree blocks the delete: conflict, session kept.
			if errors.Is(err, services.ErrWorktreeDirty) {
				return sessionError(c, fiber.StatusConflict, err.Error())
			}
			return sessionError(c, fiber.StatusInternalServerError, err.Error())
		}
		if !deleted {
			return sessionError(c, fiber.StatusNotFound, "Deleted session '"+id+"' not found.")
		}
		return c.JSON(fiber.Map{"success": true, "data": fiber.Map{"id": id, "permanentlyDeleted": true}})
	})

	// GET /api/sessions/:id/subagents — live (running) subagents.
	h.Get("/:id/subagents", func(c *fiber.Ctx) error {
		subagents, err := sessions.GetSubagents(c.Params("id"))
		if err != nil {
			return sessionError(c, fiber.StatusInternalServerError, err.Error())
		}
		return c.JSON(fiber.Map{"success": true, "data": subagents})
	})

	// GET /api/sessions/:id/todos — persisted todos for a session.
	h.Get("/:id/todos", func(c *fiber.Ctx) error {
		todos, err := sessions.GetSessionTodos(c.Params("id"))
		if err != nil {
			return sessionError(c, fiber.StatusInternalServerError, err.Error())
		}
		return c.JSON(fiber.Map{"success": true, "data": todos})
	})

	// GET /api/sessions/:id/changes — session file changes with optional turn filter.
	h.Get("/:id/changes", func(c *fiber.Ctx) error {
		sessionID := c.Params("id")
		turnIndex := -1 // -1 means all turns
		if turnStr := c.Query("turnIndex"); turnStr != "" {
			turnIndex = c.QueryInt("turnIndex", 0)
		}
		changes, err := sessions.GetSessionFileChanges(sessionID, turnIndex)
		if err != nil {
			return sessionError(c, fiber.StatusInternalServerError, err.Error())
		}
		return c.JSON(fiber.Map{"success": true, "data": changes})
	})

	// GET /api/sessions/:id/changes/diff — raw diff text for a specific file change.
	h.Get("/:id/changes/diff", func(c *fiber.Ctx) error {
		sessionID := c.Params("id")
		path := c.Query("path")
		if path == "" {
			return sessionError(c, fiber.StatusBadRequest, "path query parameter is required")
		}
		turnIndex := -1
		if turnStr := c.Query("turnIndex"); turnStr != "" {
			turnIndex = c.QueryInt("turnIndex", 0)
		}
		changes, err := sessions.GetSessionFileChanges(sessionID, turnIndex)
		if err != nil {
			return sessionError(c, fiber.StatusInternalServerError, err.Error())
		}
		for _, change := range changes {
			if change.Path == path && change.DiffText != nil {
				return c.JSON(fiber.Map{"success": true, "data": fiber.Map{"diffText": *change.DiffText}})
			}
		}
		return sessionError(c, fiber.StatusNotFound, "file change not found")
	})
}

// sessionError mirrors the TS error shape with TS status codes.
func sessionError(c *fiber.Ctx, code int, message string) error {
	return c.Status(code).JSON(fiber.Map{"success": false, "error": message})
}

// parsePageParams mirrors the TS get-session pagination: default limit 50,
// both values positive integers when present.
func parsePageParams(c *fiber.Ctx) (limit, before int64, ok bool) {
	limit = 50
	if raw := c.Query("limit"); raw != "" {
		v, err := strconv.Atoi(raw)
		if err != nil || v < 1 {
			return 0, 0, false
		}
		limit = int64(v)
	}
	if raw := c.Query("before"); raw != "" {
		v, err := strconv.Atoi(raw)
		if err != nil || v < 1 {
			return 0, 0, false
		}
		before = int64(v)
	}
	return limit, before, true
}

// bodyFieldsPresent reports which of the named top-level JSON keys are
// present in a request body (used to tell omitted apart from explicit null).
func bodyFieldsPresent(body []byte, keys ...string) map[string]bool {
	var raw map[string]json.RawMessage
	present := make(map[string]bool, len(keys))
	if err := json.Unmarshal(body, &raw); err != nil {
		return present
	}
	for _, k := range keys {
		_, present[k] = raw[k]
	}
	return present
}

// bodyFieldIsNull reports whether a top-level JSON key is present with an
// explicit null value.
func bodyFieldIsNull(body []byte, key string) bool {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(body, &raw); err != nil {
		return false
	}
	v, ok := raw[key]
	return ok && string(v) == "null"
}

// parseNullableString reads a top-level JSON string key: nil for explicit
// null (or when unparseable), matching the TS string|null DTO fields.
func parseNullableString(body []byte, key string) *string {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil
	}
	v, ok := raw[key]
	if !ok || string(v) == "null" {
		return nil
	}
	var s string
	if err := json.Unmarshal(v, &s); err != nil {
		return nil
	}
	return &s
}
