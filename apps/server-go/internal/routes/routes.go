// Fiber API surface. Route parity with apps/server/api/src/routes is
// tracked in docs/plan/go-server-rewrite.md (Phase 5).
package routes

import (
	"log/slog"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/recover"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/tools"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/auth"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/db"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/fff"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/providers/usage"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/run"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/services"
)

type Config struct {
	DB            *db.DB
	Watch         *services.FsWatchService
	Ports         *services.PortRegistry
	Notifications *services.NotificationService
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

	registerSessionRoutes(app, services.NewSessionService(cfg.DB))
	fffManager := fff.NewManager()
	services.SetFffManager(fffManager)
	tools.SetFffManager(fffManager)
	registerFsRoutes(app, services.NewFsService(), cfg.Watch)
	registerGitRoutes(app, services.NewGitService(), cfg.Watch)
	registerTerminalRoutes(app, services.NewPtyManager())
	registerScriptRoutes(app, services.NewProjectScriptsService(services.NewProjectService(cfg.DB)))
	registerProjectRoutes(app, services.NewProjectService(cfg.DB))
	registerFavoriteRoutes(app, services.NewFavoriteService(cfg.DB))
	registerPortRoutes(app, cfg.Ports)
	registerSettingsRoutes(app, services.NewSettingsService())
	registerUsageRoutes(app, usage.NewService())
	registerAuthRoutes(app, auth.NewAuthService())
	registerProviderRoutes(app, services.NewFavoriteService(cfg.DB))
	registerRunRoutes(app, run.NewService(services.NewSessionService(cfg.DB)))
	registerMiscRoutes(app, cfg.Notifications)
	registerAssistRoutes(app, services.NewSessionService(cfg.DB), services.NewFsService(), services.NewSkillsService())

	slog.Info("api routes registered")
	return app
}
