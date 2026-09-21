// Project script routes (/projects/:projectId/scripts/*). Port of
// apps/server/api/src/routes/project-scripts.ts.
package routes

import (
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/services"
)

func registerScriptRoutes(app *fiber.App, scripts *services.ProjectScriptsService) {
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

	app.Get("/api/projects/:projectId/scripts", func(c *fiber.Ctx) error {
		result, err := scripts.List(c.Params("projectId"))
		if err != nil {
			return fail(c, fiber.StatusBadRequest, err)
		}
		return ok(c, result)
	})

	app.Post("/api/projects/:projectId/scripts/:scriptId/runs", func(c *fiber.Ctx) error {
		run, err := scripts.Run(c.Params("projectId"), c.Params("scriptId"))
		if err != nil {
			return fail(c, fiber.StatusBadRequest, err)
		}
		return ok(c, run, fiber.StatusCreated)
	})

	app.Get("/api/projects/:projectId/scripts/runs", func(c *fiber.Ctx) error {
		return ok(c, scripts.ListRuns(c.Params("projectId")))
	})

	app.Get("/api/projects/:projectId/scripts/runs/:runId", func(c *fiber.Ctx) error {
		run := scripts.GetRun(c.Params("projectId"), c.Params("runId"))
		if run == nil {
			return fail(c, fiber.StatusNotFound, errNotFound("Run not found."))
		}
		return ok(c, run)
	})

	app.Post("/api/projects/:projectId/scripts/runs/:runId/stop", func(c *fiber.Ctx) error {
		if !scripts.Stop(c.Params("projectId"), c.Params("runId")) {
			return fail(c, fiber.StatusNotFound, errNotFound("Run not found or already stopped."))
		}
		return ok(c, fiber.Map{"stopped": true})
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
			// End the stream when the run stops running, like the TS
			// 100ms status poll.
			poll := time.NewTicker(100 * time.Millisecond)
			defer poll.Stop()
			for {
				select {
				case event := <-events:
					if err := sse.Send(event.Type, mustJSON(event)); err != nil {
						return
					}
				case <-poll.C:
					if !scripts.IsRunning(projectID, runID) {
						return
					}
				}
			}
		})
	})
}

type notFoundError string

func (e notFoundError) Error() string { return string(e) }

func errNotFound(msg string) error { return notFoundError(msg) }
