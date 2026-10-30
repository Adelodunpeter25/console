// Package serve runs the agent server (Fiber API with graceful shutdown).
// It backs both cmd/server and the CONSOLE_SERVE=1 mode of the multi-call
// console binary.
package serve

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
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
	fssvc "github.com/Adelodunpeter25/console/apps/server-go/internal/services/fs"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/services/terminal"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/tunnel"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/tunnel/cfrelay"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/tunnel/opentunnel"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/utils"
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

// startGrantPurge deletes spent pairing grants on a timer, and returns a
// stop func for shutdown.
//
// Pairing grants are one-time: once redeemed (or expired) they can never
// be used, so keeping them only grows the table.
func startGrantPurge(database *db.DB) func() {
	ticker := time.NewTicker(time.Hour)
	done := make(chan struct{})
	go func() {
		for {
			select {
			case <-ticker.C:
				database.PurgePairingGrants(time.Now().UnixMilli())
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

	watch, err := fssvc.NewFsWatchService()
	if err != nil {
		slog.Error("failed to start fs watcher", "error", err)
		return err
	}
	defer watch.Close()

	ports := terminal.NewPortRegistry()
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
	// Plain HTTP is the default for a local daemon, which is fine on a
	// laptop. On a network it is not: every request, including the device
	// credential, travels in the clear. The plan's rule is that a server
	// reachable from off the machine must be encrypted, and the operator can
	// insist on it with CONSOLE_REQUIRE_TLS.
	if !wtCfg.WTEnabled() {
		if strings.EqualFold(strings.TrimSpace(os.Getenv("CONSOLE_REQUIRE_TLS")), "true") {
			slog.Error("CONSOLE_REQUIRE_TLS is set but the server has no TLS configured; refusing to serve plain HTTP")
			return fmt.Errorf("TLS is required but not configured (set CONSOLE_TLS_CERT and CONSOLE_TLS_KEY, or use 'console start --wt')")
		}
		slog.Warn("serving plain HTTP: device credentials travel unencrypted. Use 'console start --wt' for TLS.")
	}
	tokenIssuer, err := auth.NewTokenIssuerStrict()
	if err != nil {
		slog.Error("failed to initialize authentication", "error", err)
		return err
	}
	identity, err := auth.LoadServerIdentity()
	if err != nil {
		slog.Error("failed to load server identity", "error", err)
		return err
	}
	authManager := auth.NewManager(manager, tokenIssuer)
	authManager.Identity = identity
	// No pairing grant is minted here, and none is printed. A grant is a
	// credential, and the daemon's output is a log file — a place credentials
	// leak from. The operator gets one on the terminal instead, where
	// `console pair` and `console status` print it.
	//
	// Spent grants are purged here instead: the table would otherwise grow
	// without bound, one row per pairing attempt.
	manager.PurgePairingGrants(time.Now().UnixMilli())
	app, runs, wtDeps, shutdownAll := routes.New(routes.Config{
		DB: manager, Watch: watch, Ports: ports, Notifications: notifications,
		TokenIssuer: tokenIssuer, Identity: identity, AltSvc: wtAltSvcPort(wtCfg),
	})

	// Deleted chats stay restorable for 7 days, then the backend purges
	// them permanently. The sweep runs once at startup plus once a day.
	stopSweep := startDeletedChatSweep(runs)
	defer stopSweep()

	// Spent pairing grants are purged hourly as well as at startup: a
	// server that hands out a URL every few minutes would otherwise
	// accumulate a row per attempt.
	stopGrantPurge := startGrantPurge(manager)
	defer stopGrantPurge()

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
	routes.RegisterDebugRoutes(app, notifications, wtServer.Stats)
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

	// Remote access: a permanent public HTTPS address through OpenTunnel,
	// created on the first start and resumed on every later one. A tunnel that
	// cannot be reached never stops the server; it keeps retrying.
	stopTunnel := startRemoteAccess(addr)
	defer stopTunnel()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)
	<-quit
	slog.Info("shutting down")
	stopTunnel()
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

// remoteAccessDisabled reports whether the operator turned the tunnel off.
func remoteAccessDisabled() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("CONSOLE_REMOTE"))) {
	case "0", "off", "false", "no", "disabled":
		return true
	}
	return false
}

// startRemoteAccess opens the OpenTunnel tunnel (or the relay, with CONSOLE_RELAY=1) to this server and returns a
// function that closes it. CONSOLE_REMOTE=off skips it; CONSOLE_TUNNEL_API
// points it at another relay.
func startRemoteAccess(listenAddr string) func() {
	if remoteAccessDisabled() {
		slog.Info("remote access is off (CONSOLE_REMOTE)")
		return func() {}
	}
	// The tunnel forwards to this process, so aim it at the loopback port even
	// when the server binds a wildcard address.
	port := strings.TrimPrefix(listenAddr, ":")
	if _, p, err := net.SplitHostPort(listenAddr); err == nil {
		port = p
	}
	target := net.JoinHostPort("127.0.0.1", port)
	dir := filepath.Join(utils.ConsoleStorageDir(), "tunnel")
	if tunnel.RelayEnabled() {
		return startRelay(dir, target)
	}
	manager := &opentunnel.Manager{
		API:    os.Getenv("CONSOLE_TUNNEL_API"),
		Dir:    dir,
		Target: target,
		Logger: slog.Default(),
		OnReady: func(url string) {
			slog.Info("remote access ready", "url", url)
		},
	}
	if err := manager.Start(context.Background()); err != nil {
		slog.Warn("remote access is unavailable", "error", err)
		return func() {}
	}
	slog.Info("remote access starting", "url", manager.URL())
	return manager.Close
}

// startRelay opens the relay Worker tunnel (CONSOLE_RELAY=1). CONSOLE_RELAY_API
// points it at another relay, and CONSOLE_RELAY_NAME asks for a name on the
// first run, such as "peter-dev".
func startRelay(dir, target string) func() {
	manager := &cfrelay.Manager{
		API:    os.Getenv("CONSOLE_RELAY_API"),
		Name:   os.Getenv("CONSOLE_RELAY_NAME"),
		Dir:    dir,
		Target: target,
		Logger: slog.Default(),
		OnReady: func(url string) {
			slog.Info("remote access ready", "url", url)
		},
		OnFailure: func(err error) {
			slog.Warn("remote access stopped", "error", err)
		},
	}
	if err := manager.Start(context.Background()); err != nil {
		slog.Warn("remote access is unavailable", "error", err)
		return func() {}
	}
	slog.Info("remote access starting", "url", manager.URL())
	return manager.Close
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
