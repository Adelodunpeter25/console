package routes

import (
	"encoding/json"
	"strings"

	"github.com/gofiber/fiber/v2"
	"google.golang.org/protobuf/proto"

	consolev1 "github.com/Adelodunpeter25/console/apps/server-go/internal/gen/console/v1"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/run"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/services"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/services/background"
	sessionSvc "github.com/Adelodunpeter25/console/apps/server-go/internal/services/session"
)

const protoContentType = "application/x-protobuf"

// wantsBinaryBootstrap reports whether the client asked for the protobuf
// encoding. The JSON envelope stays the default so existing clients are
// unaffected; the binary form removes base64 and field names from messages.
func wantsBinaryBootstrap(c *fiber.Ctx) bool {
	for _, part := range strings.Split(c.Get("Accept"), ",") {
		if strings.TrimSpace(strings.SplitN(part, ";", 2)[0]) == protoContentType {
			return true
		}
	}
	return false
}

func backgroundWorkToProto(w background.Work) *consolev1.BackgroundWork {
	return &consolev1.BackgroundWork{
		Id: w.ID, SessionId: w.SessionID,
		ParentRunId:  optionalNonEmpty(w.ParentRunID),
		ParentCallId: optionalNonEmpty(w.ParentCallID),
		Kind:         string(w.Kind),
		Status:       string(w.Status),
		CreatedAt:    w.CreatedAt,
		StartedAt:    optionalNonEmpty(w.StartedAt),
		FinishedAt:   optionalNonEmpty(w.FinishedAt),
		Summary:      optionalNonEmpty(w.Summary),
		Error:        optionalNonEmpty(w.Error),
		Metadata:     w.Metadata,
	}
}

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
		header := sessionHeaderToProto(result.Header)
		messages := visibleMessages(result.Messages)

		changes, err := sessions.GetSessionFileChanges(id, -1)
		if err != nil {
			return sessionError(c, fiber.StatusInternalServerError, err.Error())
		}
		changeRows := sessionFileChangesToProto(changes)
		for _, row := range changeRows {
			row.DiffText = nil
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

		suggestions, err := sessions.GetSessionSuggestions(id)
		if err != nil {
			return sessionError(c, fiber.StatusInternalServerError, err.Error())
		}

		queue, err := runs.QueuedPrompts(id)
		if err != nil {
			return sessionError(c, fiber.StatusInternalServerError, err.Error())
		}

		work := runs.BackgroundWorkManager().List(id)

		stored, err := drafts.Get(id)
		if err != nil {
			return sessionError(c, fiber.StatusInternalServerError, err.Error())
		}

		if wantsBinaryBootstrap(c) {
			binaryMessages := make([]*consolev1.AgentMessage, 0, len(messages))
			for _, raw := range messages {
				var msg consolev1.AgentMessage
				// A row that no longer parses is skipped, matching the
				// desktop's lenient message decoder.
				if err := protoUnmarshal.Unmarshal(raw, &msg); err != nil {
					continue
				}
				binaryMessages = append(binaryMessages, &msg)
			}
			queued := make([]*consolev1.QueuedPrompt, 0, len(queue))
			for i := range queue {
				queued = append(queued, run.QueuedPromptToProto(&queue[i]))
			}
			rows := make([]*consolev1.BackgroundWork, 0, len(work))
			for _, w := range work {
				rows = append(rows, backgroundWorkToProto(w))
			}
			body := &consolev1.SessionBootstrapResponse{
				Header: header, Messages: binaryMessages, HasMore: result.HasMore,
				NextCursor: result.NextCursor, Changes: changeRows, Todos: todoItems,
				Suggestions: suggestions, QueuedPrompts: queued, BackgroundWork: rows,
				Draft: stored,
			}
			raw, err := proto.Marshal(body)
			if err != nil {
				return sessionError(c, fiber.StatusInternalServerError, "encode failed")
			}
			c.Set("Content-Type", protoContentType)
			return c.Send(raw)
		}

		// JSON remains the default. Every key is present; an absent draft is
		// a literal JSON null rather than an empty proto message.
		headerRaw, err := protoMarshal.Marshal(header)
		if err != nil {
			return sessionError(c, fiber.StatusInternalServerError, "encode failed")
		}
		changesData, err := marshalProtoList(changeRows)
		if err != nil {
			return sessionError(c, fiber.StatusInternalServerError, "encode failed")
		}
		todosData, err := marshalProtoList(todoItems)
		if err != nil {
			return sessionError(c, fiber.StatusInternalServerError, "encode failed")
		}
		queuedPrompts := make([]any, 0, len(queue))
		for i := range queue {
			item, err := run.QueuedPromptData(&queue[i])
			if err != nil {
				return sessionError(c, fiber.StatusInternalServerError, "encode failed")
			}
			queuedPrompts = append(queuedPrompts, item)
		}
		var draft json.RawMessage = json.RawMessage("null")
		if stored != nil {
			raw, err := protoMarshal.Marshal(stored)
			if err != nil {
				return sessionError(c, fiber.StatusInternalServerError, "encode failed")
			}
			draft = raw
		}

		return c.JSON(fiber.Map{"success": true, "data": fiber.Map{
			"header":         json.RawMessage(headerRaw),
			"messages":       messages,
			"hasMore":        result.HasMore,
			"nextCursor":     result.NextCursor,
			"changes":        changesData,
			"todos":          todosData,
			"suggestions":    suggestions,
			"queuedPrompts":  queuedPrompts,
			"backgroundWork": work,
			"draft":          draft,
		}})
	})
}
