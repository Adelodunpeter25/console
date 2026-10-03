// Package serve runs the agent server (Fiber API with graceful shutdown).
// It backs both cmd/server and the CONSOLE_SERVE=1 mode of the multi-call
// console binary.
package serve

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/db"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/gitconfig"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/providers/antigravity"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/providers/claude"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/providers/codex"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/routes"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/run"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/services"
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

	app, runs, shutdownAll := routes.New(routes.Config{DB: manager, Watch: watch, Ports: ports, Notifications: notifications})

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

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)
	<-quit
	slog.Info("shutting down")
	_ = app.ShutdownWithTimeout(5 * time.Second)
	// Stop the server means stop everything it owns: in-flight agent runs,
	// detached background bash jobs, terminal PTYs, and managed project
	// scripts — not just the HTTP listener.
	shutdownAll()
	return nil
}
