// Project script routes (/projects/:projectId/scripts/*).
//
// Sixth domain on the shared protobuf schema (desktop + server only; mobile
// has no scripts client). List, run, and stop payloads are built from
// console.v1 generated types with byte-identical output, and the SSE stream
// frames move to the ScriptRunEvent oneof ({"status":{...}} instead of
// {"type":"status",...}). Dropped from the wire: the exit signal, unread by
// every client. An unset shortcut encodes as "" rather than null (see the
// proto comments).
package routes

import (
	"encoding/json"
	"time"

	"github.com/gofiber/fiber/v2"
	"google.golang.org/protobuf/proto"

	consolev1 "github.com/Adelodunpeter25/console/apps/server-go/internal/gen/console/v1"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/services"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/types"
)

func scriptToProto(s types.ProjectScript) *consolev1.ProjectScript {
	shortcut := ""
	if s.Shortcut != nil {
		shortcut = *s.Shortcut
	}
	return &consolev1.ProjectScript{
		Id: s.ID, Label: s.Label, Command: s.Command,
		Shortcut: &shortcut, Persistent: s.Persistent,
	}
}

func scriptRunToProto(r types.ScriptRun) *consolev1.ScriptRun {
	out := &consolev1.ScriptRun{
		RunId: r.RunID, ProjectId: r.ProjectID, ScriptId: r.ScriptID,
		Label: r.Label, Persistent: r.Persistent,
		Status: string(r.Status), StartedAt: r.StartedAt,
		Stdout: r.Stdout, Stderr: r.Stderr,
	}
	if r.EndedAt != nil {
		out.EndedAt = r.EndedAt
	}
	if r.ExitCode != nil {
		out.ExitCode = func() *int32 { v := int32(*r.ExitCode); return &v }()
	}
	return out
}

func scriptRunsToProto(runs []types.ScriptRun) []*consolev1.ScriptRun {
	out := make([]*consolev1.ScriptRun, 0, len(runs))
	for _, r := range runs {
		out = append(out, scriptRunToProto(r))
	}
	return out
}

// scriptRunEventToProto converts a stream event to the canonical wire type
// plus the SSE event name previously carried in the "type" field.
func scriptRunEventToProto(e types.ScriptRunEvent) (*consolev1.ScriptRunEvent, string) {
	switch e.Type {
	case "output":
		return &consolev1.ScriptRunEvent{Event: &consolev1.ScriptRunEvent_Output{
			Output: &consolev1.ScriptOutputEvent{Stream: e.Stream, Text: e.Text},
		}}, "output"
	case "exit":
		out := &consolev1.ScriptExitEvent{Status: string(e.Status)}
		if e.ExitCode != nil {
			out.ExitCode = func() *int32 { v := int32(*e.ExitCode); return &v }()
		}
		return &consolev1.ScriptRunEvent{Event: &consolev1.ScriptRunEvent_Exit{Exit: out}}, "exit"
	default:
		return &consolev1.ScriptRunEvent{Event: &consolev1.ScriptRunEvent_Status{
			Status: &consolev1.ScriptStatusEvent{Status: string(e.Status)},
		}}, "status"
	}
}

func RegisterScriptRoutes(app *fiber.App, scripts *services.ProjectScriptsService) {
	ok := func(c *fiber.Ctx, data any, status ...int) error {
		code := fiber.StatusOK
		if len(status) > 0 {
			code = status[0]
		}
		return c.Status(code).JSON(fiber.Map{"success": true, "data": data})
	}
	fail := func(c *fiber.Ctx, status int, err error) error {
		return c.Status(status).JSON(fiber.Map{"success": false, "error": err.Error()})
	}
	sendProto := func(c *fiber.Ctx, msg proto.Message, status ...int) error {
		raw, err := protoMarshal.Marshal(msg)
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"success": false, "error": "encode failed"})
		}
		return ok(c, json.RawMessage(raw), status...)
	}

	app.Get("/api/projects/:projectId/scripts", func(c *fiber.Ctx) error {
		result, err := scripts.ListIn(c.Params("projectId"), c.Query("cwd"))
		if err != nil {
			return fail(c, fiber.StatusBadRequest, err)
		}
		protos := make([]*consolev1.ProjectScript, 0, len(result.Scripts))
		for _, s := range result.Scripts {
			protos = append(protos, scriptToProto(s))
		}
		return sendProto(c, &consolev1.ProjectScriptsResult{
			ProjectId: result.ProjectID, Scripts: protos, Source: result.Source,
		})
	})

	app.Post("/api/projects/:projectId/scripts/:scriptId/runs", func(c *fiber.Ctx) error {
		run, err := scripts.RunIn(c.Params("projectId"), c.Params("scriptId"), c.Query("cwd"))
		if err != nil {
			return fail(c, fiber.StatusBadRequest, err)
		}
		return sendProto(c, scriptRunToProto(run), fiber.StatusCreated)
	})

	app.Get("/api/projects/:projectId/scripts/runs", func(c *fiber.Ctx) error {
		data, err := marshalProtoList(scriptRunsToProto(scripts.ListRuns(c.Params("projectId"))))
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"success": false, "error": "encode failed"})
		}
		return ok(c, data)
	})

	app.Get("/api/projects/:projectId/scripts/runs/:runId", func(c *fiber.Ctx) error {
		run := scripts.GetRun(c.Params("projectId"), c.Params("runId"))
		if run == nil {
			return fail(c, fiber.StatusNotFound, errNotFound("Run not found."))
		}
		return sendProto(c, scriptRunToProto(*run))
	})

	app.Post("/api/projects/:projectId/scripts/runs/:runId/stop", func(c *fiber.Ctx) error {
		if !scripts.Stop(c.Params("projectId"), c.Params("runId")) {
			return fail(c, fiber.StatusNotFound, errNotFound("Run not found or already stopped."))
		}
		return sendProto(c, &consolev1.StopScriptRunResponse{Stopped: true})
	})

	app.Get("/api/projects/:projectId/scripts/runs/:runId/stream", func(c *fiber.Ctx) error {
		projectID, runID := c.Params("projectId"), c.Params("runId")
		if scripts.GetRun(projectID, runID) == nil {
			return fail(c, fiber.StatusNotFound, errNotFound("Run not found."))
		}
		return streamSSE(c, func(sse *sseStream) {
			events, subscribed := scripts.Subscribe(projectID, runID)
			if !subscribed {
				return
			}
			defer scripts.Unsubscribe(projectID, runID, events)
			sendEvent := func(event types.ScriptRunEvent) error {
				msg, name := scriptRunEventToProto(event)
				raw, err := protoMarshal.Marshal(msg)
				if err != nil {
					return err
				}
				return sse.Send(name, string(raw))
			}
			// End the stream when the run stops running, detected by a
			// 100ms status poll.
			poll := time.NewTicker(100 * time.Millisecond)
			defer poll.Stop()
			for {
				select {
				case event := <-events:
					if err := sendEvent(event); err != nil {
						return
					}
				case <-poll.C:
					if !scripts.IsRunning(projectID, runID) {
						// The stop/exit path may still have a queued final
						// event that select has not picked up yet; drain and
						// send it so the client always sees the terminal
						// status before the stream ends.
						for {
							select {
							case event := <-events:
								if err := sendEvent(event); err != nil {
									return
								}
							default:
								return
							}
						}
					}
				}
			}
		})
	})
}

type notFoundError string

func (e notFoundError) Error() string { return string(e) }

func errNotFound(msg string) error { return notFoundError(msg) }
