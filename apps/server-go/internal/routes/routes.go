// Fiber API surface.
package routes

import (
	"context"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/services/git"
	"log/slog"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/compress"
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
	"github.com/Adelodunpeter25/console/apps/server-go/internal/services/automations"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/services/fs"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/services/mcp"
	projectssvc "github.com/Adelodunpeter25/console/apps/server-go/internal/services/projects"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/services/stt"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/services/terminal"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/services/webhooks"
)

type Config struct {
	DB            *db.DB
	Watch         *fs.FsWatchService
	Ports         *terminal.PortRegistry
	Notifications *services.NotificationService
	// TokenIssuer mints the device tokens the pairing flow hands out.
	// Nil builds one internally (persistent secret, ephemeral fallback).
	TokenIssuer *auth.TokenIssuer
	// Identity is the server's public identity, reported in the pairing
	// responses so a client can confirm which machine it paired with.
	Identity auth.ServerIdentity
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
		// Speech-to-text uploads are multipart. Fiber's 4 MB default leaves
		// no room for framing around a near-limit 4 MB audio file.
		BodyLimit: 8 << 20,
	})
	app.Use(recover.New())

	app.Use(compress.New(compress.Config{
		Next: func(c *fiber.Ctx) bool {
			// SSE uses SetBodyStreamWriter; the Next hook runs before handlers
			// set Content-Type, so exclude streams by path instead. /run is a
			// POST so plain paths stay compressible.
			path := c.Path()
			return strings.HasSuffix(path, "/stream") ||
				strings.HasSuffix(path, "/watch") ||
				(c.Method() == fiber.MethodPost && strings.HasSuffix(path, "/run"))
		},
	}))

	app.Get("/health", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{"status": "ok"})
	})
	RegisterVersionRoutes(app)

	// Authentication is protected by default. Pairing and webhook ingress are
	// registered above this boundary; webhooks authenticate per-subscription
	// via secret/HMAC rather than a device token.
	var runSvc *run.Service
	RegisterWebhookRoutes(app, func(topic, subID string, headers map[string]string, queryToken string, body []byte) (int, error) {
		if runSvc == nil {
			return 0, run.ErrWebhooksUnavailable
		}
		return runSvc.DeliverWebhook(topic, subID, headers, queryToken, body)
	})
	tokenIssuer := cfg.TokenIssuer
	if tokenIssuer == nil {
		tokenIssuer = auth.NewTokenIssuer()
	}
	authManager := auth.NewManager(cfg.DB, tokenIssuer)
	authManager.Identity = cfg.Identity
	RegisterPairingRoutes(app, authManager)
	app.Use(RequireDeviceToken(authManager))

	RegisterFavoriteRoutes(app, services.NewFavoriteService(cfg.DB))
	draftService := services.NewDraftService(cfg.DB)
	RegisterDraftRoutes(app, draftService)
	RegisterFileCommentRoutes(app, services.NewFileCommentService(cfg.DB))
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
	RegisterRepoTaskRoutes(app, projects, projectssvc.NewProjectBootstrapService(), services.NewSessionService(cfg.DB))
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
	RegisterSttRoutes(app, stt.NewClient(""))
	RegisterGitHubRoutes(app)
	registerProviderRoutes(app, services.NewFavoriteService(cfg.DB))
	runSvc = run.NewService(services.NewSessionService(cfg.DB))
	runSvc.SetNotifications(cfg.Notifications)
	runSvc.SetMemories(memory.NewRegistry(""))
	bashJobs := terminal.NewBashJobManager(cfg.Ports)
	runSvc.SetBashJobs(bashJobs)
	runSvc.SetPorts(cfg.Ports)
	runSvc.SetProjectScripts(scriptsSvc)
	runSvc.SetAutomations(automations.New(cfg.DB.Global()))
	runSvc.SetWebhooks(webhooks.New(cfg.DB.Global()))
	runSvc.SetWebhookBaseURL(defaultWebhookBaseURL)
	runSvc.SetConsoleServices(projects)
	go runSvc.RunAutomationScanner(context.Background())
	mcpManager := mcp.NewManager(mcp.NewConfigStore(""), mcp.NewCredentialStore(""), nil)
	runSvc.SetMCP(mcpManager)
	go mcpManager.AutoConnect()
	registerMCPRoutes(app, mcpManager)
	sessionService := services.NewSessionService(cfg.DB)
	RegisterSessionRoutes(app, sessionService, runSvc)
	RegisterBootstrapRoutes(app, sessionService, runSvc, draftService)
	RegisterWorktreeLaunchRoutes(app, services.NewSessionService(cfg.DB), runSvc)
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
	registerChromeImportRoutes(app)

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
	// Serve owns all listener lifecycle; routes hands back the shutdown that
	// stops everything the server owns.
	return app, runSvc, shutdown
}
