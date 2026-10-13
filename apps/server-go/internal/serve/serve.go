// Package serve runs the agent server (Fiber API with graceful shutdown).
// It backs both cmd/server and the CONSOLE_SERVE=1 mode of the multi-call
// console binary.
package serve

import (
	"context"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	_ "net/http/pprof"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/auth"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/db"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/gitconfig"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/providers/antigravity"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/providers/claude"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/providers/codex"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/routes"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/run"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/services"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/wt"
)

// startDeletedChatSweep purges soft-deleted chats past retention once
// immediately, then daily. It returns a stop func for shutdown.
func startDeletedChatSweep(runs *run.Service) func() {
	sweep := func() {
		purged := runs.PurgeExpiredDeletedSessions()
		if len(purged) > 0 {
			slog.Info("purged deleted chats older than 7 days", "count", len(purged))
		}
	}
	sweep()
	ticker := time.NewTicker(24 * time.Hour)
	done := make(chan struct{})
	go func() {
		for {
			select {
			case <-ticker.C:
				sweep()
			case <-done:
				return
			}
		}
	}()
	return func() {
		ticker.Stop()
		close(done)
	}
}

// Run starts the server and blocks until SIGINT/SIGTERM. It returns nil on
// clean shutdown and a non-nil error when startup fails.
func Run() error {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	// Teach git about the GitHub credential helper before anything can spawn:
	// PtyManager caches os.Environ() in a sync.Once on first use, so a
	// Setenv after the first terminal spawn would silently miss every shell.
	// Best-effort — a server without git auth is still perfectly usable, so a
	// failure here is logged rather than fatal.
	if err := gitconfig.Configure(); err != nil {
		slog.Warn("git credential helper not configured", "error", err)
	}

	manager, err := db.Open(db.OpenOptions{})
	if err != nil {
		slog.Error("failed to open database", "error", err)
		return err
	}
	defer manager.Close()

	watch, err := services.NewFsWatchService()
	if err != nil {
		slog.Error("failed to start fs watcher", "error", err)
		return err
	}
	defer watch.Close()

	ports := services.NewPortRegistry()
	defer ports.CloseAll()
	ports.StartReaper(5 * time.Second)
	notifications := services.NewNotificationService()

	// Transport surface for the QUIC migration: WT serves alongside Fiber
	// when TLS is configured, otherwise the server stays legacy-only. One
	// token issuer feeds both the mint route and the WT handshake so a
	// revocation applies everywhere.
	wtCfg, err := wt.ConfigFromEnv()
	if err != nil {
		slog.Error("invalid transport config", "error", err)
		return err
	}
	tokenIssuer := auth.NewTokenIssuer()
	app, runs, wtDeps, shutdownAll := routes.New(routes.Config{
		DB: manager, Watch: watch, Ports: ports, Notifications: notifications,
		TokenIssuer: tokenIssuer, AltSvc: wtAltSvcPort(wtCfg),
	})

	// Deleted chats stay restorable for 7 days, then the backend purges
	// them permanently. The sweep runs once at startup plus once a day.
	stopSweep := startDeletedChatSweep(runs)
	defer stopSweep()

	// Warm the Antigravity model cache in the background when logged in, so
	// the provider catalog serves discovered models (with context windows)
	// from the first client read instead of an empty list. Best-effort:
	// boot never waits on it and failures just leave the cache cold.
	if antigravity.CredentialExists() {
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
			defer cancel()
			antigravity.CachedModels(ctx)
		}()
	}
	if claude.CredentialExists() {
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
			defer cancel()
			claude.CachedModels(ctx)
		}()
	}
	if codex.CredentialExists() {
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
			defer cancel()
			codex.CachedModels(ctx)
		}()
	}

	addr := ":3000"
	if p := os.Getenv("PORT"); p != "" {
		addr = ":" + p
	}

	go func() {
		slog.Info("server listening", "addr", addr)
		if err := app.Listen(addr); err != nil {
			slog.Error("server stopped", "error", err)
			os.Exit(1)
		}
	}()

	// Dual-stack serving when WebTransport is on: Fiber speaks TLS on the
	// TCP half of WTAddr while the WT server takes the UDP half — same
	// port, different sockets, no SO_REUSEPORT needed. The plaintext
	// listener above stays as the dev path either way.
	wtServer := wt.New(wtCfg, wtDeps)
	routes.RegisterDebugRoutes(app, tokenIssuer, notifications, wtServer.Stats)
	startDebugPprof()
	if wtCfg.WTEnabled() {
		go func() {
			slog.Info("server listening (TLS)", "addr", wtCfg.WTAddr)
			if err := app.ListenTLS(wtCfg.WTAddr, wtCfg.TLSCertFile, wtCfg.TLSKeyFile); err != nil {
				slog.Error("TLS listener stopped", "error", err)
				os.Exit(1)
			}
		}()
		if err := wtServer.Start(); err != nil {
			slog.Error("webtransport failed to start", "error", err)
			return err
		}
	}

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)
	<-quit
	slog.Info("shutting down")
	// WT first: closing sessions unblocks every stream handler (chat and
	// watch pumps observe session death even mid-silence), then the HTTP
	// listener drains, then everything the server owns stops.
	_ = wtServer.Close()
	_ = app.ShutdownWithTimeout(5 * time.Second)
	// Stop the server means stop everything it owns: in-flight agent runs,
	// detached background bash jobs, terminal PTYs, and managed project
	// scripts — not just the HTTP listener.
	shutdownAll()
	return nil
}

// wtAltSvcPort renders the H3 port for the Alt-Svc header ("443"), or ""
// when WebTransport is off so nothing is advertised.
func wtAltSvcPort(cfg wt.Config) string {
	if !cfg.WTEnabled() {
		return ""
	}
	if _, port, err := net.SplitHostPort(cfg.WTAddr); err == nil && port != "" {
		return port
	}
	return ""
}

// startDebugPprof serves net/http/pprof when CONSOLE_DEBUG_PPROF_ADDR names
// a loopback address (e.g. 127.0.0.1:6060). Anything else — unset, or a
// non-loopback address — stays off: profiling endpoints never bind
// publicly from this flag.
func startDebugPprof() {
	addr := strings.TrimSpace(os.Getenv("CONSOLE_DEBUG_PPROF_ADDR"))
	if addr == "" {
		return
	}
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		slog.Warn("debug pprof address unparseable, profiling stays off", "addr", addr)
		return
	}
	ip := net.ParseIP(host)
	if ip == nil || !ip.IsLoopback() {
		slog.Warn("debug pprof address is not loopback, profiling stays off", "addr", addr)
		return
	}
	go func() {
		slog.Info("pprof listening (loopback only)", "addr", addr)
		if err := http.ListenAndServe(addr, nil); err != nil {
			slog.Warn("pprof listener stopped", "error", err)
		}
	}()
}
