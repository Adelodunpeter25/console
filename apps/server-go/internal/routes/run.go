// Agent run routes. Port of apps/server/api/src/routes/run.ts: run,
// attach-to-stream, abort, queue, steer, approve, answer.
package routes

import (
	"errors"
	"strings"

	"github.com/gofiber/fiber/v2"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/tools"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/run"
)

type runPromptBody struct {
	Prompt       string `json:"prompt"`
	ModelID      string `json:"modelId"`
	Provider     string `json:"provider"`
	ApprovalMode string `json:"approvalMode"`
	Thinking     string `json:"thinkingLevel"`
	Attachments  []struct {
		Data     string `json:"data"`
		MimeType string `json:"mimeType"`
	} `json:"attachments"`
}

func bodyToPrompt(body runPromptBody) run.Prompt {
	dto := run.Prompt{
		Text: strings.TrimSpace(body.Prompt), ModelID: body.ModelID, Provider: body.Provider,
		ApprovalMode: body.ApprovalMode, Thinking: body.Thinking,
	}
	for _, a := range body.Attachments {
		dto.Attachments = append(dto.Attachments, run.Attachment{Data: a.Data, MimeType: a.MimeType})
	}
	return dto
}

func registerRunRoutes(app *fiber.App, runs *run.Service) {
	// POST /api/sessions/:id/run — start a run and stream its events.
	app.Post("/api/sessions/:id/run", func(c *fiber.Ctx) error {
		sessionID := c.Params("id")
		var body runPromptBody
		if err := c.BodyParser(&body); err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"success": false, "error": "Invalid request body."})
		}
		if strings.TrimSpace(body.Prompt) == "" {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"success": false, "error": "Field 'prompt' is required."})
		}
		hub, err := runs.StartRun(sessionID, bodyToPrompt(body))
		if err != nil {
			switch {
			case errors.Is(err, run.ErrActive):
				return c.Status(fiber.StatusConflict).JSON(fiber.Map{"success": false, "error": err.Error()})
			case errors.Is(err, run.ErrNoSession):
				return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"success": false, "error": err.Error()})
			case strings.HasPrefix(err.Error(), "Unknown provider"):
				return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"success": false, "error": err.Error()})
			default:
				return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"success": false, "error": err.Error()})
			}
		}
		return streamSSE(c, func(sse *sseStream) {
			pumpHub(sse, hub, nil)
		})
	})

	// GET /api/sessions/:id/run/stream — attach to an in-flight run.
	// ?since=<seq> replays buffered events newer than seq after a
	// streamReset frame. 409 when no run is active.
	app.Get("/api/sessions/:id/run/stream", func(c *fiber.Ctx) error {
		sessionID := c.Params("id")
		var since *int64
		if raw := strings.TrimSpace(c.Query("since")); raw != "" {
			parsed, err := parseSince(raw)
			if err != nil {
				return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"success": false, "error": "Field 'since' must be a non-negative integer."})
			}
			since = &parsed
		}
		if !runs.IsActive(sessionID) {
			return c.Status(fiber.StatusConflict).JSON(fiber.Map{"success": false, "error": "No active run for session '" + sessionID + "'."})
		}
		hub := runs.Hub(sessionID)
		if hub == nil {
			return c.Status(fiber.StatusConflict).JSON(fiber.Map{"success": false, "error": "No active run for session '" + sessionID + "'."})
		}
		return streamSSE(c, func(sse *sseStream) {
			pumpHub(sse, hub, since)
		})
	})

	// POST /api/sessions/:id/abort — cancel the active run and discard any
	// staged prompt: Stop means stop everything.
	app.Post("/api/sessions/:id/abort", func(c *fiber.Ctx) error {
		sessionID := c.Params("id")
		if !runs.Abort(sessionID) {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"success": false, "error": "No active run found for session '" + sessionID + "'."})
		}
		if _, err := runs.ClearQueuedPrompt(sessionID); err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"success": false, "error": err.Error()})
		}
		return c.JSON(fiber.Map{"success": true, "data": fiber.Map{"sessionId": sessionID, "aborted": true}})
	})

	// POST /api/sessions/:id/answer — answer a pending agent question.
	app.Post("/api/sessions/:id/answer", func(c *fiber.Ctx) error {
		sessionID := c.Params("id")
		var body struct {
			RequestID string `json:"requestId"`
			Answer    any    `json:"answer"`
		}
		if err := c.BodyParser(&body); err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"success": false, "error": "Invalid request body."})
		}
		answer, ok := parseAnswer(body.Answer)
		if !ok {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"success": false, "error": "Field 'answer' must be a string or string array."})
		}
		if !runs.AnswerQuestion(sessionID, body.RequestID, answer) {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"success": false, "error": "No pending question for requestId '" + body.RequestID + "'."})
		}
		return c.JSON(fiber.Map{"success": true, "data": fiber.Map{"answered": true}})
	})

	// POST /api/sessions/:id/approve — approve or deny a pending tool request.
	app.Post("/api/sessions/:id/approve", func(c *fiber.Ctx) error {
		sessionID := c.Params("id")
		var body struct {
			RequestID string `json:"requestId"`
			Allow     bool   `json:"allow"`
		}
		if err := c.BodyParser(&body); err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"success": false, "error": "Invalid request body."})
		}
		if !runs.ApprovePermission(sessionID, body.RequestID, body.Allow) {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"success": false, "error": "No pending permission for requestId '" + body.RequestID + "'."})
		}
		return c.JSON(fiber.Map{"success": true, "data": fiber.Map{"approved": body.Allow}})
	})

	// POST /api/sessions/:id/queue — stage (or replace) the prompt that
	// runs once the active turn settles.
	app.Post("/api/sessions/:id/queue", func(c *fiber.Ctx) error {
		sessionID := c.Params("id")
		var body runPromptBody
		if err := c.BodyParser(&body); err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"success": false, "error": "Invalid request body."})
		}
		if strings.TrimSpace(body.Prompt) == "" {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"success": false, "error": "Field 'prompt' is required."})
		}
		queued, err := runs.QueuePrompt(sessionID, bodyToPrompt(body))
		if err != nil {
			return queueError(c, sessionID, err)
		}
		return c.JSON(fiber.Map{"success": true, "data": queued})
	})

	// PUT /api/sessions/:id/queue — edit the staged prompt in place.
	app.Put("/api/sessions/:id/queue", func(c *fiber.Ctx) error {
		sessionID := c.Params("id")
		var body runPromptBody
		if err := c.BodyParser(&body); err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"success": false, "error": "Invalid request body."})
		}
		if strings.TrimSpace(body.Prompt) == "" {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"success": false, "error": "Field 'prompt' is required."})
		}
		updated, err := runs.EditQueuedPrompt(sessionID, bodyToPrompt(body))
		if err != nil {
			return queueError(c, sessionID, err)
		}
		if updated == nil {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"success": false, "error": "No queued prompt found for session '" + sessionID + "'."})
		}
		return c.JSON(fiber.Map{"success": true, "data": updated})
	})

	// GET /api/sessions/:id/queue — fetch the staged prompt, if any.
	app.Get("/api/sessions/:id/queue", func(c *fiber.Ctx) error {
		queued, err := runs.QueuedPrompt(c.Params("id"))
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"success": false, "error": err.Error()})
		}
		return c.JSON(fiber.Map{"success": true, "data": queued})
	})

	// DELETE /api/sessions/:id/queue — discard the staged prompt.
	app.Delete("/api/sessions/:id/queue", func(c *fiber.Ctx) error {
		deleted, err := runs.ClearQueuedPrompt(c.Params("id"))
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"success": false, "error": err.Error()})
		}
		return c.JSON(fiber.Map{"success": true, "data": fiber.Map{"deleted": deleted}})
	})

	// POST /api/sessions/:id/steer — halt the run and stage body to start
	// as the next turn as soon as the aborted run settles.
	app.Post("/api/sessions/:id/steer", func(c *fiber.Ctx) error {
		sessionID := c.Params("id")
		var body runPromptBody
		if err := c.BodyParser(&body); err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"success": false, "error": "Invalid request body."})
		}
		if strings.TrimSpace(body.Prompt) == "" {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"success": false, "error": "Field 'prompt' is required."})
		}
		steered, err := runs.Steer(sessionID, bodyToPrompt(body))
		if err != nil {
			return queueError(c, sessionID, err)
		}
		if !steered {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"success": false, "error": "No active run found for session '" + sessionID + "'."})
		}
		return c.JSON(fiber.Map{"success": true, "data": fiber.Map{"steered": true}})
	})
}

