// Manager owns the live MCP connections: one lazily-established session per
// enabled server, with status tracking, tool caching and reconnect-on-demand.
package mcp

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"sync"
	"time"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

const (
	StatusDisconnected = "disconnected"
	StatusConnecting   = "connecting"
	StatusNeedsAuth    = "needs_auth" // browser sign-in in progress
	StatusConnected    = "connected"
	StatusError        = "error"

	connectTimeout = 30 * time.Second
)

// ServerStatus is the API-facing snapshot of one server.
type ServerStatus struct {
	ID        string `json:"id"`
	Label     string `json:"label"`
	Enabled   bool   `json:"enabled"`
	Transport string `json:"transport"`
	Status    string `json:"status"`
	Error     string `json:"error,omitempty"`
	// AuthURL is set while browser sign-in is pending so the UI can offer
	// it if the automatic browser open failed.
	AuthURL   string `json:"authUrl,omitempty"`
	ToolCount int    `json:"toolCount"`
}

type serverState struct {
	status  string
	err     string
	authURL string
	client  *Client
	tools   []RemoteTool
	cancel  context.CancelFunc
	// ready is closed when the current connect attempt finishes.
	ready chan struct{}
}

type Manager struct {
	Config      *ConfigStore
	Credentials *CredentialStore
	OAuth       *OAuthCoordinator

	mu     sync.Mutex
	states map[string]*serverState
}

func NewManager(cfg *ConfigStore, creds *CredentialStore, oauth *OAuthCoordinator) *Manager {
	if oauth == nil {
		oauth = NewOAuthCoordinator()
	}
	return &Manager{Config: cfg, Credentials: creds, OAuth: oauth, states: map[string]*serverState{}}
}

// Connect starts connecting in the background and returns immediately.
// Progress is visible through Status; Ensure waits for the outcome.
func (m *Manager) Connect(id string) error {
	cfg, ok, err := m.Config.Get(id)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("mcp server %q not found", id)
	}
	if !cfg.Enabled {
		return fmt.Errorf("mcp server %q is disabled", id)
	}
	m.mu.Lock()
	if st := m.states[id]; st != nil && (st.status == StatusConnecting || st.status == StatusNeedsAuth || st.status == StatusConnected) {
		m.mu.Unlock()
		return nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	st := &serverState{status: StatusConnecting, cancel: cancel, ready: make(chan struct{})}
	m.states[id] = st
	m.mu.Unlock()
	go m.run(ctx, cfg, st)
	return nil
}

func (m *Manager) update(st *serverState, fn func(*serverState)) {
	m.mu.Lock()
	fn(st)
	m.mu.Unlock()
}

func (m *Manager) fail(st *serverState, err error) {
	slog.Warn("mcp connect failed", "error", err)
	m.update(st, func(s *serverState) { s.status, s.err, s.authURL = StatusError, err.Error(), "" })
	close(st.ready)
}

func (m *Manager) run(ctx context.Context, cfg ServerConfig, st *serverState) {
	timeout := connectTimeout
	usesOAuth := cfg.Transport == TransportHTTP && cfg.Auth != nil && cfg.Auth.Type == AuthOAuth2
	if usesOAuth {
		timeout = authTimeout + connectTimeout
	}
	cctx, ccancel := context.WithTimeout(ctx, timeout)
	defer ccancel()

	var transport sdk.Transport
	var err error
	if usesOAuth {
		h, herr := m.OAuth.NewHandler(cfg, m.Credentials, func(u string) {
			m.update(st, func(s *serverState) { s.status, s.authURL = StatusNeedsAuth, u })
		})
		if herr != nil {
			m.fail(st, herr)
			return
		}
		transport, err = buildTransport(cfg, m.Credentials, h)
	} else {
		transport, err = buildTransport(cfg, m.Credentials, nil)
	}
	if err != nil {
		m.fail(st, err)
		return
	}

	sdkClient := newSDKClient(func() { go m.refreshTools(cfg.ID, st) })
	session, err := sdkClient.Connect(cctx, transport, nil)
	if err != nil {
		m.fail(st, fmt.Errorf("connect: %w", err))
		return
	}
	client := &Client{session: session}
	tools, err := client.ListTools(cctx)
	if err != nil {
		_ = client.Close()
		m.fail(st, fmt.Errorf("list tools: %w", err))
		return
	}
	m.update(st, func(s *serverState) {
		s.status, s.err, s.authURL, s.client, s.tools = StatusConnected, "", "", client, tools
	})
	close(st.ready)

	// Session ended (server exited / network dropped): mark disconnected so
	// the next Ensure reconnects.
	go func() {
		_ = client.Wait()
		m.update(st, func(s *serverState) {
			if s.client == client {
				s.status, s.client = StatusDisconnected, nil
			}
		})
	}()
}

