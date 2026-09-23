// Fiber API surface. Route parity with apps/server/api/src/routes is
// tracked in docs/plan/go-server-rewrite.md (Phase 5).
package routes

import (
	"log/slog"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/recover"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/memory"
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

func New(cfg Config) (*fiber.App, *run.Service, func()) {
	app := fiber.New(fiber.Config{
		AppName:     "console-server-go",
		IdleTimeout: 5 * time.Minute,
		ReadTimeout: 15 * time.Second,
		// fasthttp's WriteTimeout is an absolute deadline from the start of
		// the response, not an idle timeout — it silently kills SSE run
		// streams once elapsed even while the connection is still active.
		// Agent turns (extended thinking, long tool loops) routinely run
		// past any fixed window, so leave writes unbounded.
		WriteTimeout: 0,
		// Params/body strings otherwise alias fasthttp's reused buffers and
		// get overwritten by later requests; run goroutines outlive their
		// handler, so a mutated session id silently dropped persistence.
		Immutable: true,
	})
	app.Use(recover.New())

	app.Get("/health", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{"status": "ok"})
	})

	registerFavoriteRoutes(app, services.NewFavoriteService(cfg.DB))
	registerPortRoutes(app, cfg.Ports)
	registerSettingsRoutes(app, services.NewSettingsService())
	registerFsRoutes(app, services.NewFsService(), cfg.Watch)
	registerGitRoutes(app, services.NewGitService(), cfg.Watch)
	projects := services.NewProjectService(cfg.DB)
	ptyManager := services.NewPtyManager(cfg.Ports, func(cwd string) string {
		project, err := projects.GetByDir(cwd)
		if err != nil {
			return ""
		}
		return project.ID
	})
	registerTerminalRoutes(app, ptyManager)
	scriptsSvc := services.NewProjectScriptsService(services.NewProjectService(cfg.DB), cfg.Ports)
	registerScriptRoutes(app, scriptsSvc)
	registerProjectRoutes(app, services.NewProjectService(cfg.DB))
	registerUsageRoutes(app, usage.NewService())
	registerAuthRoutes(app, auth.NewAuthService())
	registerProviderRoutes(app, services.NewFavoriteService(cfg.DB))
	runSvc := run.NewService(services.NewSessionService(cfg.DB))
	runSvc.SetNotifications(cfg.Notifications)
	runSvc.SetMemories(memory.NewRegistry(""))
	bashJobs := services.NewBashJobManager(cfg.Ports)
	runSvc.SetBashJobs(bashJobs)
	registerSessionRoutes(app, services.NewSessionService(cfg.DB), runSvc)
	registerWorktreeRoutes(app, services.NewSessionService(cfg.DB), services.NewWorktreeService())
	fffManager := fff.NewManager()
	if fffManager.Enabled() {
		slog.Info("fff file-search index enabled")
	} else if err := fff.Load(); err != nil {
		slog.Warn("fff file-search index disabled, using walk fallback", "error", err)
	} else {
		slog.Warn("fff file-search index disabled, using walk fallback")
	}
	services.SetFffManager(fffManager)
	tools.SetFffManager(fffManager)
	registerRunRoutes(app, runSvc)
	registerMiscRoutes(app, cfg.Notifications)
	registerAssistRoutes(app, services.NewSessionService(cfg.DB), services.NewFsService(), services.NewSkillsService())

	slog.Info("api routes registered")
	// shutdown stops every process the server owns: in-flight agent runs,
	// detached background bash jobs, terminal PTYs, and managed project
	// scripts. Called once, after the HTTP listener drains, so "stop the
	// server" really means stop everything — not just the API.
	shutdown := func() {
		runSvc.AbortAll()
		bashJobs.KillAll()
		ptyManager.KillAll()
		scriptsSvc.StopAll()
	}
	return app, runSvc, shutdown
}
