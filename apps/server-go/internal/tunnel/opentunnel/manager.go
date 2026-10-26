// Package opentunnel gives a local server a permanent public HTTPS address
// through the hosted OpenTunnel service (https://github.com/anomalyco/opentunnel).
//
// OpenTunnel is a blind TLS tunnel: the relay reads only the TLS ClientHello
// to route a visitor, and this client terminates TLS with a certificate whose
// private key never leaves the machine. The tunnel's id and token are stored,
// so every restart resumes the same hostname.
package opentunnel

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"log/slog"
	"math/rand"
	"net/http"
	"net/url"
	"sync"
	"time"
)

const (
	backoffBase      = 250 * time.Millisecond
	backoffMax       = 30 * time.Second
	stableConnection = 30 * time.Second

	certificatePoll     = 2 * time.Second
	certificateTimeout  = 10 * time.Minute
	certificateFresh    = 12 * time.Hour
	certificateDue      = time.Minute
	certificateRenewing = 30 * 24 * time.Hour
)

// Manager keeps one tunnel open for the life of the server.
type Manager struct {
	// API is the tunnel service's address. Empty means DefaultAPI.
	API string
	// Dir is where the tunnel's identity is stored.
	Dir string
	// Target is the local address traffic is forwarded to, as host:port.
	Target string
	// HTTPClient performs the API calls. Optional.
	HTTPClient *http.Client
	// PollInterval is how often a pending certificate is checked. Optional.
	PollInterval time.Duration
	// Logger receives the tunnel's events. Optional.
	Logger *slog.Logger

	// OnReady is called once the tunnel is provisioned and its certificate is
	// ready, with its public URL. It runs again if the tunnel had to be
	// replaced because the service no longer knew it.
	OnReady func(url string)
	// OnFailure is called when the manager gives up.
	OnFailure func(err error)

	mu       sync.Mutex
	identity *Identity
	api      *api
	apiURL   *url.URL
	certMu   sync.RWMutex
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
	return m.identity.URL()
}

func (m *Manager) certificate() *tls.Certificate {
	m.certMu.RLock()
	defer m.certMu.RUnlock()
	m.mu.Lock()
	id := m.identity
	m.mu.Unlock()
	if id == nil {
		return nil
	}
	return id.Certificate
}

