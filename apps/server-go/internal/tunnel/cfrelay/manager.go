// Package cfrelay gives a local server a permanent public HTTPS address
// through the relay Worker in apps/relay, hosted on workers.dev.
//
// The server dials out to the relay and keeps one WebSocket open; visitors
// hitting https://<relay>/<name>/... are carried down that socket and replayed
// against the local target. The name and token are stored, so every restart
// resumes the same URL. The relay sees traffic in the clear: it terminates TLS.
package cfrelay

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math/rand"
	"net/http"
	"net/url"
	"sync"
	"time"
)

// DefaultAPI is the hosted relay.
const DefaultAPI = "https://relay.adelodunpeter.workers.dev"

const (
	backoffBase      = 250 * time.Millisecond
	backoffMax       = 30 * time.Second
	stableConnection = 30 * time.Second
)

// Manager keeps one relay tunnel open for the life of the server.
type Manager struct {
	// API is the relay's address. Empty means DefaultAPI.
	API string
	// Dir is where the identity is stored.
	Dir string
	// Target is the local address traffic is forwarded to, as host:port.
	Target string
	// Name is the name to ask for on first registration. Optional; empty
	// lets the relay generate one. Ignored once an identity is stored.
	Name string
	// HTTPClient performs the registration call. Optional.
	HTTPClient *http.Client
	// Logger receives events. Optional.
	Logger *slog.Logger

	// OnReady is called once the tunnel has its public URL.
	OnReady func(url string)
	// OnFailure is called when the manager gives up.
	OnFailure func(err error)

	mu       sync.Mutex
	identity *Identity
	api      *url.URL
	stopped  chan struct{}
	cancel   context.CancelFunc
}

func (m *Manager) log() *slog.Logger {
	if m.Logger != nil {
		return m.Logger
	}
	return slog.Default()
}

// URL is the tunnel's public address, or "" until one exists.
func (m *Manager) URL() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.identity == nil {
		return ""
	}
	return m.identity.URL
}

// Start registers on the first run, then serves in the background until
// Close. It returns once the URL is known.
func (m *Manager) Start(ctx context.Context) error {
	if m.Target == "" {
		return errors.New("cfrelay: no target configured")
	}
	if m.Dir == "" {
		return errors.New("cfrelay: no storage directory configured")
	}
	base := m.API
	if base == "" {
		base = DefaultAPI
	}
	api, err := url.Parse(base)
	if err != nil || api.Host == "" {
		return fmt.Errorf("cfrelay: invalid relay address %q", base)
	}
	m.api = api

	store := Store{Dir: m.Dir}
	identity, err := store.Load()
	if err != nil {
		m.log().Warn("the saved relay identity is unreadable, registering again", "error", err)
		_ = store.Clear()
		identity = nil
	}
	// A stored identity for another relay is no use here.
	if identity != nil && identity.API != api.String() {
		identity = nil
	}
	if identity == nil {
		if identity, err = m.register(ctx, store); err != nil {
			return err
		}
	}
	m.mu.Lock()
	m.identity = identity
	m.stopped = make(chan struct{})
	runCtx, cancel := context.WithCancel(context.Background())
	m.cancel = cancel
	m.mu.Unlock()

	go m.supervise(runCtx, store)
	return nil
}

// Close stops the tunnel and waits for it to wind down.
func (m *Manager) Close() {
	m.mu.Lock()
	cancel, stopped := m.cancel, m.stopped
	m.mu.Unlock()
	if cancel == nil {
		return
	}
	cancel()
	<-stopped
}

func (m *Manager) supervise(ctx context.Context, store Store) {
	defer close(m.stopped)

	announced := false
	attempt := 0
	for ctx.Err() == nil {
		m.mu.Lock()
		id := m.identity
		m.mu.Unlock()

		if !announced {
			announced = true
			if m.OnReady != nil {
				m.OnReady(id.URL)
			}
		}

		connectedAt := time.Time{}
		sess := newSession(sessionConfig{
			api: m.api, name: id.Name, token: id.Token, target: m.Target, log: m.log(),
			onAttached: func() {
				connectedAt = time.Now()
				m.log().Info("relay connected", "url", id.URL)
			},
		})
		end := sess.run(ctx)

		switch end.kind {
		case endShutdown:
			return
		case endFatal:
			// The relay no longer accepts our token, so the name is lost.
			m.log().Warn("the relay no longer accepts this tunnel, registering a new one", "error", end.err)
			_ = store.Clear()
			next, err := m.registerLoop(ctx, store, &attempt)
			if err != nil {
				return
			}
			m.mu.Lock()
			m.identity = next
			m.mu.Unlock()
			announced = false
			continue
		default:
			m.log().Info("relay disconnected, reconnecting", "reason", end.err)
		}
		if !connectedAt.IsZero() && time.Since(connectedAt) >= stableConnection {
			attempt = 0
		}
		if !m.wait(ctx, &attempt) {
			return
		}
	}
}

func (m *Manager) registerLoop(ctx context.Context, store Store, attempt *int) (*Identity, error) {
	for ctx.Err() == nil {
		id, err := m.register(ctx, store)
		if err == nil {
			return id, nil
		}
		if errors.Is(err, errNameTaken) || errors.Is(err, errBadName) {
			if m.OnFailure != nil {
				m.OnFailure(err)
			}
			return nil, err
		}
		m.log().Warn("could not register with the relay", "error", err)
		if !m.wait(ctx, attempt) {
			break
		}
	}
	return nil, ctx.Err()
}

func (m *Manager) wait(ctx context.Context, attempt *int) bool {
	delay := backoffBase << min(*attempt, 7)
	if delay > backoffMax {
		delay = backoffMax
	}
	*attempt++
	delay = delay/2 + time.Duration(rand.Int63n(int64(delay/2)+1))
	t := time.NewTimer(delay)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-t.C:
		return true
	}
}