func (m *Manager) refreshTools(id string, st *serverState) {
	m.mu.Lock()
	client := st.client
	m.mu.Unlock()
	if client == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), connectTimeout)
	defer cancel()
	tools, err := client.ListTools(ctx)
	if err != nil {
		slog.Warn("mcp refresh tools failed", "server", id, "error", err)
		return
	}
	m.update(st, func(s *serverState) { s.tools = tools })
}

// Ensure connects if needed and blocks until the server is usable, the
// attempt fails, or ctx ends.
func (m *Manager) Ensure(ctx context.Context, id string) error {
	for attempt := 0; attempt < 2; attempt++ {
		if err := m.Connect(id); err != nil {
			return err
		}
		m.mu.Lock()
		st := m.states[id]
		ready := st.ready
		m.mu.Unlock()
		select {
		case <-ready:
		case <-ctx.Done():
			return ctx.Err()
		}
		m.mu.Lock()
		status, msg := st.status, st.err
		m.mu.Unlock()
		switch status {
		case StatusConnected:
			return nil
		case StatusError:
			return fmt.Errorf("%s", msg)
		}
		// Disconnected between ready and check: loop reconnects once.
	}
	return fmt.Errorf("mcp server %q is not connected", id)
}

// Tools returns the cached tool list of a connected server.
func (m *Manager) Tools(id string) []RemoteTool {
	m.mu.Lock()
	defer m.mu.Unlock()
	if st := m.states[id]; st != nil {
		return append([]RemoteTool(nil), st.tools...)
	}
	return nil
}

// Call runs a tool, reconnecting first when the session dropped.
func (m *Manager) Call(ctx context.Context, id, tool string, args map[string]any) (*sdk.CallToolResult, error) {
	if err := m.Ensure(ctx, id); err != nil {
		return nil, err
	}
	m.mu.Lock()
	client := m.states[id].client
	m.mu.Unlock()
	if client == nil {
		return nil, fmt.Errorf("mcp server %q is not connected", id)
	}
	return client.Call(ctx, tool, args)
}

// Disconnect closes the session and cancels any pending sign-in.
func (m *Manager) Disconnect(id string) {
	m.mu.Lock()
	st := m.states[id]
	delete(m.states, id)
	m.mu.Unlock()
	if st == nil {
		return
	}
	if st.cancel != nil {
		st.cancel()
	}
	if st.client != nil {
		_ = st.client.Close()
	}
}

// ResetAuth drops the stored credential so the next connect signs in afresh.
func (m *Manager) ResetAuth(id string) error {
	cfg, ok, err := m.Config.Get(id)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("mcp server %q not found", id)
	}
	m.Disconnect(id)
	if cfg.Auth != nil && cfg.Auth.TokenRef != "" {
		_, err = m.Credentials.Delete(cfg.Auth.TokenRef)
	}
	return err
}

// Status lists every configured server with its live state.
func (m *Manager) Status() ([]ServerStatus, error) {
	cfgs, err := m.Config.List()
	if err != nil {
		return nil, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]ServerStatus, 0, len(cfgs))
	for _, c := range cfgs {
		s := ServerStatus{ID: c.ID, Label: c.Label, Enabled: c.Enabled, Transport: c.Transport, Status: StatusDisconnected}
		if st := m.states[c.ID]; st != nil {
			s.Status, s.Error, s.AuthURL, s.ToolCount = st.status, st.err, st.authURL, len(st.tools)
		}
		out = append(out, s)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

// CloseAll ends every session and the OAuth listener (server shutdown).
func (m *Manager) CloseAll() {
	m.mu.Lock()
	ids := make([]string, 0, len(m.states))
	for id := range m.states {
		ids = append(ids, id)
	}
	m.mu.Unlock()
	for _, id := range ids {
		m.Disconnect(id)
	}
	m.OAuth.Close()
}
