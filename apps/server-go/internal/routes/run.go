// Agent run routes: run,
// attach-to-stream, abort, queue, steer, approve, answer.
package routes

import (
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/loop"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/tools"
	consolev1 "github.com/Adelodunpeter25/console/apps/server-go/internal/gen/console/v1"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/run"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/types"
)

type runPromptBody struct {
	Prompt       string   `json:"prompt"`
	ContextFiles []string `json:"contextFiles"`
	ModelID      string   `json:"modelId"`
	Provider     string   `json:"provider"`
	ApprovalMode string   `json:"approvalMode"`
	Thinking     string   `json:"thinkingLevel"`
	Attachments  []struct {
		Data     string `json:"data"`
		MimeType string `json:"mimeType"`
	} `json:"attachments"`
	Annotations []types.BrowserAnnotation `json:"annotations"`
}

func bodyToPrompt(body runPromptBody) run.Prompt {
	dto := run.Prompt{
		Text: strings.TrimSpace(body.Prompt), ContextFiles: body.ContextFiles,
		ModelID: body.ModelID, Provider: body.Provider,
		ApprovalMode: body.ApprovalMode, Thinking: body.Thinking,
		Annotations: body.Annotations,
	}
	for _, a := range body.Attachments {
		dto.Attachments = append(dto.Attachments, run.Attachment{Data: a.Data, MimeType: a.MimeType})
	}
	return dto
}

// queuedPromptToProto converts a staged prompt row to the canonical wire
// type. Attachments/annotations reuse the message schema; created_at stays
// the RFC3339 string the server stores; empty optionals stay absent.
func queuedPromptToProto(qp *types.QueuedPrompt) *consolev1.QueuedPrompt {
	if qp == nil {
		return nil
	}
	out := &consolev1.QueuedPrompt{
		Id: qp.ID, SessionId: qp.SessionID, Prompt: qp.Prompt, CreatedAt: qp.CreatedAt,
	}
	out.ContextFiles = append(out.ContextFiles, qp.ContextFiles...)
	for _, a := range qp.Attachments {
		out.Attachments = append(out.Attachments, &consolev1.ImageAttachment{
			Data: a.Data, MimeType: a.MimeType,
		})
	}
	for _, a := range qp.Annotations {
		out.Annotations = append(out.Annotations, loop.AnnotationToProto(a))
	}
	if qp.ModelID != "" {
		out.ModelId = &qp.ModelID
	}
	if qp.Provider != "" {
		out.Provider = &qp.Provider
	}
	if qp.ApprovalMode != "" {
		out.ApprovalMode = &qp.ApprovalMode
	}
	return out
}

// queuedPromptData marshals a staged row for the success envelope, preserving
// the old data:null when nothing is staged.
func queuedPromptData(qp *types.QueuedPrompt) (any, error) {
	if qp == nil {
		return nil, nil
	}
	raw, err := protoMarshal.Marshal(queuedPromptToProto(qp))
	if err != nil {
		return nil, err
	}
	return json.RawMessage(raw), nil
}

// toolCallToProto converts a live tool call to the canonical wire type,
// reusing the transcript schema: arguments cross as raw JSON bytes. Empty
// or literal-null argument payloads stay absent, as omitempty did.
func toolCallToProto(c tools.ToolCall) *consolev1.ToolCall {
	out := &consolev1.ToolCall{Id: c.ID, Name: c.Name}
	if len(c.Arguments) > 0 && string(c.Arguments) != "null" {
		out.Arguments = append([]byte(nil), c.Arguments...)
	}
	if c.ThoughtSignature != "" {
		out.ThoughtSignature = &c.ThoughtSignature
	}
	return out
}

