package hostc

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

// backoff is the reconnect delay schedule: 250ms, 500ms, 1s … capped at 10s,
// with jitter so several clients do not reconnect in lockstep.
//
// The attempt count resets only after a connection has been stable, so a
// server that accepts a connection and drops it straight away does not turn
// into a reconnect every 250 milliseconds.
type backoff struct {
	attempt     int
	connectedAt time.Time
}

const (
	backoffBase        = 250 * time.Millisecond
	backoffMax         = 10 * time.Second
	stableConnection   = 30 * time.Second
	backoffJitterRatio = 0.2
)

// next returns the next delay.
func (b *backoff) next() time.Duration {
	b.attempt++
	delay := backoffMax
	// The shift is bounded so the delay cannot overflow into a negative one.
	if b.attempt <= 8 {
		delay = backoffBase << (b.attempt - 1)
	}
	if delay > backoffMax {
		delay = backoffMax
	}
	jitter := 1 + (rand.Float64()*2-1)*backoffJitterRatio
	return time.Duration(float64(delay) * jitter)
}

// connected marks a healthy connection.
func (b *backoff) connected() { b.connectedAt = time.Now() }

// disconnected resets the schedule when the connection had been up long
// enough to count as healthy.
func (b *backoff) disconnected() {
	if time.Since(b.connectedAt) >= stableConnection {
		b.attempt = 0
	}
}

// Manager keeps a tunnel open across connection drops.
//
// The public URL survives both a dropped connection and a restart: the
// tunnel's id and connect token are persisted, and a reconnect resumes the
// same tunnel rather than creating a new one. Only when that tunnel has
// expired does a new one have to be created, and only then does the URL
// change — a client paired with the old URL has to pair again.
type Manager struct {
	// ServerURL is the tunnel server, e.g. https://hostc.dev.
	ServerURL string
	// Target is the local origin traffic is forwarded to, e.g.
	// http://127.0.0.1:3000.
	Target *url.URL
	// Store persists the tunnel. When nil the URL cannot survive a restart.
	Store Store
	// HTTPClient performs the local requests. Optional.
	HTTPClient *http.Client
	// Logger receives the tunnel's events. Optional.
	Logger *slog.Logger

	// OnURLChanged is called when the public URL changes, which happens only
	// when the tunnel expired and a new one had to be created. Clients
	// paired with the old URL have to pair again.
	OnURLChanged func(url string)
	// OnReconnect is called after a dropped connection is re-established.
	OnReconnect func(url string)
	// OnFailure is called when the manager gives up, which is when the
	// server needs a newer client or another client took the tunnel over.
	OnFailure func(err error)

	mu     sync.Mutex
	tunnel *Tunnel
	client *Client
	closed bool

	closeOnce sync.Once
	stopped   chan struct{}
}

// Start opens the tunnel and keeps it open until Close.
//
// It returns once the first connection is up, so a caller can read the
// public URL straight away. A tunnel that cannot be reached yet is retried
// in the background rather than failing the caller's startup.
func (m *Manager) Start(ctx context.Context) error {
	if m.Target == nil {
		return fmt.Errorf("hostc: no target configured")
	}
	m.stopped = make(chan struct{})

	// Resume the persisted tunnel first, so a restart keeps the URL. A
	// tunnel the server no longer knows is replaced; anything else is
	// transient and must not cost the URL.
	if m.Store != nil {
		saved, err := m.Store.Load()
		if err != nil {
			m.log("could not read the saved tunnel", "error", err)
		} else if saved != nil {
			client := m.newClient()
			if err := client.Connect(ctx, saved); err == nil {
				m.adopt(client, saved, false)
				go m.supervise(ctx)
				return nil
			} else if !errors.Is(err, ErrTunnelGone) {
				m.log("could not resume the saved tunnel", "error", err)
				m.fail(err)
				return err
			} else {
				m.log("the saved tunnel is gone", "error", err)
			}
		}
	}

	tunnel, err := m.createTunnel(ctx)
	if err != nil {
		m.fail(err)
		return err
	}
	client := m.newClient()
	if err := client.Connect(ctx, tunnel); err != nil {
		m.fail(err)
		return err
	}
	m.adopt(client, tunnel, true)
	go m.supervise(ctx)
	return nil
}

// PublicURL is the address clients should use, or "" when there is no tunnel.
func (m *Manager) PublicURL() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.tunnel == nil {
		return ""
	}
	return m.tunnel.URL
}

// Stop ends the supervising and drops the connection, leaving the tunnel
// where the server can serve it again.
//
// The server keeps a dropped tunnel for a grace period, so a restart can
// resume the same public URL and a client paired with it stays paired. The
// URL is public in the meantime, pointing at a server that is not running.
//
// Use Release to give the tunnel up instead.
func (m *Manager) Stop() error {
	var err error
	m.closeOnce.Do(func() {
		m.mu.Lock()
		m.closed = true
		client := m.client
		m.mu.Unlock()
		if client != nil {
			err = client.Drop()
		}
		if m.stopped != nil {
			close(m.stopped)
		}
	})
	return err
}