// Start provisions the tunnel if this is the first run, then serves it in the
// background until Close. It returns as soon as the tunnel has a public
// hostname, so the caller can show the address straight away; the certificate
// and the connection finish in the background.
func (m *Manager) Start(ctx context.Context) error {
	if m.Target == "" {
		return errors.New("opentunnel: no target configured")
	}
	if m.Dir == "" {
		return errors.New("opentunnel: no storage directory configured")
	}
	client, err := newAPI(m.API, m.HTTPClient)
	if err != nil {
		return err
	}
	m.api, m.apiURL = client, client.base

	store := Store{Dir: m.Dir}
	identity, err := store.Load()
	if err != nil {
		// A damaged identity cannot be resumed. Starting over costs the URL,
		// but is better than a server with no remote access at all.
		m.log().Warn("the saved tunnel is unreadable, creating a new one", "error", err)
		_ = store.Clear()
		identity = nil
	}
	if identity == nil {
		if identity, err = m.provision(ctx, store); err != nil {
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

// provision creates a tunnel and its key and certificate request. The
// pending identity is saved before the certificate is requested, so a crash
// in between resumes the same tunnel instead of leaking another.
func (m *Manager) provision(ctx context.Context, store Store) (*Identity, error) {
	created, err := m.api.create(ctx)
	if err != nil {
		return nil, fmt.Errorf("create the tunnel: %w", err)
	}
	key, err := newKey()
	if err != nil {
		return nil, err
	}
	csr, err := certificateRequest(key, created.Tunnel.Hostname)
	if err != nil {
		return nil, fmt.Errorf("build the certificate request: %w", err)
	}
	if err := store.savePending(created.Tunnel.ID, created.Tunnel.Hostname, created.Token, key, csr); err != nil {
		return nil, err
	}
	m.log().Info("opentunnel: created a tunnel", "hostname", created.Tunnel.Hostname)
	return &Identity{
		ID: created.Tunnel.ID, Hostname: created.Tunnel.Hostname, Token: created.Token, Key: key, CSR: csr,
	}, nil
}

// ensureCertificate binds the request and waits until the certificate is
// ready. Binding the request a tunnel already has just returns its current
// certificate, so this is safe to run after a restart.
func (m *Manager) ensureCertificate(ctx context.Context, store Store, id *Identity) error {
	if id.Ready() {
		return nil
	}
	if id.CSR == "" {
		csr, err := certificateRequest(id.Key, id.Hostname)
		if err != nil {
			return err
		}
		id.CSR = csr
	}
	info, err := m.api.bindCertificate(ctx, id.ID, id.Token, id.CSR)
	if err != nil {
		return fmt.Errorf("request the certificate: %w", err)
	}
	deadline := time.Now().Add(certificateTimeout)
	for {
		switch info.State.Type {
		case "ready":
			return m.installCertificate(store, id, info.State)
		case "failed":
			return fmt.Errorf("the certificate could not be issued: %s", info.State.Reason)
		}
		if time.Now().After(deadline) {
			return errors.New("the certificate was not ready in time")
		}
		poll := m.PollInterval
		if poll <= 0 {
			poll = certificatePoll
		}
		if err := sleepCtx(ctx, poll); err != nil {
			return err
		}
		if info, err = m.api.certificate(ctx, id.ID, id.Token); err != nil {
			return fmt.Errorf("check the certificate: %w", err)
		}
	}
}

func (m *Manager) installCertificate(store Store, id *Identity, state certificateState) error {
	expiry, err := time.Parse(time.RFC3339, state.Expiry)
	if err != nil {
		expiry = time.Time{}
	}
	m.certMu.Lock()
	err = id.setCertificate(state.Certificate, state.Chain, nil)
	if err == nil && !expiry.IsZero() {
		id.Expiry = expiry
	}
	saved := id.Expiry
	m.certMu.Unlock()
	if err != nil {
		return err
	}
	return store.saveCertificate(id.ID, id.Hostname, state.Certificate, state.Chain, saved)
}

// supervise gets the certificate ready, then keeps the bridge connected.
func (m *Manager) supervise(ctx context.Context, store Store) {
	defer close(m.stopped)

	wasReady := false
	attempt := 0
	for ctx.Err() == nil {
		m.mu.Lock()
		id := m.identity
		m.mu.Unlock()

		if err := m.ensureCertificate(ctx, store, id); err != nil {
			if ctx.Err() != nil {
				return
			}
			if IsGone(err) {
				if !m.replace(ctx, store) {
					return
				}
				wasReady = false
				continue
			}
			m.log().Warn("opentunnel: the certificate is not ready", "error", err)
			if !m.wait(ctx, &attempt) {
				return
			}
			continue
		}
		if !wasReady {
			wasReady = true
			if m.OnReady != nil {
				m.OnReady(id.URL())
			}
		}

		certCtx, stopRefresh := context.WithCancel(ctx)
		go m.refreshCertificates(certCtx, store, id)

		connectedAt := time.Time{}
		sess := newSession(sessionConfig{
			api: m.apiURL, id: id.ID, token: id.Token, hostname: id.Hostname, target: m.Target,
			cert: m.certificate, log: m.log(),
			onAttached: func() {
				connectedAt = time.Now()
				m.log().Info("opentunnel: connected", "url", id.URL())
			},
		})
		end := sess.run(ctx)
		stopRefresh()

		switch end.kind {
		case endShutdown:
			return
		case endFatal:
			if end.code == codeAttachBadTok || end.code == codeGone {
				// The service no longer accepts our token: the tunnel is gone.
				if !m.replace(ctx, store) {
					return
				}
				wasReady = false
				continue
			}
			m.log().Warn("opentunnel: the bridge was refused", "code", end.code)
		default:
			m.log().Info("opentunnel: disconnected, reconnecting", "reason", end.err)
		}
		if !connectedAt.IsZero() && time.Since(connectedAt) >= stableConnection {
			attempt = 0
		}
		if !m.wait(ctx, &attempt) {
			return
		}
	}
}

// replace creates a new tunnel after the service has forgotten ours. This is
// the only thing that changes the public URL, and paired devices have to pair
// again afterwards.
func (m *Manager) replace(ctx context.Context, store Store) bool {
	m.log().Warn("opentunnel: the service no longer knows this tunnel, creating a new one")
	_ = store.Clear()
	for attempt := 0; ctx.Err() == nil; {
		id, err := m.provision(ctx, store)
		if err == nil {
			m.mu.Lock()
			m.identity = id
			m.mu.Unlock()
			return true
		}
		m.log().Warn("opentunnel: could not create a replacement tunnel", "error", err)
		if !m.wait(ctx, &attempt) {
			return false
		}
	}
	return false
}

// refreshCertificates picks up a certificate the service has renewed: rarely
// while the current one is fresh, every minute once renewal is due.
func (m *Manager) refreshCertificates(ctx context.Context, store Store, id *Identity) {
	for {
		wait := certificateFresh
		m.certMu.RLock()
		expiry := id.Expiry
		m.certMu.RUnlock()
		if !expiry.IsZero() && time.Until(expiry) < certificateRenewing {
			wait = certificateDue
		}
		if err := sleepCtx(ctx, wait); err != nil {
			return
		}
		info, err := m.api.certificate(ctx, id.ID, id.Token)
		if err != nil || info.State.Type != "ready" || info.State.Certificate == "" {
			continue
		}
		m.certMu.RLock()
		same := id.certPEM == info.State.Certificate
		m.certMu.RUnlock()
		if same {
			continue
		}
		if err := m.installCertificate(store, id, info.State); err != nil {
			m.log().Warn("opentunnel: could not use the renewed certificate", "error", err)
			continue
		}
		m.log().Info("opentunnel: certificate renewed")
	}
}

// wait sleeps for the next backoff step, returning false if the manager was
// closed meanwhile.
func (m *Manager) wait(ctx context.Context, attempt *int) bool {
	delay := backoffBase << min(*attempt, 7)
	if delay > backoffMax {
		delay = backoffMax
	}
	*attempt++
	delay = delay/2 + time.Duration(rand.Int63n(int64(delay/2)+1))
	return sleepCtx(ctx, delay) == nil
}