// toolResultToProto converts a live tool result to the canonical wire type.
// Content re-encodes from its parsed form (encoding/json sorts map keys, so
// the bytes match what the old direct marshal emitted). The loop's echoed
// call args are dropped — no client reads them (message.proto notes this
// for the transcript rows too).
func toolResultToProto(r tools.ToolResult) *consolev1.ToolResult {
	out := &consolev1.ToolResult{ToolCallId: r.ToolCallID}
	if r.ToolName != "" {
		out.ToolName = &r.ToolName
	}
	if r.Content != nil {
		if raw, err := json.Marshal(r.Content); err == nil && string(raw) != "null" {
			out.Content = raw
		}
	}
	if r.IsError {
		out.IsError = &r.IsError
	}
	return out
}

// snapshotToProto converts a context-window snapshot to the canonical wire
// type. Counts narrow to int32 and stay JSON numbers.
func snapshotToProto(snap loop.ContextSnapshot) *consolev1.ContextSnapshot {
	return &consolev1.ContextSnapshot{
		UsedTokens: int32(snap.UsedTokens), ContextWindow: int32(snap.ContextWindow),
		PercentUsed: snap.PercentUsed, ThresholdRatio: snap.ThresholdRatio,
		ModelId: snap.ModelID, Provider: snap.Provider, Source: snap.Source,
	}
}

func RegisterRunRoutes(app *fiber.App, runs *run.Service) {
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

	// GET /api/sessions/:id/context — estimated context-window occupancy
	// for the footer ring. Served from the per-session cache when the
	// message count hasn't moved; recomputed otherwise.
	app.Get("/api/sessions/:id/context", func(c *fiber.Ctx) error {
		snap, err := runs.ContextUsage(c.Context(), c.Params("id"))
		if err != nil {
			if errors.Is(err, run.ErrNoSession) {
				return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"success": false, "error": "Session not found."})
			}
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"success": false, "error": err.Error()})
		}
		raw, err := protoMarshal.Marshal(snapshotToProto(snap))
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"success": false, "error": "encode failed"})
		}
		return c.JSON(fiber.Map{"success": true, "data": json.RawMessage(raw)})
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

	// POST /api/sessions/:id/browser-action — resolve a pending browser action from the desktop client.
	app.Post("/api/sessions/:id/browser-action", func(c *fiber.Ctx) error {
		sessionID := c.Params("id")
		var body struct {
			RequestID   string `json:"requestId"`
			Result      string `json:"result"`
			Error       string `json:"error"`
			ImageBase64 string `json:"imageBase64"`
		}
		if err := c.BodyParser(&body); err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"success": false, "error": "Invalid request body."})
		}
		if !runs.ResolveBrowserAction(sessionID, body.RequestID, tools.BrowserActionResult{Result: body.Result, Error: body.Error, ImageBase64: body.ImageBase64}) {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"success": false, "error": "No pending browser action for requestId '" + body.RequestID + "'."})
		}
		return c.JSON(fiber.Map{"success": true, "data": fiber.Map{"resolved": true}})
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
		data, err := queuedPromptData(queued)
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"success": false, "error": "encode failed"})
		}
		return c.JSON(fiber.Map{"success": true, "data": data})
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
		data, err := queuedPromptData(updated)
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"success": false, "error": "encode failed"})
		}
		return c.JSON(fiber.Map{"success": true, "data": data})
	})

	// GET /api/sessions/:id/queue — fetch the staged prompt, if any.
	app.Get("/api/sessions/:id/queue", func(c *fiber.Ctx) error {
		queued, err := runs.QueuedPrompt(c.Params("id"))
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"success": false, "error": err.Error()})
		}
		data, err := queuedPromptData(queued)
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"success": false, "error": "encode failed"})
		}
		return c.JSON(fiber.Map{"success": true, "data": data})
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

// parseAnswer maps the answer union (string | string[]) to AskAnswer.
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

// runPingInterval spaces run-stream heartbeats.
const runPingInterval = 15 * time.Second