// queueError maps queue storage errors: missing session → 404.
func queueError(c *fiber.Ctx, sessionID string, err error) error {
	if errors.Is(err, run.ErrNoSession) {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"success": false, "error": "No session found for id '" + sessionID + "'."})
	}
	return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"success": false, "error": err.Error()})
}

// parseAnswer maps the TS answer union (string | string[]) to AskAnswer.
func parseAnswer(raw any) (tools.AskAnswer, bool) {
	switch v := raw.(type) {
	case string:
		return tools.AskAnswer{Text: v}, true
	case []any:
		answer := tools.AskAnswer{}
		for _, item := range v {
			s, ok := item.(string)
			if !ok {
				return tools.AskAnswer{}, false
			}
			answer.Multi = append(answer.Multi, s)
		}
		return answer, true
	default:
		return tools.AskAnswer{}, false
	}
}

// pumpHub streams live frames until the hub settles, then a terminal frame.
// Send errors (client gone) end the pump; the server-side run continues.
func pumpHub(sse *sseStream, hub *run.Hub, since *int64) {
	id, ch, replay := hub.Subscribe(since)
	if ch == nil {
		_ = sse.Send("error", mustJSON(fiber.Map{"type": "error", "error": fiber.Map{"message": "Run already settled."}}))
		return
	}
	defer hub.Unsubscribe(id)
	if since != nil {
		if err := sse.Send("streamReset", mustJSON(fiber.Map{"type": "streamReset"})); err != nil {
			return
		}
	}
	for _, f := range replay {
		if err := sendFrame(sse, f); err != nil {
			return
		}
	}
	for f := range ch {
		if err := sendFrame(sse, f); err != nil {
			return
		}
	}
	if hub.Outcome == run.OutcomeAborted {
		_ = sse.Send("aborted", mustJSON(fiber.Map{"type": "aborted", "reason": "Run was aborted."}))
	} else {
		_ = sse.Send("done", mustJSON(fiber.Map{"type": "done"}))
	}
}

func sendFrame(sse *sseStream, f run.Frame) error {
	return sse.Send(string(f.Event.Kind), mustJSON(f.Event))
}

func parseSince(raw string) (int64, error) {
	var v int64
	for i := 0; i < len(raw); i++ {
		if raw[i] < '0' || raw[i] > '9' {
			return 0, errors.New("not a non-negative integer")
		}
		v = v*10 + int64(raw[i]-'0')
	}
	return v, nil
}
