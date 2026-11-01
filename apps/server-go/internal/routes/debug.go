// Debug routes for load testing and profiling: process snapshots and
// synthetic notification injection for fanout blasts.
//
// These sit behind the same device-token boundary as every other route —
// they are registered after the middleware, so a caller must already hold a
// paired device credential. (The session counters et al. from the old
// WebTransport debug surface are gone with it.)
package routes

import (
	"runtime"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"

	consolev1 "github.com/Adelodunpeter25/console/apps/server-go/internal/gen/console/v1"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/services"
)

// RegisterDebugRoutes wires /api/debug/*. Statistics are plain process
// snapshots; the WT session counters went away with WebTransport.
func RegisterDebugRoutes(app *fiber.App, notifications *services.NotificationService) {
	h := app.Group("/api/debug")

	h.Get("/stats", func(c *fiber.Ctx) error {
		var mem runtime.MemStats
		runtime.ReadMemStats(&mem)
		return c.JSON(fiber.Map{"success": true, "data": fiber.Map{
			"uptimeMillis": time.Since(startTime).Milliseconds(),
			"goroutines":   runtime.NumGoroutine(),
			"heapAlloc":    mem.HeapAlloc,
			"heapSys":      mem.HeapSys,
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

// startTime marks when the process began serving, for the uptime metric.
var startTime = time.Now()
