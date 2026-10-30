// Fiber API surface.
package routes

import (
	"github.com/Adelodunpeter25/console/apps/server-go/internal/services/git"
	"log/slog"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/recover"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/accounts"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/memory"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/tools"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/auth"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/db"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/fff"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/providers/usage"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/run"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/services"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/services/fs"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/services/mcp"
	projectssvc "github.com/Adelodunpeter25/console/apps/server-go/internal/services/projects"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/services/terminal"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/wt"
)

type Config struct {
	DB            *db.DB
	Watch         *fs.FsWatchService
	Ports         *terminal.PortRegistry
	Notifications *services.NotificationService
	// TokenIssuer mints the API bearer tokens the WT handshake validates.
	// Nil builds one internally (persistent secret, ephemeral fallback).
	TokenIssuer *auth.TokenIssuer
	// Identity is the server's public identity, reported in the pairing
	// responses so a client can confirm which machine it paired with.
	Identity auth.ServerIdentity
	// AltSvc advertises the H3 port (e.g. "443" renders h3=":443").
	// Empty disables the header. Registered before all routes.
	AltSvc string
}

func New(cfg Config) (*fiber.App, *run.Service, wt.Deps, func()) {
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
	if cfg.AltSvc != "" {
		advertise := `h3="` + cfg.AltSvc + `"`
		if !strings.HasPrefix(cfg.AltSvc, ":") {
			advertise = `h3=":` + cfg.AltSvc + `"`
		}
		app.Use(func(c *fiber.Ctx) error {
			c.Set("Alt-Svc", advertise)
			return c.Next()
		})
	}

	app.Get("/health", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{"status": "ok"})
	})
	RegisterVersionRoutes(app)

	// Authentication is protected by default. Pairing is the sole public
	// application endpoint; health/version are registered above this boundary.
	tokenIssuer := cfg.TokenIssuer
	if tokenIssuer == nil {
		tokenIssuer = auth.NewTokenIssuer()
	}
	authManager := auth.NewManager(cfg.DB, tokenIssuer)
	authManager.Identity = cfg.Identity
	RegisterPairingRoutes(app, authManager)
	app.Use(RequireDeviceToken(authManager))

	RegisterFavoriteRoutes(app, services.NewFavoriteService(cfg.DB))
	RegisterPortRoutes(app, cfg.Ports)
	RegisterSettingsRoutes(app, services.NewSettingsService())
	RegisterFsRoutes(app, fs.NewFsService(), cfg.Watch)
	gitSvc := git.NewGitService()
	RegisterGitRoutes(app, gitSvc, cfg.Watch)
	projects := projectssvc.NewProjectService(cfg.DB)
	ptyManager := terminal.NewPtyManager(cfg.Ports, func(cwd string) string {
		project, err := projects.GetByDir(cwd)
		if err != nil {
			return ""
		}
		return project.ID
	})
	RegisterTerminalRoutes(app, ptyManager)
	scriptsSvc := projectssvc.NewProjectScriptsService(projectssvc.NewProjectService(cfg.DB), cfg.Ports)
	RegisterScriptRoutes(app, scriptsSvc)
	RegisterProjectRoutes(app, projectssvc.NewProjectService(cfg.DB), projectssvc.NewProjectBootstrapService())
	// One shared usage instance: auth-driven account mutations invalidate
	// the same cache the usage routes read from.
	usageSvc := usage.NewService()
	RegisterUsageRoutes(app, usageSvc)
	authSvc := auth.NewAuthService()
	authSvc.Usage = usageSvc
	RegisterAuthRoutes(app, authSvc)
	RegisterDeviceRoutes(app, authManager)
	// The old freely-mintable token endpoint is intentionally removed. Device
	// tokens can only be issued through a consumed pairing grant.
	RegisterIntegrationRoutes(app)
	registerProviderRoutes(app, services.NewFavoriteService(cfg.DB))
	runSvc := run.NewService(services.NewSessionService(cfg.DB))
	runSvc.SetNotifications(cfg.Notifications)
	runSvc.SetMemories(memory.NewRegistry(""))
	bashJobs := terminal.NewBashJobManager(cfg.Ports)
	runSvc.SetBashJobs(bashJobs)
	runSvc.SetPorts(cfg.Ports)
	runSvc.SetProjectScripts(scriptsSvc)
	runSvc.SetConsoleServices(projects)
	mcpManager := mcp.NewManager(mcp.NewConfigStore(""), mcp.NewCredentialStore(""), nil)
	runSvc.SetMCP(mcpManager)
	go mcpManager.AutoConnect()
	registerMCPRoutes(app, mcpManager)
	RegisterSessionRoutes(app, services.NewSessionService(cfg.DB), runSvc)
	registerWorktreeRoutes(app, services.NewSessionService(cfg.DB), git.NewWorktreeService())
	// The provider account store backs multi-account support. Configure
	// also migrates the legacy single-credential files; a failure here must
	// not block boot — providers keep working off their legacy files
	// (docs/plan/provider-accounts.md §8).
	if err := accounts.Configure(cfg.DB); err != nil {
		slog.Warn("provider account store unavailable", "error", err)
	}
	fffManager := fff.NewManager()
	if fffManager.Enabled() {
		slog.Info("fff file-search index enabled")
	} else if err := fff.Load(); err != nil {
		slog.Warn("fff file-search index disabled, using walk fallback", "error", err)
	} else {
		slog.Warn("fff file-search index disabled, using walk fallback")
	}
	fs.SetFffManager(fffManager)
	tools.SetFffManager(fffManager)
	RegisterRunRoutes(app, runSvc)
	RegisterPasteRoutes(app, runSvc)
	registerMiscRoutes(app, cfg.Notifications)
	deviceSvc := services.NewDeviceService()
	registerDeviceRoutes(app, deviceSvc)
	registerAssistRoutes(app, services.NewSessionService(cfg.DB), fs.NewFsService(), services.NewSkillsService())

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
		fffManager.CloseAll()
		mcpManager.CloseAll()
		_ = deviceSvc.Close()
	}
	// The WT server drives the same services over QUIC; serve.go owns its
	// listener lifecycle from this bundle.
	return app, runSvc, wt.Deps{
		Issuer:        tokenIssuer,
		Authenticate:  authManager.Authenticate,
		Runs:          runSvc,
		PTY:           ptyManager,
		Ports:         cfg.Ports,
		Watch:         cfg.Watch,
		Git:           gitSvc,
		Notifications: cfg.Notifications,
		Scripts:       scriptsSvc,
		Devices:       deviceSvc,
	}, shutdown
}
