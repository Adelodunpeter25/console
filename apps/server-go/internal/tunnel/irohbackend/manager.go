//go:build iroh && ((linux && (amd64 || arm64)) || (darwin && (amd64 || arm64)) || (windows && amd64))

// Package irohbackend serves the local HTTP target over an iroh endpoint.
//
// Instead of dialing out to a public relay and speaking its frame protocol
// (the cfrelay shape), the server binds an iroh endpoint. Clients reach it
// by its node ID — the public half of a 32-byte keypair kept on disk —
// over QUIC. Every byte between the endpoints is encrypted by iroh itself;
// no relay in the path can read traffic. n0's public relays still exist
// behind the scenes, but only to broker first handshakes and to carry
// traffic when hole-punching fails.
//
// The API itself is plain HTTP/1.1 carried over bidirectional QUIC streams
// on the console-device/1 ALPN. One stream is one TCP connection to the
// loopback target, bridged byte for byte, so keep-alive, SSE and WebSocket
// traffic behave as they do over TCP. A client that sends one request and
// half-closes (Finish) gets the response and then EOF.
//
// The package is behind the `iroh` build tag: the iroh-go FFI ships
// static libraries for linux, macOS and windows-amd64 only (see the
// Adelodunpeter25/iroh-go fork), so default builds stay free of it. Build with `-tags iroh` to
// include it; the tunnel switch then picks this backend with
// CONSOLE_RELAY=iroh.
package irohbackend

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"sync"
	"time"

	iroh "github.com/Adelodunpeter25/iroh-go"
)

// onlineWait caps how long Start waits for the relay registration.
const onlineWait = 10 * time.Second

// dialTimeout bounds the dial to the loopback target.
const dialTimeout = 5 * time.Second

// halfCloseGrace is how long one direction may stay open after the other
// has ended.
const halfCloseGrace = 30 * time.Second

// Manager keeps an iroh endpoint alive for the life of the server.
type Manager struct {
	// Dir is where the endpoint keypair is stored.
	Dir string
	// Target is the local address traffic is forwarded to, as host:port.
	Target string
	// Logger receives events. Optional.
	Logger *slog.Logger
	// OnReady is called once the endpoint is bound and serving.
	OnReady func(nodeID string)

	store  Store
	mu     sync.Mutex
	nodeID string
	done   chan struct{}
	closed bool
}

func (m *Manager) log() *slog.Logger {
	if m.Logger != nil {
		return m.Logger
	}
	return slog.Default()
}

// NodeID is this endpoint's public identity, or "" until bound.
func (m *Manager) NodeID() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.nodeID
}

// Start binds the endpoint, stores the node ID and serves until Close.
// It returns once the endpoint is bound and announcing: unlike the relay's
// register call, no round-trip has to complete before peers can dial.
func (m *Manager) Start(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if m.Target == "" {
		return errors.New("iroh: no target configured")
	}
	if m.Dir == "" {
		return errors.New("iroh: no storage directory configured")
	}
	if m.mu.Lock(); m.done != nil {
		m.mu.Unlock()
		return errors.New("iroh: manager already started")
	}
	m.mu.Unlock()

	m.store = Store{Dir: m.Dir}
	key, info, err := m.store.LoadOrCreate()
	if err != nil {
		return err
	}

	preset := iroh.PresetN0()
	opts := iroh.EndpointOptions{
		Preset:    &preset,
		Alpns:     &[][]byte{[]byte(ALPN)},
		SecretKey: &key,
	}
	endpoint, err := iroh.EndpointBind(opts)
	if err != nil {
		return fmt.Errorf("iroh: bind endpoint: %w", err)
	}

	nodeID := endpoint.Id().String()
	// Store the node ID before the ready signal so a CLI reading the dir
	// never sees readiness ahead of the file.
	if info.NodeID != nodeID {
		info.NodeID = nodeID
		if err := m.store.SaveInfo(info); err != nil {
			return err
		}
	}
	m.mu.Lock()
	m.nodeID = nodeID
	m.done = make(chan struct{})
	done := m.done
	m.mu.Unlock()

	// Online blocks until the home relay has registered the endpoint; a
	// dial before that cannot find us. It can hang with no network, so cap
	// it and serve anyway (direct peers still connect).
	online := make(chan struct{})
	go func() { endpoint.Online(); close(online) }()
	select {
	case <-online:
	case <-time.After(onlineWait):
		m.log().Warn("iroh: relay not reachable yet, serving without it")
	}

	m.log().Info("iroh endpoint serving", "node_id", nodeID)
	if m.OnReady != nil {
		m.OnReady(nodeID)
	}

	m.acceptLoop(ctx, endpoint, done)
	return nil
}

// Close marks the manager closed and waits for Start to unwind.
func (m *Manager) Close() {
	m.mu.Lock()
	if m.done == nil || m.closed {
		m.mu.Unlock()
		return
	}
	m.closed = true
	done := m.done
	m.mu.Unlock()
	<-done
}

// acceptLoop takes incoming connections until ctx ends or Close wins.
func (m *Manager) acceptLoop(ctx context.Context, endpoint *iroh.Endpoint, done chan struct{}) {
	defer close(done)
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		select {
		case <-done:
			return
		default:
		}
		// AcceptNext is a blocking FFI call with no cancellation: while it
		// waits, a Close request is answered the next time the loop spins.
		incoming := endpoint.AcceptNext()
		if incoming == nil || *incoming == nil {
			return
		}
		go m.handleIncoming(*incoming)
	}
}

func (m *Manager) handleIncoming(incoming *iroh.Incoming) {
	accepting, err := incoming.Accept()
	if err != nil {
		m.log().Debug("iroh: incoming rejected", "error", err)
		return
	}
	conn, err := accepting.Connect()
	if err != nil {
		// A wrong-ALPN or otherwise unwanted peer: nothing to serve.
		m.log().Debug("iroh: connect refused", "error", err)
		return
	}
	// A client opens one stream per TCP-like connection; serve every one.
	for {
		stream, err := conn.AcceptBi()
		if err != nil {
			m.log().Debug("iroh: connection ended", "error", err)
			return
		}
		go m.serveStream(conn, stream)
	}
}

// serveStream bridges one bidirectional stream to one loopback TCP
// connection, byte for byte. It is deliberately not HTTP-aware: keep-alive,
// SSE and WebSocket upgrades all ride on the one connection, exactly as
// they would over TCP. The client half-closing the stream ends the request
// direction; the target closing ends the response direction.
func (m *Manager) serveStream(conn *iroh.Connection, stream *iroh.BiStream) {
	bridge := newStreamConn(stream.Recv(), stream.Send())
	defer bridge.Close()

	upstream, err := net.DialTimeout("tcp", m.Target, dialTimeout)
	if err != nil {
		m.log().Warn("iroh: could not reach the local server", "error", err)
		return
	}
	defer upstream.Close()

	done := make(chan struct{}, 2)
	go func() {
		_, _ = io.Copy(upstream, bridge)
		if tcp, ok := upstream.(*net.TCPConn); ok {
			_ = tcp.CloseWrite()
		}
		done <- struct{}{}
	}()
	go func() {
		_, _ = io.Copy(bridge, upstream)
		_ = bridge.HalfClose()
		done <- struct{}{}
	}()
	<-done
	// One side ended; give the other a bounded time to drain.
	select {
	case <-done:
	case <-time.After(halfCloseGrace):
	}
}
