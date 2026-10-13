// Debug routes for load testing and profiling: process/WT snapshots and
// synthetic notification injection for fanout blasts. Every route requires
// a valid API bearer token (the middleware's first production use), but
// like all local-daemon endpoints they assume loopback-adjacent trust.
package routes

import (
	"strings"

	"github.com/gofiber/fiber/v2"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/auth"
	consolev1 "github.com/Adelodunpeter25/console/apps/server-go/internal/gen/console/v1"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/services"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/wt"
)

// RegisterDebugRoutes wires /api/debug/*: snapshot reads wt.Stats through
// the provider so serve.go owns the server object. Exported for route tests.
func RegisterDebugRoutes(app *fiber.App, issuer *auth.TokenIssuer, notifications *services.NotificationService, snapshot func() wt.Stats) {
	h := app.Group("/api/debug", RequireAPIToken(issuer))

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
