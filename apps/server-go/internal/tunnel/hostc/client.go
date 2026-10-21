package hostc

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/fasthttp/websocket"
)

// Tunnel is a public URL and the credentials that hold it open.
type Tunnel struct {
	ID         string `json:"id"`
	URL        string `json:"url"`
	ConnectURL string `json:"connectUrl"`
	Token      string `json:"token"`
}

// RequestLog describes one request the tunnel carried.
type RequestLog struct {
	Method    string
	Path      string
	Status    int
	Duration  time.Duration
	WebSocket bool
}

// stream is one multiplexed request or WebSocket over the tunnel.
type stream interface {
	// start begins the work for the stream.
	start()
	// receive handles a frame addressed to this stream.
	receive(f Frame)
	// abort tears down local resources without telling the server.
	abort()
}

// Client forwards a local target through a hostc tunnel.
//
// One Client owns one tunnel: one WebSocket to the tunnel server, with every
// public request and WebSocket multiplexed over it.
type Client struct {
	// ServerURL is the tunnel server, e.g. https://hostc.dev.
	ServerURL string
	// Target is the local origin traffic is forwarded to, e.g.
	// http://127.0.0.1:3000.
	Target *url.URL
	// HTTPClient performs the local requests. A client with no timeout is
	// used when this is nil, because a streamed response may legitimately
	// stay open for a long time.
	HTTPClient *http.Client
	// Dialer opens the local WebSocket connections. A default dialer is
	// used when this is nil.
	Dialer *websocket.Dialer
	// Logger receives one line per request. Optional.
	Logger *slog.Logger
	// OnRequest, when set, is called once per request with its outcome.
	OnRequest func(RequestLog)

	conn         *websocket.Conn
	publicOrigin string

	writeMu  sync.Mutex
	pongMu   sync.Mutex
	pongSeen bool

	mu      sync.Mutex
	streams map[uint32]stream

	closeOnce sync.Once
	done      chan struct{}

	closeMu  sync.Mutex
	closeErr error
}

// Open creates a tunnel and connects to it.
//
// The returned URL is public: anyone holding it can reach the target. The
// caller must Close the client to release the tunnel.
func (c *Client) Open(ctx context.Context) (*Tunnel, error) {
	tunnel, err := c.createTunnel(ctx)
	if err != nil {
		return nil, err
	}
	if err := c.Connect(ctx, tunnel); err != nil {
		return nil, err
	}
	return tunnel, nil
}

// Connect opens the connection to a tunnel that already exists.
//
// The tunnel is resumed by its id and token, which is what keeps the public
// URL the same across a restart. A tunnel the server no longer knows is
// reported as ErrTunnelGone, and one the server refuses to serve because it
// needs a newer client as ErrUpgradeRequired.
func (c *Client) Connect(ctx context.Context, tunnel *Tunnel) error {
	if c.Target == nil {
		return fmt.Errorf("hostc: no target configured")
	}
	if tunnel == nil || tunnel.ConnectURL == "" || tunnel.Token == "" {
		return fmt.Errorf("hostc: the tunnel is incomplete")
	}
	header := http.Header{}
	header.Set("Authorization", "Bearer "+tunnel.Token)
	header.Set(ProtocolHeader, strconv.Itoa(ProtocolVersion))
	conn, resp, err := c.dialer().DialContext(ctx, tunnel.ConnectURL, header)
	if err != nil {
		return connectError(resp, err)
	}
	// A forwarded WebSocket message can be up to MaxWebSocketMessageBytes,
	// and a frame adds a five byte header on top of that.
	conn.SetReadLimit(MaxWebSocketMessageBytes + frameHeaderBytes + 1024)

	public, err := url.Parse(tunnel.URL)
	if err != nil {
		_ = conn.Close()
		return fmt.Errorf("hostc: the tunnel returned an unusable URL %q: %w", tunnel.URL, err)
	}

	c.conn = conn
	c.publicOrigin = originOf(public)
	c.streams = map[uint32]stream{}
	c.done = make(chan struct{})

	go c.readPump()
	go c.heartbeat()
	return nil
}

// Done is closed when the connection ends, for whatever reason.
func (c *Client) Done() <-chan struct{} {
	return c.done
}

// CloseCode is the WebSocket close code the connection ended with, or 0 when
// it ended without one.
func (c *Client) CloseCode() int {
	c.closeMu.Lock()
	defer c.closeMu.Unlock()
	var closeErr *websocket.CloseError
	if errors.As(c.closeErr, &closeErr) {
		return closeErr.Code
	}
	return 0
}