// Release ends the supervising and releases the tunnel, so the public URL
// stops working at once and the next start has to mint a new one.
func (m *Manager) Release() error {
	var err error
	m.closeOnce.Do(func() {
		m.mu.Lock()
		m.closed = true
		client := m.client
		m.mu.Unlock()
		if client != nil {
			err = client.Close()
		}
		if m.Store != nil {
			if storeErr := m.Store.Clear(); storeErr != nil {
				m.log("could not remove the tunnel file", "error", storeErr)
			}
		}
		if m.stopped != nil {
			close(m.stopped)
		}
	})
	return err
}

// supervise keeps the connection up, reconnecting to the same tunnel until
// it is gone.
func (m *Manager) supervise(ctx context.Context) {
	schedule := &backoff{}
	for {
		m.mu.Lock()
		client := m.client
		m.mu.Unlock()
		if client == nil {
			return
		}
		<-client.Done()
		code := client.CloseCode()

		if m.isClosed() || ctx.Err() != nil {
			return
		}
		if code == CloseReplaced {
			// Another client took the tunnel over. Reconnecting would take
			// it back and start a tug of war.
			m.fail(fmt.Errorf("hostc: another client took over the tunnel"))
			return
		}
		m.log("the tunnel connection dropped", "code", code)
		schedule.disconnected()

		delay := schedule.next()
		m.log("reconnecting", "in", delay.String())
		if !m.sleep(ctx, delay) {
			return
		}
		if !m.reconnect(ctx) {
			return
		}
		schedule.connected()
	}
}

// reconnect re-establishes the connection, minting a new tunnel only when the
// one we had is gone. It reports whether the manager should keep going.
func (m *Manager) reconnect(ctx context.Context) bool {
	m.mu.Lock()
	tunnel := m.tunnel
	m.mu.Unlock()

	if tunnel != nil {
		client := m.newClient()
		err := client.Connect(ctx, tunnel)
		if err == nil {
			m.adopt(client, tunnel, false)
			m.reconnected(tunnel.URL)
			return true
		}
		if !errors.Is(err, ErrTunnelGone) {
			if errors.Is(err, ErrUpgradeRequired) {
				m.fail(err)
				return false
			}
			// A transient failure: the tunnel is still ours, so try again
			// without touching the URL.
			m.log("could not reconnect", "error", err)
			return true
		}
		m.log("the tunnel is gone", "error", err)
	}

	// The tunnel expired: a new one means a new public URL.
	fresh, err := m.createTunnel(ctx)
	if err != nil {
		if errors.Is(err, ErrUpgradeRequired) {
			m.fail(err)
			return false
		}
		m.log("could not create a tunnel", "error", err)
		return true
	}
	client := m.newClient()
	if err := client.Connect(ctx, fresh); err != nil {
		m.log("could not connect to the new tunnel", "error", err)
		return true
	}
	m.adopt(client, fresh, true)
	m.reconnected(fresh.URL)
	return true
}

// adopt takes a live connection and the tunnel it belongs to.
func (m *Manager) adopt(client *Client, tunnel *Tunnel, changed bool) {
	m.mu.Lock()
	previous := ""
	if m.tunnel != nil {
		previous = m.tunnel.URL
	}
	m.client = client
	m.tunnel = tunnel
	m.mu.Unlock()

	if m.Store != nil {
		if err := m.Store.Save(tunnel); err != nil {
			m.log("could not save the tunnel", "error", err)
		}
	}
	if changed && previous != "" && previous != tunnel.URL {
		m.log("the public URL changed", "url", tunnel.URL)
		if m.OnURLChanged != nil {
			m.OnURLChanged(tunnel.URL)
		}
	}
}

// createTunnel asks the server for a new tunnel.
func (m *Manager) createTunnel(ctx context.Context) (*Tunnel, error) {
	client := &Client{ServerURL: m.ServerURL, Target: m.Target, HTTPClient: m.HTTPClient}
	tunnel, err := client.createTunnel(ctx)
	if err != nil {
		return nil, err
	}
	return tunnel, nil
}

// newClient builds a client for one connection attempt. Every attempt gets a
// fresh client: a connection is not reused after it ends.
func (m *Manager) newClient() *Client {
	return &Client{
		ServerURL:  m.ServerURL,
		Target:     m.Target,
		HTTPClient: m.HTTPClient,
	}
}

func (m *Manager) isClosed() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.closed
}

// sleep waits for the delay, reporting false when the manager was closed.
func (m *Manager) sleep(ctx context.Context, delay time.Duration) bool {
	if m.stopped == nil {
		return false
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-m.stopped:
		return false
	case <-timer.C:
		return true
	}
}

func (m *Manager) reconnected(url string) {
	m.log("reconnected", "url", url)
	if m.OnReconnect != nil {
		m.OnReconnect(url)
	}
}

func (m *Manager) fail(err error) {
	m.log("the tunnel failed", "error", err)
	if m.OnFailure != nil {
		m.OnFailure(err)
	}
}

func (m *Manager) log(msg string, args ...any) {
	if m.Logger != nil {
		m.Logger.Info(msg, args...)
	}
}