// pumpHub streams live frames until the hub settles, then closes silently:
// the terminal sessionEnd hub event is the
// desktop's run-completion signal, so no extra terminal frame is needed.
// Send errors (client gone) end the pump; the server-side run continues.
func pumpHub(sse *sseStream, hub *run.Hub, since *int64) {
	id, ch, replay := hub.Subscribe(since)
	if ch == nil {
		_ = sse.Send("error", mustJSON(fiber.Map{"type": "error", "error": fiber.Map{"message": "Run already settled."}}))
		return
	}
	defer hub.Unsubscribe(id)
	for _, f := range replay {
		if err := sendFrame(sse, f); err != nil {
			return
		}
	}
	if err := sse.Flush(); err != nil {
		return
	}
	// Heartbeat comment frames keep the connection alive through long
	// silent stretches (a question awaiting the user, slow tools); SSE
	// parsers ignore comments.
	ping := time.NewTicker(runPingInterval)
	defer ping.Stop()
	for {
		var f run.Frame
		select {
		case <-ping.C:
			if err := sse.Ping(); err != nil {
				return
			}
			continue
		case next, ok := <-ch:
			if !ok {
				return
			}
			f = next
		}
		if err := sendFrame(sse, f); err != nil {
			return
		}
		// Drain whatever the run has already queued before paying for a
		// flush: a burst of model deltas then costs one syscall instead of
		// one per delta. The buffer is never left holding a frame — the
		// loop only continues while more frames are immediately available.
		for more := true; more; {
			select {
			case next, ok := <-ch:
				if !ok {
					_ = sse.Flush()
					return
				}
				if err := sendFrame(sse, next); err != nil {
					return
				}
			default:
				more = false
			}
		}
		if err := sse.Flush(); err != nil {
			return
		}
	}
}

// sendFrame translates one hub event into the desktop wire shape:
// {"type": <event>, ...payload}, prefixed with an `id:` line carrying the hub
// sequence so a reconnecting client can resume from ?since=<seq>. Internal-only
// kinds (token accounting, raw provider deltas) are dropped; the desktop
// cannot parse them.
func sendFrame(sse *sseStream, f run.Frame) error {
	name, body, drop := wireFrame(f.Event)
	if drop {
		return nil
	}
	return sse.SendSeqJSON(name, f.Seq, body)
}

// modelStreamPartFrame is the one wire frame emitted per model token, so it
// is a struct rather than a fiber.Map: encoding/json reflects over maps and
// sorts their keys on every call. The part itself is protojson (shared
// schema); the ~2.5µs/frame overhead measured against encoding/json is
// 0.01% of a core at real delta rates — schema uniformity wins.
type modelStreamPartFrame struct {
	Type string          `json:"type"`
	Part json.RawMessage `json:"part"`
}

