package routes

import (
	"encoding/json"
	"errors"
	"path/filepath"
	"strconv"

	"github.com/gofiber/fiber/v2"

	consolev1 "github.com/Adelodunpeter25/console/apps/server-go/internal/gen/console/v1"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/run"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/services"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/services/session"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/types"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/utils"
)

// subagentToProto converts a service row to the canonical wire type.
// Activity args cross as raw JSON bytes; counts narrow to int32.
func subagentToProto(s types.SubagentInfo) *consolev1.SubagentInfo {
	out := &consolev1.SubagentInfo{
		SubagentId: s.SubagentID, ParentToolCallId: s.ParentToolCallID,
		Name: s.Name, Role: s.Role, Prompt: s.Prompt,
		MaxTurns: int32(s.MaxTurns), CurrentTurn: int32(s.CurrentTurn),
		Status: s.Status, CreatedAt: s.CreatedAt, UpdatedAt: s.UpdatedAt,
	}
	if s.Summary != nil {
		out.Summary = s.Summary
	}
	if s.Error != nil {
		out.Error = s.Error
	}
	for _, a := range s.Activities {
		item := &consolev1.SubagentActivityItem{
			TurnIndex: int32(a.TurnIndex), ToolCallId: a.ToolCallID,
			ToolName: a.ToolName, Status: a.Status,
		}
		if a.Summary != nil {
			item.Summary = a.Summary
		}
		if len(a.Args) > 0 {
			item.Args = append([]byte(nil), a.Args...)
		}
		if a.Error != nil {
			item.Error = a.Error
		}
		out.Activities = append(out.Activities, item)
	}
	return out
}

// sessionFileChangeToProto converts a file-change row to the canonical
// wire type. Counts narrow to uint32 and stay JSON numbers.
func sessionFileChangeToProto(c types.SessionFileChange) *consolev1.SessionFileChange {
	out := &consolev1.SessionFileChange{
		Path: c.Path, TurnIndex: uint32(c.TurnIndex), UserMessageId: c.UserMessageID, Status: c.Status,
		Additions: uint32(c.Additions), Deletions: uint32(c.Deletions),
		Reviewed: c.Reviewed, UpdatedAt: c.UpdatedAt,
	}
	if c.DiffText != nil {
		out.DiffText = c.DiffText
	}
	return out
}

func sessionFileChangesToProto(list []types.SessionFileChange) []*consolev1.SessionFileChange {
	out := make([]*consolev1.SessionFileChange, 0, len(list))
	for _, c := range list {
		out = append(out, sessionFileChangeToProto(c))
	}
	return out
}

// sessionHeaderToProto converts a service header to the canonical wire
// type. Timestamps encode as protojson strings; messageCount is always
// populated, preserving the old always-present number.
func sessionHeaderToProto(h types.SessionHeader) *consolev1.SessionHeader {
	out := &consolev1.SessionHeader{
		Id: h.ID, Title: h.Title, Cwd: h.Cwd,
		ModelId: h.ModelID, Provider: h.Provider, ApprovalMode: h.ApprovalMode,
		CreatedAt: h.CreatedAt, UpdatedAt: h.UpdatedAt,
		MessageCount: func() *int32 { v := int32(h.MessageCount); return &v }(),
		Status:       h.Status,
	}
	if h.ProjectID != nil {
		out.ProjectId = h.ProjectID
	}
	if h.ThinkingLevel != nil {
		out.ThinkingLevel = h.ThinkingLevel
	}
	if h.DeletedAt != nil {
		out.DeletedAt = h.DeletedAt
	}
	if h.Worktree != nil {
		out.Worktree = &consolev1.SessionWorktree{
			Path: h.Worktree.Path, Branch: h.Worktree.Branch, Repo: h.Worktree.Repo,
		}
	}
	return out
}

