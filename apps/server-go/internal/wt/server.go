// WebTransport server (QUIC migration, Phase 2): one WT session per device
// over HTTP/3, multiplexing the six stream kinds from the wire schema.
// Runs alongside Fiber until Phase 8 deprecates the legacy streams; unary
// control ops stay on REST in v1.
//
// Authentication happens before the session exists: the CONNECT handler
// validates the API bearer token and rejects with an HTTP status (so
// clients can tell "bad token" from "UDP blocked") before Upgrade. The
// validated device id travels to stream handlers for binding.
package wt

import (
	"crypto/tls"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/quic-go/quic-go"
	"github.com/quic-go/quic-go/http3"
	"github.com/quic-go/quic-go/qlog"
	webtransport "github.com/quic-go/webtransport-go"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/auth"
	consolev1 "github.com/Adelodunpeter25/console/apps/server-go/internal/gen/console/v1"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/run"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/services"
)

// ConnectPath is the WT handshake path. The token rides the query string
// (base64url needs no further encoding there).
const ConnectPath = "/wt/v1/connect"

// Deps are the services the WT handlers drive. All required except where
// noted; the server never mutates them.
type Deps struct {
	Issuer        *auth.TokenIssuer
	Runs          *run.Service
	PTY           *services.PtyManager
	Ports         *services.PortRegistry
	Watch         *services.FsWatchService
	Git           *services.GitService
	Notifications *services.NotificationService
	Scripts       *services.ProjectScriptsService
	Devices       *services.DeviceService
}

// Server is a WebTransport listener. The zero value is not usable;
// construct with New, Start, then Close.
type Server struct {
	cfg  Config
	deps Deps

	mu     sync.Mutex
	h3     *http3.Server
	wt     *webtransport.Server
	conn   net.PacketConn
	serveErr error
}

// New builds an unstarted server.
func New(cfg Config, deps Deps) *Server {
	return &Server{cfg: cfg, deps: deps}
}

// Start binds UDP, loads TLS, and serves in the background. A no-op when
// the config leaves WT disabled, so callers never branch on it.
func (s *Server) Start() error {
	if !s.cfg.WTEnabled() {
		slog.Info("webtransport disabled (no TLS material or legacy mode)")
		return nil
	}
	if s.deps.Issuer == nil || s.deps.Runs == nil {
		return fmt.Errorf("webtransport server requires a token issuer and run service.")
	}
	cert, err := tls.LoadX509KeyPair(s.cfg.TLSCertFile, s.cfg.TLSKeyFile)
	if err != nil {
		return fmt.Errorf("webtransport TLS: %w", err)
	}
	tlsConf := http3.ConfigureTLSConfig(&tls.Config{Certificates: []tls.Certificate{cert}})
	quicConf := &quic.Config{
		KeepAlivePeriod:                15 * time.Second,
		MaxIdleTimeout:                 60 * time.Second,
		MaxIncomingStreams:             1000,
		MaxIncomingUniStreams:          1000,
		InitialStreamReceiveWindow:     512 * 1024,
		InitialConnectionReceiveWindow: 512 * 1024,
		MaxConnectionReceiveWindow:     2 * 1024 * 1024,
		// Datagrams stay enabled at the transport level because the
		// WebTransport handshake requires the SETTINGS bit; the protocol
		// itself never sends any (see the wire schema).
		Tracer: qlog.DefaultConnectionTracer,
	}
	if s.cfg.QlogDir != "" {
		if err := os.MkdirAll(s.cfg.QlogDir, 0o755); err != nil {
			return fmt.Errorf("webtransport qlog dir: %w", err)
		}
		// qlog.DefaultConnectionTracer writes wherever QLOGDIR points;
		// unset means tracing silently stays off.
		_ = os.Setenv("QLOGDIR", s.cfg.QlogDir)
	}
	mux := http.NewServeMux()
	mux.HandleFunc(ConnectPath, s.handleConnect)
	s.h3 = &http3.Server{
		Addr:       s.cfg.WTAddr,
		TLSConfig:  tlsConf,
		Handler:    mux,
		QUICConfig: quicConf,
	}
	s.wt = &webtransport.Server{H3: s.h3}
	udpAddr, err := net.ResolveUDPAddr("udp", s.cfg.WTAddr)
	if err != nil {
		return fmt.Errorf("webtransport address: %w", err)
	}
	s.conn, err = net.ListenUDP("udp", udpAddr)
	if err != nil {
		return fmt.Errorf("webtransport listen: %w", err)
	}
	go func() {
		if err := s.wt.Serve(s.conn); err != nil {
			s.mu.Lock()
			s.serveErr = err
			s.mu.Unlock()
			slog.Error("webtransport serve ended", "error", err)
		}
	}()
	slog.Info("webtransport listening", "addr", s.conn.LocalAddr())
	return nil
}