// connectError maps a failed connect handshake to the error the caller acts
// on: a tunnel the server no longer knows, a server that needs a newer
// client, or a transient failure worth retrying.
func connectError(resp *http.Response, err error) error {
	if resp != nil {
		switch resp.StatusCode {
		case http.StatusNotFound, http.StatusUnauthorized:
			return fmt.Errorf("%w: %s", ErrTunnelGone, resp.Status)
		case http.StatusUpgradeRequired:
			return fmt.Errorf("%w: %s", ErrUpgradeRequired, resp.Status)
		}
		return fmt.Errorf("hostc: could not connect to the tunnel: %s: %w", resp.Status, err)
	}
	return fmt.Errorf("hostc: could not connect to the tunnel: %w", err)
}

// Close releases the tunnel. The public URL stops working as soon as the
// server sees the close.
func (c *Client) Close() error {
	var err error
	c.closeOnce.Do(func() {
		if c.done != nil {
			close(c.done)
		}
		c.writeMu.Lock()
		if c.conn != nil {
			_ = c.conn.WriteMessage(websocket.CloseMessage,
				websocket.FormatCloseMessage(CloseShutdown, ""))
			err = c.conn.Close()
		}
		c.writeMu.Unlock()
		c.abortStreams()
	})
	return err
}

// Drop ends the connection without telling the server the tunnel is done.
//
// The server keeps a dropped tunnel for a grace period and lets the same
// client reconnect to it, so the public URL survives. Close releases it
// instead, which costs the URL.
func (c *Client) Drop() error {
	var err error
	c.closeOnce.Do(func() {
		if c.done != nil {
			close(c.done)
		}
		c.writeMu.Lock()
		if c.conn != nil {
			// No close frame: the server sees an abnormal closure and
			// keeps the tunnel for a reconnect.
			err = c.conn.Close()
		}
		c.writeMu.Unlock()
		c.abortStreams()
	})
	return err
}

// createTunnel asks the tunnel server for a public URL.
func (c *Client) createTunnel(ctx context.Context) (*Tunnel, error) {
	server := strings.TrimRight(c.ServerURL, "/")
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, server+"/api/tunnels", nil)
	if err != nil {
		return nil, fmt.Errorf("hostc: bad tunnel server URL %q: %w", c.ServerURL, err)
	}
	req.Header.Set(ProtocolHeader, strconv.Itoa(ProtocolVersion))
	resp, err := c.apiClient().Do(req)
	if err != nil {
		return nil, fmt.Errorf("hostc: could not reach the tunnel server at %s: %w", server, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("hostc: could not read the tunnel server's reply: %w", err)
	}
	if resp.StatusCode != http.StatusCreated {
		return nil, fmt.Errorf("hostc: the tunnel server refused to create a tunnel: %s", tunnelErrorText(resp))
	}
	var tunnel Tunnel
	if err := json.Unmarshal(body, &tunnel); err != nil {
		return nil, fmt.Errorf("hostc: the tunnel server returned an invalid tunnel: %w", err)
	}
	if tunnel.ID == "" || tunnel.URL == "" || tunnel.ConnectURL == "" || tunnel.Token == "" {
		return nil, fmt.Errorf("hostc: the tunnel server returned an incomplete tunnel")
	}
	return &tunnel, nil
}

// readPump reads frames until the connection ends.
func (c *Client) readPump() {
	defer c.shutdown()
	for {
		msgType, data, err := c.conn.ReadMessage()
		if err != nil {
			c.setCloseError(err)
			return
		}
		if msgType == websocket.TextMessage {
			if string(data) == Pong {
				c.pongMu.Lock()
				c.pongSeen = true
				c.pongMu.Unlock()
			}
			continue
		}
		frame, err := DecodeFrame(data)
		if err != nil {
			c.closeWith(CloseProtocolError, err.Error())
			return
		}
		c.dispatch(frame)
	}
}

// dispatch routes a frame to its stream.
func (c *Client) dispatch(f Frame) {
	if f.Type == FrameOpen {
		c.startStream(f)
		return
	}
	s := c.stream(f.Stream)
	if s == nil {
		// Frames for streams that already ended are expected and ignored.
		return
	}
	s.receive(f)
}

// startStream opens a new stream for an OPEN frame.
func (c *Client) startStream(f Frame) {
	open, err := decodeOpen(f.Payload)
	if err != nil {
		c.closeWith(CloseProtocolError, err.Error())
		return
	}
	var s stream
	// The websocket key's presence marks an upgrade, not its length: a
	// public client that offers no subprotocol sends an empty list.
	if open.WebSocket != nil {
		s = newWebSocketStream(c, f.Stream, open)
	} else {
		s = newHTTPStream(c, f.Stream, open)
	}
	c.mu.Lock()
	if c.streams == nil {
		c.mu.Unlock()
		return
	}
	if _, exists := c.streams[f.Stream]; exists {
		c.mu.Unlock()
		c.closeWith(CloseProtocolError, fmt.Sprintf("stream %d is already open", f.Stream))
		return
	}
	c.streams[f.Stream] = s
	c.mu.Unlock()
	s.start()
}