// RegisterSessionRoutes registers the /api/sessions routes, with the
// response shapes, status codes, and defaults the desktop client expects.
func RegisterSessionRoutes(app *fiber.App, sessions *services.SessionService, runs *run.Service) {
	h := app.Group("/api/sessions")

	// sendSessionHeader emits one header through the canonical wire type.
	sendSessionHeader := func(c *fiber.Ctx, header types.SessionHeader) error {
		raw, err := protoMarshal.Marshal(sessionHeaderToProto(header))
		if err != nil {
			return sessionError(c, fiber.StatusInternalServerError, "encode failed")
		}
		return c.JSON(fiber.Map{"success": true, "data": json.RawMessage(raw)})
	}

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
		headers := make([]*consolev1.SessionHeader, 0, len(list))
		for _, header := range list {
			headers = append(headers, sessionHeaderToProto(header))
		}
		data, err := marshalProtoList(headers)
		if err != nil {
			return sessionError(c, fiber.StatusInternalServerError, "encode failed")
		}
		return c.JSON(fiber.Map{"success": true, "data": data})
	})

	// POST /api/sessions — create a new session. Defaults: fallback
	// model/provider, cwd, project inference, scratchpad for explicit-null
	// projectId.
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
				errors.Is(err, services.ErrUnknownBaseBranch) ||
				errors.Is(err, services.ErrUnbornHEAD) {
				return sessionError(c, fiber.StatusBadRequest, err.Error())
			}
			return sessionError(c, fiber.StatusInternalServerError, err.Error())
		}
		return sendSessionHeader(c, header)
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
		// Mixed envelope until messages migrate: proto header plus the raw
		// message page, hasMore flag, and numeric-or-null cursor, assembled
		// by hand in the old key order. Messages pass through byte-identical.
		headerRaw, err := protoMarshal.Marshal(sessionHeaderToProto(result.Header))
		if err != nil {
			return sessionError(c, fiber.StatusInternalServerError, "encode failed")
		}
		return c.JSON(fiber.Map{"success": true, "data": fiber.Map{
			"hasMore": result.HasMore, "header": json.RawMessage(headerRaw),
			"messages": result.Messages, "nextCursor": result.NextCursor,
		}})
	})

	// PATCH /api/sessions/:id — update title, model/provider, approval mode,
	// or cwd/project (the latter only before the first message).
	// Returns the refreshed header.
	h.Patch("/:id", func(c *fiber.Ctx) error {
		id := c.Params("id")
		var req struct {
			Title         *string `json:"title"`
			ModelID       *string `json:"modelId"`
			Provider      *string `json:"provider"`
			ApprovalMode  *string `json:"approvalMode"`
			ThinkingLevel *string `json:"thinkingLevel"`
			Cwd           *string `json:"cwd"`
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
			// messages (a no-op update, which keeps the client's
			// header refresh running).
			if header.MessageCount == 0 {
				cwd := header.Cwd
				if req.Cwd != nil {
					cwd = *req.Cwd
				}
				var projectID *string
				projectGiven := present["projectId"]
				if present["projectId"] {
					// Explicit value (or null) passes through as-is;
					// null/"scratch" become scratch in UpdateCwd.
					projectID = parseNullableString(c.Body(), "projectId")
					// Clearing to No project without a cwd (mobile): point at
					// the session's sandboxed scratch dir.
					if (projectID == nil || *projectID == "" || *projectID == "scratch") && req.Cwd == nil {
						cwd = filepath.Join(utils.ConsoleStorageDir(), "scratch", id)
					}
				} else if cwd != "" {
					// Key absent: infer the project from cwd.
					// No match means no project.
					if found, err := sessions.ProjectByDir(cwd); err == nil && found != "" {
						projectID = &found
					}
					projectGiven = true
				} else {
					projectID = header.ProjectID
				}
				if err := sessions.UpdateCwd(id, cwd, projectID, projectGiven); err != nil {
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
		if req.ThinkingLevel != nil && *req.ThinkingLevel != "" {
			if err := sessions.UpdateThinkingLevel(id, *req.ThinkingLevel); err != nil {
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
		return sendSessionHeader(c, *updated)
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
		return sendSessionHeader(c, header)
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
		raw, err := protoMarshal.Marshal(&consolev1.SessionDeleteResponse{Id: id, Deleted: true})
		if err != nil {
			return sessionError(c, fiber.StatusInternalServerError, "encode failed")
		}
		return c.JSON(fiber.Map{"success": true, "data": json.RawMessage(raw)})
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
		raw, err := protoMarshal.Marshal(&consolev1.SessionRestoreResponse{Id: id, Restored: true})
		if err != nil {
			return sessionError(c, fiber.StatusInternalServerError, "encode failed")
		}
		return c.JSON(fiber.Map{"success": true, "data": json.RawMessage(raw)})
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
		raw, err := protoMarshal.Marshal(&consolev1.SessionPermanentDeleteResponse{Id: id, PermanentlyDeleted: true})
		if err != nil {
			return sessionError(c, fiber.StatusInternalServerError, "encode failed")
		}
		return c.JSON(fiber.Map{"success": true, "data": json.RawMessage(raw)})
	})

	// GET /api/sessions/:id/subagents — live (running) subagents.
	h.Get("/:id/subagents", func(c *fiber.Ctx) error {
		subagents, err := sessions.GetSubagents(c.Params("id"))
		if err != nil {
			return sessionError(c, fiber.StatusInternalServerError, err.Error())
		}
		items := make([]*consolev1.SubagentInfo, 0, len(subagents))
		for _, sub := range subagents {
			items = append(items, subagentToProto(sub))
		}
		data, err := marshalProtoList(items)
		if err != nil {
			return sessionError(c, fiber.StatusInternalServerError, "encode failed")
		}
		return c.JSON(fiber.Map{"success": true, "data": data})
	})

	// GET /api/sessions/:id/todos — persisted todos for a session.
	h.Get("/:id/todos", func(c *fiber.Ctx) error {
		todos, err := sessions.GetSessionTodos(c.Params("id"))
		if err != nil {
			return sessionError(c, fiber.StatusInternalServerError, err.Error())
		}
		items := make([]*consolev1.TodoItem, 0, len(todos))
		for _, item := range todos {
			items = append(items, &consolev1.TodoItem{
				Id: int32(item.ID), Content: item.Content, Status: item.Status,
			})
		}
		data, err := marshalProtoList(items)
		if err != nil {
			return sessionError(c, fiber.StatusInternalServerError, "encode failed")
		}
		return c.JSON(fiber.Map{"success": true, "data": data})
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
		data, err := marshalProtoList(sessionFileChangesToProto(changes))
		if err != nil {
			return sessionError(c, fiber.StatusInternalServerError, "encode failed")
		}
		return c.JSON(fiber.Map{"success": true, "data": data})
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
				raw, err := protoMarshal.Marshal(&consolev1.SessionFileChangeDiff{DiffText: *change.DiffText})
				if err != nil {
					return sessionError(c, fiber.StatusInternalServerError, "encode failed")
				}
				return c.JSON(fiber.Map{"success": true, "data": json.RawMessage(raw)})
			}
		}
		return sessionError(c, fiber.StatusNotFound, "file change not found")
	})
	// POST /api/sessions/:id/changes/reviewed — mark/unmark a file change as reviewed.
	h.Post("/:id/changes/reviewed", func(c *fiber.Ctx) error {
		sessionID := c.Params("id")
		var body struct {
			Path      string `json:"path"`
			TurnIndex int    `json:"turnIndex"`
			Reviewed  bool   `json:"reviewed"`
		}
		if err := c.BodyParser(&body); err != nil {
			return sessionError(c, fiber.StatusBadRequest, "invalid request body")
		}
		if body.Path == "" {
			return sessionError(c, fiber.StatusBadRequest, "path is required")
		}
		if err := sessions.SetFileChangeReviewed(sessionID, body.Path, body.TurnIndex, body.Reviewed); err != nil {
			return sessionError(c, fiber.StatusInternalServerError, err.Error())
		}
		return c.SendStatus(fiber.StatusNoContent)
	})
}

// sessionError writes the {success: false, error} response with the given
// status code.
func sessionError(c *fiber.Ctx, code int, message string) error {
	return c.Status(code).JSON(fiber.Map{"success": false, "error": message})
}

// parsePageParams reads get-session pagination: default limit 50,
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
// null (or when unparseable), matching the nullable string DTO fields.
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
