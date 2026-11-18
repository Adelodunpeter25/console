// Authenticated automation management + SSE stream (/api/automations/*).
package routes

import (
	"errors"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/services/automations"
)

// RegisterAutomationRoutes wires the authenticated automation REST and SSE
// endpoints onto app.
func RegisterAutomationRoutes(app *fiber.App, store *automations.Service) {
	h := app.Group("/api/automations")

	h.Get("/", func(c *fiber.Ctx) error {
		sessionID := c.Query("sessionId")
		var (
			list []automations.Automation
			err  error
		)
		if sessionID != "" {
			list, err = store.ListForSession(sessionID)
		} else {
			list, err = store.ListAll()
		}
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"success": false, "error": err.Error()})
		}
		if list == nil {
			list = []automations.Automation{}
		}
		return c.JSON(fiber.Map{"success": true, "data": list})
	})

	h.Get("/stream", func(c *fiber.Ctx) error {
		sessionID := c.Query("sessionId")
		return streamSSE(c, func(sse *sseStream) {
			events := store.Subscribe()
			defer store.Unsubscribe(events)

			snapshotOf := func() string {
				var (
					list []automations.Automation
					err  error
				)
				if sessionID != "" {
					list, err = store.ListForSession(sessionID)
				} else {
					list, err = store.ListAll()
				}
				if err != nil || list == nil {
					list = []automations.Automation{}
				}
				return mustJSON(list)
			}

			if err := sse.Send("automations", snapshotOf()); err != nil {
				return
			}
			ticker := time.NewTicker(15 * time.Second)
			defer ticker.Stop()
			for {
				select {
				case ev := <-events:
					if sessionID != "" && ev.SessionID != "" && ev.SessionID != sessionID {
						continue
					}
					if err := sse.Send("automation", mustJSON(ev)); err != nil {
						return
					}
				case <-ticker.C:
					if err := sse.Send("ping", ""); err != nil {
						return
					}
				}
			}
		})
	})

	h.Post("/:id/pause", func(c *fiber.Ctx) error {
		id := c.Params("id")
		existing, err := store.Get(id)
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"success": false, "error": err.Error()})
		}
		if existing == nil {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"success": false, "error": "automation not found"})
		}
		paused, err := store.Pause(id)
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"success": false, "error": err.Error()})
		}
		if paused == nil {
			return c.Status(fiber.StatusConflict).JSON(fiber.Map{"success": false, "error": "automation is " + existing.Status + " and cannot be paused"})
		}
		return c.JSON(fiber.Map{"success": true, "data": paused})
	})

	h.Post("/:id/resume", func(c *fiber.Ctx) error {
		id := c.Params("id")
		existing, err := store.Get(id)
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"success": false, "error": err.Error()})
		}
		if existing == nil {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"success": false, "error": "automation not found"})
		}
		resumed, err := store.Resume(id, time.Now())
		if errors.Is(err, automations.ErrActiveExists) {
			return c.Status(fiber.StatusConflict).JSON(fiber.Map{"success": false, "error": err.Error()})
		}
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"success": false, "error": err.Error()})
		}
		if resumed == nil {
			return c.Status(fiber.StatusConflict).JSON(fiber.Map{"success": false, "error": "automation is " + existing.Status + " and cannot be resumed"})
		}
		return c.JSON(fiber.Map{"success": true, "data": resumed})
	})

	h.Delete("/:id", func(c *fiber.Ctx) error {
		deleted, err := store.Delete(c.Params("id"))
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"success": false, "error": err.Error()})
		}
		if !deleted {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"success": false, "error": "automation not found"})
		}
		return c.JSON(fiber.Map{"success": true, "data": fiber.Map{"deleted": true}})
	})

	app.Get("/api/sessions/:id/automation", func(c *fiber.Ctx) error {
		sessionID := c.Params("id")
		active, err := store.ActiveForSession(sessionID)
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"success": false, "error": err.Error()})
		}
		list, err := store.ListForSession(sessionID)
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"success": false, "error": err.Error()})
		}
		if list == nil {
			list = []automations.Automation{}
		}
		return c.JSON(fiber.Map{"success": true, "data": fiber.Map{"active": active, "automations": list}})
	})
}
