// Debug routes for load testing and profiling: process/WT snapshots and
// synthetic notification injection for fanout blasts.
//
// These sit behind the same device-token boundary as every other route —
// they are registered after the middleware, so a caller must already hold a
// paired device credential. The old dedicated bearer middleware is gone: two
// token systems meant two revocation states, and the looser one leaked
// through the query string.
package routes

import (
	"strings"

	"github.com/gofiber/fiber/v2"

	consolev1 "github.com/Adelodunpeter25/console/apps/server-go/internal/gen/console/v1"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/services"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/wt"
)

// RegisterDebugRoutes wires /api/debug/*: snapshot reads wt.Stats through
// the provider so serve.go owns the server object. Exported for route tests.
func RegisterDebugRoutes(app *fiber.App, notifications *services.NotificationService, snapshot func() wt.Stats) {
	h := app.Group("/api/debug")

	h.Get("/stats", func(c *fiber.Ctx) error {
		snap := snapshot()
		return c.JSON(fiber.Map{"success": true, "data": fiber.Map{
			"uptimeMillis":  snap.UptimeMillis,
			"goroutines":    snap.Goroutines,
			"heapAlloc":     snap.HeapAlloc,
			"heapSys":       snap.HeapSys,
			"sessionsTotal": snap.SessionsTotal,
			"streamsByKind": snap.StreamsByKind,
		}})
	})

	h.Post("/notify", func(c *fiber.Ctx) error {
		var req struct {
			Kind     string `json:"kind"`
			Title    string `json:"title"`
			Subtitle string `json:"subtitle"`
		}
		if len(c.Body()) > 0 {
			if err := c.BodyParser(&req); err != nil {
				return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"success": false, "error": "Invalid request body."})
			}
		}
		if strings.TrimSpace(req.Kind) == "" {
			req.Kind = "bench"
		}
		if strings.TrimSpace(req.Title) == "" {
			req.Title = "bench notification"
		}
		notifications.Push(&consolev1.NotificationEvent{
			Type: "notification", Kind: req.Kind, Title: req.Title, Subtitle: req.Subtitle,
		})
		return c.JSON(fiber.Map{"success": true, "data": fiber.Map{"pushed": true}})
	})
}