// wireFrame maps a hub event to its SSE event name and JSON body.
func wireFrame(e loop.Event) (string, any, bool) {
	switch e.Kind {
	case loop.EventSessionStart:
		return "sessionStart", fiber.Map{"type": "sessionStart"}, false
	case loop.EventTurnStart:
		return "turnStart", fiber.Map{"type": "turnStart", "prompt": e.Text}, false
	case loop.EventModelStreamStart:
		return "modelStreamStart", fiber.Map{"type": "modelStreamStart", "turnId": e.Text}, false
	case loop.EventModelStreamPart:
		part, ok := e.Part.(*consolev1.ModelStreamPart)
		if !ok {
			return "", nil, true
		}
		raw, err := protoMarshal.Marshal(part)
		if err != nil {
			return "", nil, true
		}
		return "modelStreamPart", modelStreamPartFrame{Type: "modelStreamPart", Part: json.RawMessage(raw)}, false
	case loop.EventModelStreamEnd:
		var turn any
		if m, ok := e.Message.(loop.AssistantMessage); ok {
			raw, err := protoMarshal.Marshal(loop.AssistantToProto(m))
			if err != nil {
				return "", nil, true
			}
			turn = json.RawMessage(raw)
		}
		return "modelStreamEnd", fiber.Map{"type": "modelStreamEnd", "turnId": e.Text, "turn": turn}, false
	case loop.EventToolExecutionStart:
		calls := make([]*consolev1.ToolCall, 0, len(e.Calls))
		for _, c := range e.Calls {
			calls = append(calls, toolCallToProto(c))
		}
		data, err := marshalProtoList(calls)
		if err != nil {
			return "", nil, true
		}
		return "toolExecutionStart", fiber.Map{"type": "toolExecutionStart", "calls": data}, false
	case loop.EventToolExecutionResult:
		if e.Result == nil {
			return "", nil, true
		}
		raw, err := protoMarshal.Marshal(toolResultToProto(*e.Result))
		if err != nil {
			return "", nil, true
		}
		return "toolExecutionResult", fiber.Map{"type": "toolExecutionResult", "result": json.RawMessage(raw)}, false
	case loop.EventToolExecutionEnd:
		results := make([]*consolev1.ToolResult, 0, len(e.Results))
		for _, r := range e.Results {
			results = append(results, toolResultToProto(r))
		}
		data, err := marshalProtoList(results)
		if err != nil {
			return "", nil, true
		}
		return "toolExecutionEnd", fiber.Map{"type": "toolExecutionEnd", "results": data}, false
	case loop.EventTurnEnd:
		return "turnEnd", fiber.Map{"type": "turnEnd", "turnId": e.Text}, false
	case loop.EventContextUpdate:
		if e.Context == nil {
			return "", nil, true
		}
		raw, err := protoMarshal.Marshal(snapshotToProto(*e.Context))
		if err != nil {
			return "", nil, true
		}
		return "contextUpdate", fiber.Map{"type": "contextUpdate", "context": json.RawMessage(raw)}, false
	case loop.EventSessionEnd:
		return "sessionEnd", fiber.Map{"type": "sessionEnd"}, false
	case loop.EventSessionTitleUpdated:
		return "sessionTitleUpdated", fiber.Map{"type": "sessionTitleUpdated", "title": e.Title}, false
	case loop.EventQueueUpdated:
		data, err := queuedPromptData(e.Queued)
		if err != nil {
			return "", nil, true
		}
		return "queueUpdated", fiber.Map{"type": "queueUpdated", "queuedPrompt": data}, false
	case loop.EventAskQuestion:
		return "askQuestion", fiber.Map{"type": "askQuestion", "request": e.Ask}, false
	case loop.EventBrowserAction:
		return "browserAction", fiber.Map{"type": "browserAction", "request": e.Browser}, false
	case loop.EventPermissionRequest:
		return "permissionRequest", fiber.Map{"type": "permissionRequest", "request": e.Permission}, false
	case loop.EventTodoUpdate:
		return "todoUpdate", fiber.Map{"type": "todoUpdate", "items": nonNilTodos(e.Items), "action": e.Action}, false
	case loop.EventSubagentStart, loop.EventSubagentActivity, loop.EventSubagentEnd:
		body, ok := subagentWire(e)
		if !ok {
			return "", nil, true
		}
		return string(e.Kind), body, false
	case loop.EventError:
		return "error", fiber.Map{"type": "error", "error": fiber.Map{"message": e.Text}}, false
	default:
		// Internal provider kinds (text/thinking/toolCall/usage/toolResult/
		// turnDone) never reach subscribers; drop defensively.
		return "", nil, true
	}
}

// subagentWire flattens a subagent lifecycle payload under its type tag.
func subagentWire(e loop.Event) (any, bool) {
	raw, err := json.Marshal(e.Subagent)
	if err != nil {
		return nil, false
	}
	var body map[string]any
	if err := json.Unmarshal(raw, &body); err != nil {
		return nil, false
	}
	body["type"] = string(e.Kind)
	return body, true
}

// nonNilTodos keeps the todo list as [] (never null) for the desktop card.
func nonNilTodos(items []types.TodoItem) []types.TodoItem {
	if items == nil {
		return []types.TodoItem{}
	}
	return items
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
