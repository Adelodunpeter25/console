// Server entry point. Fiber API with graceful shutdown; the terminal
// WebSocket and remaining routes land in Phase 5.
package main

import (
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/db"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/routes"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/services"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	manager, err := db.Open(db.OpenOptions{})
	if err != nil {
		slog.Error("failed to open database", "error", err)
		os.Exit(1)
	}
	defer manager.Close()

	watch, err := services.NewFsWatchService()
	if err != nil {
		slog.Error("failed to start fs watcher", "error", err)
		os.Exit(1)
	}
	defer watch.Close()

	ports := services.NewPortRegistry()
	defer ports.CloseAll()
	ports.StartReaper(5 * time.Second)

	app := routes.New(routes.Config{DB: manager, Watch: watch, Ports: ports})

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
}
