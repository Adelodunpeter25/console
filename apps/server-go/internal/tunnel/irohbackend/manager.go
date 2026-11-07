//go:build iroh && linux && (amd64 || arm64)

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
// on the console-device/1 ALPN, one request per stream: the client
// half-closes (Finish) after the request, the bridge reads until that end,
// forwards the request to the loopback target, and copies the response
// back before closing.
//
// The package is behind the `iroh` build tag: the iroh-go FFI ships
// static libraries for linux amd64/arm64 only today, so default builds —
// and every CI platform — stay free of it. Build with `-tags iroh` to
// include it; the tunnel switch then picks this backend with
// CONSOLE_RELAY=iroh.
package irohbackend

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	iroh "git.coopcloud.tech/decentral1se/iroh-go"
)

// onlineWait caps how long Start waits for the relay registration.
const onlineWait = 10 * time.Second

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
	stream, err := conn.AcceptBi()
	if err != nil {
		m.log().Debug("iroh: stream refused", "error", err)
		return
	}
	m.serveStream(conn, stream)
}

// serveStream carries one HTTP request over one bidirectional stream.
func (m *Manager) serveStream(conn *iroh.Connection, stream *iroh.BiStream) {
	bridge := newStreamConn(stream.Recv(), stream.Send())
	defer bridge.Close()

	req, err := http.ReadRequest(bufio.NewReader(bridge))
	if err != nil {
		// A stream that never made it to a request: nothing to answer.
		m.log().Debug("iroh: no request on stream", "error", err)
		return
	}
	m.log().Debug("iroh: request", "method", req.Method, "path", req.URL.Path)
	resp, err := m.forward(req)
	if err != nil {
		m.log().Warn("iroh: request did not reach the local server", "error", err)
		_, _ = bridge.Write([]byte("HTTP/1.1 502 Bad Gateway\r\nContent-Length: 0\r\nConnection: close\r\n\r\n"))
		return
	}
	defer resp.Body.Close()
	if err := resp.Write(bridge); err != nil {
		m.log().Debug("iroh: response write failed", "error", err)
	}
}

// forward replays the request against the loopback target.
func (m *Manager) forward(req *http.Request) (*http.Response, error) {
	return roundTrip(req, m.Target)
}