// Close shuts the listener and all sessions down, waiting for in-flight
// stream handlers. Nil-safe on a never-started server.
func (s *Server) Close() error {
	if s.wt == nil {
		return nil
	}
	return s.wt.Close()
}

// Addr reports the bound UDP address, or nil before Start.
func (s *Server) Addr() net.Addr {
	if s.conn == nil {
		return nil
	}
	return s.conn.LocalAddr()
}

// handleConnect validates the bearer token before the session exists, so a
// bad token answers HTTP 401 while a blocked network simply times out.
func (s *Server) handleConnect(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodConnect {
		http.Error(w, "CONNECT required.", http.StatusMethodNotAllowed)
		return
	}
	if s.deps.Issuer == nil {
		http.Error(w, "Token issuer not configured.", http.StatusInternalServerError)
		return
	}
	deviceID, err := s.deps.Issuer.Validate(r.URL.Query().Get("token"))
	if err != nil {
		http.Error(w, "A valid API token is required.", http.StatusUnauthorized)
		return
	}
	sess, err := s.wt.Upgrade(w, r)
	if err != nil {
		slog.Debug("webtransport upgrade failed", "error", err)
		http.Error(w, "WebTransport upgrade failed.", http.StatusBadRequest)
		return
	}
	slog.Info("webtransport session accepted", "device", deviceID)
	go s.serveSession(sess, deviceID)
}

func (s *Server) serveSession(sess *webtransport.Session, deviceID string) {
	defer func() {
		if r := recover(); r != nil {
			slog.Error("webtransport session panic", "panic", r)
		}
		_ = sess.CloseWithError(0, "")
	}()
	for {
		str, err := sess.AcceptStream(sess.Context())
		if err != nil {
			// Session closed (or failed): every stream handler is already
			// unwinding on the same cause.
			return
		}
		go s.serveStream(str, deviceID)
	}
}

func (s *Server) serveStream(str *webtransport.Stream, deviceID string) {
	defer func() {
		if r := recover(); r != nil {
			slog.Error("webtransport stream panic", "panic", r)
		}
		_ = str.Close()
	}()
	kind, id, err := ReadStreamHeader(str)
	if err != nil {
		// Framing unknown: no error frame is expressible, just close.
		slog.Debug("webtransport bad stream header", "error", err)
		return
	}
	w := &streamWriter{str: str}
	switch kind {
	case consolev1.WtStreamKind_WT_STREAM_KIND_CHAT_RUN:
		s.serveChatRun(str, w, id)
	case consolev1.WtStreamKind_WT_STREAM_KIND_TERMINAL:
		s.serveTerminal(str, w, id)
	case consolev1.WtStreamKind_WT_STREAM_KIND_WATCH:
		s.serveWatch(str, w, id)
	case consolev1.WtStreamKind_WT_STREAM_KIND_PORT_TUNNEL:
		s.serveTunnel(str, w, id)
	case consolev1.WtStreamKind_WT_STREAM_KIND_DEVICE_STREAM:
		s.serveDeviceStream(str, w, id)
	default:
		// Unreachable: the header reader rejects unknown kinds. Kept so a
		// future kind fails loudly here rather than falling silent.
		_ = w.sendErrorf("Stream kind %s is not served yet.", kind)
	}
}