// removeStream forgets a finished stream.
func (c *Client) removeStream(id uint32) {
	c.mu.Lock()
	delete(c.streams, id)
	c.mu.Unlock()
}

func (c *Client) stream(id uint32) stream {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.streams[id]
}

func (c *Client) abortStreams() {
	c.mu.Lock()
	streams := make([]stream, 0, len(c.streams))
	for _, s := range c.streams {
		streams = append(streams, s)
	}
	c.streams = map[uint32]stream{}
	c.mu.Unlock()
	for _, s := range streams {
		s.abort()
	}
}

// send writes one frame to the tunnel server.
func (c *Client) send(t byte, stream uint32, payload []byte) {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	if c.conn == nil {
		return
	}
	_ = c.conn.WriteMessage(websocket.BinaryMessage, Frame{Type: t, Stream: stream, Payload: payload}.Encode())
}

// sendJSON writes a JSON payload as one frame.
func (c *Client) sendJSON(t byte, stream uint32, value any) {
	payload, err := json.Marshal(value)
	if err != nil {
		return
	}
	c.send(t, stream, payload)
}

// heartbeat pings the server and gives up on a connection that has silently
// died — a sleeping laptop or a changed network never emits a close event,
// so the missing pong is what reveals it.
func (c *Client) heartbeat() {
	ticker := time.NewTicker(HeartbeatInterval)
	defer ticker.Stop()
	for {
		select {
		case <-c.done:
			return
		case <-ticker.C:
		}
		c.pongMu.Lock()
		c.pongSeen = false
		c.pongMu.Unlock()

		c.writeMu.Lock()
		err := c.conn.WriteMessage(websocket.TextMessage, []byte(Ping))
		c.writeMu.Unlock()
		if err != nil {
			return
		}
		time.AfterFunc(HeartbeatTimeout, func() {
			c.pongMu.Lock()
			seen := c.pongSeen
			c.pongMu.Unlock()
			if !seen {
				c.shutdown()
			}
		})
	}
}

// setCloseError records why the connection ended, so a supervisor can tell a
// dropped connection from a tunnel another client took over.
func (c *Client) setCloseError(err error) {
	c.closeMu.Lock()
	if c.closeErr == nil {
		c.closeErr = err
	}
	c.closeMu.Unlock()
}

// closeWith ends the connection with a close code, for a protocol violation.
func (c *Client) closeWith(code int, reason string) {
	c.writeMu.Lock()
	if c.conn != nil {
		_ = c.conn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(code, reason))
	}
	c.writeMu.Unlock()
	c.shutdown()
}

// shutdown tears the connection down without telling the server.
func (c *Client) shutdown() {
	c.closeOnce.Do(func() {
		if c.done != nil {
			close(c.done)
		}
		c.writeMu.Lock()
		if c.conn != nil {
			_ = c.conn.Close()
		}
		c.writeMu.Unlock()
		c.abortStreams()
	})
}

// log records one request's outcome.
func (c *Client) log(entry RequestLog) {
	if c.OnRequest != nil {
		c.OnRequest(entry)
	}
	if c.Logger != nil {
		c.Logger.Info("tunnel request",
			"method", entry.Method, "path", entry.Path,
			"status", entry.Status, "duration", entry.Duration, "websocket", entry.WebSocket)
	}
}

// apiClient talks to the tunnel server. It follows redirects, so a server
// that lives behind an http -> https redirect still works.
func (c *Client) apiClient() *http.Client {
	if c.HTTPClient != nil {
		return c.HTTPClient
	}
	return &http.Client{}
}

// localClient talks to the local target.
//
// It does not follow redirects: the local server's Location header is
// rewritten to the public URL and handed to the public client, which is the
// one that should follow it.
func (c *Client) localClient() *http.Client {
	base := c.apiClient()
	clone := *base
	clone.CheckRedirect = func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}
	return &clone
}

func (c *Client) dialer() *websocket.Dialer {
	if c.Dialer != nil {
		return c.Dialer
	}
	return &websocket.Dialer{
		HandshakeTimeout: 15 * time.Second,
		// Local dev servers commonly use self-signed certificates.
		TLSClientConfig: insecureTLSConfig(),
	}
}

// tunnelErrorText renders a failed tunnel API response.
func tunnelErrorText(resp *http.Response) string {
	if resp == nil {
		return "no response"
	}
	return resp.Status
}

// insecureTLSConfig accepts self-signed certificates, which local dev
// servers commonly use.
func insecureTLSConfig() *tls.Config {
	return &tls.Config{InsecureSkipVerify: true}
}
