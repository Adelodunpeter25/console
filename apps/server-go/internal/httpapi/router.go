// Fiber API surface. Route parity with apps/server/api/src/routes is
// tracked in docs/plan/go-server-rewrite.md (Phase 5).
package httpapi

import (
	"log/slog"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/recover"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/session"
)

type Config struct {
	Store *session.Storage
}

func New(cfg Config) *fiber.App {
	app := fiber.New(fiber.Config{
		AppName:      "console-server-go",
		IdleTimeout:  5 * time.Minute,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 30 * time.Second,
	})
	app.Use(recover.New())

	app.Get("/health", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{"status": "ok"})
	})

	registerSessionRoutes(app, cfg.Store)

	slog.Info("api routes registered")
	return app
}
