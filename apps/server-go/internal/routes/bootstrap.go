package routes

import (
	"encoding/json"

	"github.com/gofiber/fiber/v2"

	consolev1 "github.com/Adelodunpeter25/console/apps/server-go/internal/gen/console/v1"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/run"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/services"
	sessionSvc "github.com/Adelodunpeter25/console/apps/server-go/internal/services/session"
)

// RegisterBootstrapRoutes serves the single request a client makes when it
// opens a chat: header, first message page, and every side panel in one
// response, so opening a session is one round trip instead of six.
func RegisterBootstrapRoutes(app *fiber.App, sessions *services.SessionService, runs *run.Service, drafts *services.DraftService) {
	app.Get("/api/sessions/:id/bootstrap", func(c *fiber.Ctx) error {
		id := c.Params("id")

		result, err := sessions.Load(id, 50, 0)
		if err != nil {
			return sessionError(c, fiber.StatusInternalServerError, err.Error())
		}
		if result == nil {
			return sessionError(c, fiber.StatusNotFound, "Session '"+id+"' not found.")
		}
		// Settle a stale working state like GET /:id does: a chat whose run
		// is gone is done. Status-only, so the sidebar order does not move.
		if staleRunStatus(result.Header, runs.IsActive(id)) {
			if err := sessions.UpdateStatusOnly(id, "done"); err == nil {
				result.Header.Status = "done"
				result.Header.Unread = sessionSvc.IsUnread(result.Header)
			}
		}
		headerRaw, err := protoMarshal.Marshal(sessionHeaderToProto(result.Header))
		if err != nil {
			return sessionError(c, fiber.StatusInternalServerError, "encode failed")
		}

		changes, err := sessions.GetSessionFileChanges(id, -1)
		if err != nil {
			return sessionError(c, fiber.StatusInternalServerError, err.Error())
		}
		rows := sessionFileChangesToProto(changes)
		for _, row := range rows {
			row.DiffText = nil
		}
		changesData, err := marshalProtoList(rows)
		if err != nil {
			return sessionError(c, fiber.StatusInternalServerError, "encode failed")
		}

		todos, err := sessions.GetSessionTodos(id)
		if err != nil {
			return sessionError(c, fiber.StatusInternalServerError, err.Error())
		}
		todoItems := make([]*consolev1.TodoItem, 0, len(todos))
		for _, item := range todos {
			todoItems = append(todoItems, &consolev1.TodoItem{
				Id: int32(item.ID), Content: item.Content, Status: item.Status,
			})
		}
		todosData, err := marshalProtoList(todoItems)
		if err != nil {
			return sessionError(c, fiber.StatusInternalServerError, "encode failed")
		}

		suggestions, err := sessions.GetSessionSuggestions(id)
		if err != nil {
			return sessionError(c, fiber.StatusInternalServerError, err.Error())
		}

		queue, err := runs.QueuedPrompts(id)
		if err != nil {
			return sessionError(c, fiber.StatusInternalServerError, err.Error())
		}
		queuedPrompts := make([]any, 0, len(queue))
		for i := range queue {
			item, err := run.QueuedPromptData(&queue[i])
			if err != nil {
				return sessionError(c, fiber.StatusInternalServerError, "encode failed")
			}
			queuedPrompts = append(queuedPrompts, item)
		}

		backgroundWork := runs.BackgroundWorkManager().List(id)

		// A chat with no draft must serialize to a literal JSON null, not an
		// empty proto message.
		var draft json.RawMessage = json.RawMessage("null")
		stored, err := drafts.Get(id)
		if err != nil {
			return sessionError(c, fiber.StatusInternalServerError, err.Error())
		}
		if stored != nil {
			raw, err := protoMarshal.Marshal(stored)
			if err != nil {
				return sessionError(c, fiber.StatusInternalServerError, "encode failed")
			}
			draft = raw
		}

		return c.JSON(fiber.Map{"success": true, "data": fiber.Map{
			"header":         json.RawMessage(headerRaw),
			"messages":       visibleMessages(result.Messages),
			"hasMore":        result.HasMore,
			"nextCursor":     result.NextCursor,
			"changes":        changesData,
			"todos":          todosData,
			"suggestions":    suggestions,
			"queuedPrompts":  queuedPrompts,
			"backgroundWork": backgroundWork,
			"draft":          draft,
		}})
	})
}
