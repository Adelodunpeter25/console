package opentunnel

import (
	"context"
	"crypto/tls"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/fasthttp/websocket"
)

// Bridge protocol constants (docs/protocol.md, "Bridge").
const (
	subprotocol    = "opentunnel"
	clientVersion  = "console/1"
	maxConns       = 256
	maxPayload     = 32 * 1024
	connectTimeout = 10 * time.Second
	writeTimeout   = 30 * time.Second

	// A public connection that has buffered this much unread inbound data
	// is stalled: the local side is not keeping up.
	stalledBytes = 8 << 20

	// halfCloseGrace is how long the second direction of a connection may
	// stay open once the first has ended, like the relay's own rule.
	halfCloseGrace = 60 * time.Second

	serverRoute = "@"
)

// Reset codes the client sends.
const (
	codeUnknownRoute  = "unknown_route"
	codeTooManyConns  = "too_many_connections"
	codeUpstreamConn  = "upstream_connect_failed"
	codeUpstreamIO    = "upstream_io_error"
	codeAttachBadTok  = "bad_token"
	codeAttachNoCert  = "cert_not_ready"
	codeRouteConflict = "route_conflict"
	// codeGone is ours: the handshake itself was refused as unknown.
	codeGone = "gone"
)

// endKind says why a session ended.
type endKind int

const (
	endRetry endKind = iota // reconnect with backoff
	endShutdown
	endFatal // do not retry; see endInfo.code
)

type endInfo struct {
	kind     endKind
	code     string
	err      error
	attached bool
}

type controlMessage struct {
	Type        string          `json:"type"`
	Token       string          `json:"token,omitempty"`
	Transport   string          `json:"transport,omitempty"`
	Routes      []string        `json:"routes,omitempty"`
	Client      *clientInfo     `json:"client,omitempty"`
	Session     string          `json:"session,omitempty"`
	HeartbeatMs int             `json:"heartbeat_ms,omitempty"`
	IdleMs      int             `json:"idle_timeout_ms,omitempty"`
	Code        string          `json:"code,omitempty"`
	Reason      string          `json:"reason,omitempty"`
	Conn        uint32          `json:"conn"`
	Peer        string          `json:"peer,omitempty"`
	SNI         string          `json:"sni,omitempty"`
	TimeSent    json.RawMessage `json:"time_sent,omitempty"`
}

type clientInfo struct {
	Version  string `json:"version"`
	MaxConns int    `json:"max_conns"`
}

// sessionConfig is what a session needs from its manager.
type sessionConfig struct {
	api      *url.URL
	id       string
	token    string
	hostname string
	target   string
	cert     func() *tls.Certificate
	log      *slog.Logger

	// onAttached runs once the server accepts the bridge.
	onAttached func()
}

// session is one bridge WebSocket: it owns every public connection that
// arrives over it, terminates their TLS, and forwards them to the target.
type session struct {
	cfg  sessionConfig
	conn *websocket.Conn

	// ctrl carries control messages that may jump the queue (pong, ping,
	// reset); data carries connection data and each connection's `end`, in
	// order, so an `end` can never overtake the bytes it closes. The writer
	// drains ctrl first, and the read loop only ever does non-blocking sends
	// to ctrl, so a congested data path can never stop it answering a ping.
	ctrl chan []byte
	data chan outFrame

	done     chan struct{}
	doneOnce sync.Once
	result   endInfo

	mu       sync.Mutex
	channels map[uint32]*channel

	lastRecv atomic.Int64
	attached atomic.Bool
	wg       sync.WaitGroup
}

// outFrame is one ordered item on the data path.
type outFrame struct {
	kind    int // websocket.BinaryMessage or websocket.TextMessage
	payload []byte
}

func newSession(cfg sessionConfig) *session {
	return &session{
		cfg:      cfg,
		ctrl:     make(chan []byte, 1024),
		data:     make(chan outFrame, 64),
		done:     make(chan struct{}),
		channels: map[uint32]*channel{},
	}
}

// finish ends the session once, recording why.
func (s *session) finish(info endInfo) {
	s.doneOnce.Do(func() {
		info.attached = s.attached.Load()
		s.result = info
		close(s.done)
		if s.conn != nil {
			_ = s.conn.Close()
		}
	})
}

// run connects, attaches, and serves until the session ends.
func (s *session) run(ctx context.Context) endInfo {
	u := *s.cfg.api
	if u.Scheme == "https" {
		u.Scheme = "wss"
	} else {
		u.Scheme = "ws"
	}
	u.Path = strings.TrimRight(u.Path, "/") + "/api/tunnel/" + url.PathEscape(s.cfg.id) + "/connect"

	dialer := websocket.Dialer{
		Subprotocols:     []string{subprotocol},
		Proxy:            http.ProxyFromEnvironment,
		HandshakeTimeout: connectTimeout,
	}
	dialCtx, cancel := context.WithTimeout(ctx, connectTimeout)
	conn, resp, err := dialer.DialContext(dialCtx, u.String(), http.Header{"User-Agent": {userAgent}})
	cancel()
	if err != nil {
		if ctx.Err() != nil {
			return endInfo{kind: endShutdown}
		}
		// A tunnel the service has forgotten is refused at the handshake.
		if resp != nil && (resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusUnauthorized) {
			return endInfo{kind: endFatal, code: codeGone, err: fmt.Errorf("the service does not know this tunnel (HTTP %d)", resp.StatusCode)}
		}
		return endInfo{kind: endRetry, err: fmt.Errorf("connect the bridge: %w", err)}
	}
	conn.SetReadLimit(1 << 20)
	s.conn = conn
	s.lastRecv.Store(time.Now().UnixMilli())

	// Closing the context ends the session.
	stop := context.AfterFunc(ctx, func() { s.finish(endInfo{kind: endShutdown}) })
	defer stop()

	s.wg.Add(1)
	go s.writer()

	s.sendControl(controlMessage{
		Type: "attach", Token: s.cfg.token, Transport: "ws", Routes: []string{serverRoute},
		Client: &clientInfo{Version: clientVersion, MaxConns: maxConns},
	})

	// The server closes a bridge that does not attach in time; so do we.
	attachTimer := time.AfterFunc(connectTimeout, func() {
		if !s.attached.Load() {
			s.finish(endInfo{kind: endRetry, err: errors.New("the bridge did not attach in time")})
		}
	})
	defer attachTimer.Stop()

	s.readLoop()

	<-s.done
	s.closeAllChannels()
	s.wg.Wait()
	return s.result
}

func (s *session) readLoop() {
	for {
		msgType, payload, err := s.conn.ReadMessage()
		if err != nil {
			s.finish(endInfo{kind: endRetry, err: fmt.Errorf("the bridge closed: %w", err)})
			return
		}
		s.lastRecv.Store(time.Now().UnixMilli())
		switch msgType {
		case websocket.TextMessage:
			s.onControl(payload)
		case websocket.BinaryMessage:
			s.onData(payload)
		}
		select {
		case <-s.done:
			return
		default:
		}
	}
}

func (s *session) onControl(raw []byte) {
	var msg controlMessage
	if err := json.Unmarshal(raw, &msg); err != nil {
		return // unknown or malformed messages are ignored
	}
	if !s.attached.Load() {
		switch msg.Type {
		case "attached":
			s.attached.Store(true)
			s.startHeartbeat(time.Duration(msg.HeartbeatMs)*time.Millisecond, time.Duration(msg.IdleMs)*time.Millisecond)
			if s.cfg.onAttached != nil {
				s.cfg.onAttached()
			}
		case "attach_error":
			switch msg.Code {
			case codeAttachBadTok, codeAttachNoCert:
				s.finish(endInfo{kind: endFatal, code: msg.Code, err: fmt.Errorf("the bridge was refused: %s", msg.Code)})
			default:
				s.finish(endInfo{kind: endRetry, code: msg.Code, err: fmt.Errorf("the bridge was refused: %s", msg.Code)})
			}
		}
		return
	}
	switch msg.Type {
	case "open":
		s.open(msg)
	case "end":
		if ch := s.channel(msg.Conn); ch != nil {
			ch.endInbound()
		}
	case "reset":
		if ch := s.channel(msg.Conn); ch != nil {
			ch.abort(msg.Code, false)
		}
	case "ping":
		s.sendControl(controlMessage{Type: "pong", TimeSent: msg.TimeSent})
	case "drain":
		s.finish(endInfo{kind: endRetry, err: fmt.Errorf("the server is draining: %s", msg.Reason)})
	}
}

func (s *session) onData(frame []byte) {
	if len(frame) < 4 {
		return
	}
	if ch := s.channel(binary.BigEndian.Uint32(frame[:4])); ch != nil {
		ch.push(frame[4:])
	}
}

func (s *session) channel(id uint32) *channel {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.channels[id]
}

// open starts a public connection. Everything slow runs off the read loop.
func (s *session) open(msg controlMessage) {
	if !routeMatches(msg.SNI, s.cfg.hostname) {
		s.sendControl(controlMessage{Type: "reset", Conn: msg.Conn, Code: codeUnknownRoute})
		return
	}
	s.mu.Lock()
	if len(s.channels) >= maxConns {
		s.mu.Unlock()
		s.sendControl(controlMessage{Type: "reset", Conn: msg.Conn, Code: codeTooManyConns})
		return
	}
	ch := newChannel(s, msg.Conn)
	s.channels[msg.Conn] = ch
	s.mu.Unlock()

	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		s.serveChannel(ch)
	}()
}

// routeMatches reports whether sni names the tunnel hostname itself, the only
// route this client claims.
func routeMatches(sni, hostname string) bool {
	return strings.EqualFold(strings.TrimSuffix(sni, "."), hostname)
}

// serveChannel terminates TLS for one public connection and splices it to the
// local server.
func (s *session) serveChannel(ch *channel) {
	defer ch.finish()

	tlsConn := tls.Server(ch, &tls.Config{
		MinVersion: tls.VersionTLS12,
		// The target speaks HTTP/1.1, so never negotiate anything else.
		NextProtos: []string{"http/1.1"},
		GetCertificate: func(*tls.ClientHelloInfo) (*tls.Certificate, error) {
			if c := s.cfg.cert(); c != nil {
				return c, nil
			}
			return nil, errors.New("no certificate yet")
		},
	})
	handshake, cancel := context.WithTimeout(context.Background(), connectTimeout)
	err := tlsConn.HandshakeContext(handshake)
	cancel()
	if err != nil {
		ch.abort(codeUpstreamIO, true)
		return
	}
	defer tlsConn.Close()

	up, err := net.DialTimeout("tcp", s.cfg.target, connectTimeout)
	if err != nil {
		s.cfg.log.Debug("local target unreachable", "target", s.cfg.target, "error", err)
		ch.abort(codeUpstreamConn, true)
		return
	}
	defer up.Close()
	if tcp, ok := up.(*net.TCPConn); ok {
		_ = tcp.SetNoDelay(true)
	}

	finished := make(chan struct{}, 2)
	go func() { // visitor -> local server
		_, _ = io.Copy(up, tlsConn)
		if tcp, ok := up.(*net.TCPConn); ok {
			_ = tcp.CloseWrite()
		}
		finished <- struct{}{}
	}()
	go func() { // local server -> visitor
		_, _ = io.Copy(tlsConn, up)
		_ = tlsConn.CloseWrite()
		ch.closeWrite()
		finished <- struct{}{}
	}()

	<-finished
	// One direction ended. The other gets a grace period, then both go.
	grace := time.NewTimer(halfCloseGrace)
	defer grace.Stop()
	select {
	case <-finished:
	case <-grace.C:
		_ = up.Close()
		_ = tlsConn.Close()
		<-finished
	case <-s.done:
		_ = up.Close()
		_ = tlsConn.Close()
		<-finished
	}
}

// startHeartbeat pings on the server's interval and gives up on a bridge that
// has been silent for longer than its idle timeout.
func (s *session) startHeartbeat(heartbeat, idle time.Duration) {
	if heartbeat < time.Second {
		heartbeat = 15 * time.Second
	}
	if idle < 5*time.Second {
		idle = 45 * time.Second
	}
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		ticker := time.NewTicker(heartbeat)
		defer ticker.Stop()
		for {
			select {
			case <-s.done:
				return
			case <-ticker.C:
				if time.Since(time.UnixMilli(s.lastRecv.Load())) > idle {
					s.finish(endInfo{kind: endRetry, err: errors.New("the bridge went silent")})
					return
				}
				s.sendControl(controlMessage{Type: "ping", TimeSent: json.RawMessage(fmt.Sprint(time.Now().UnixMilli()))})
			}
		}
	}()
}

// sendControl queues a control message without ever blocking the caller.
func (s *session) sendControl(msg controlMessage) {
	raw, err := json.Marshal(msg)
	if err != nil {
		return
	}
	select {
	case s.ctrl <- raw:
	case <-s.done:
	default:
		// The writer is so far behind that even control traffic is stuck.
		s.finish(endInfo{kind: endRetry, err: errors.New("the bridge write queue is full")})
	}
}

// sendData queues one data frame for a connection, waiting while the bridge
// is congested. Only per-connection goroutines call it, never the read loop.
func (s *session) sendData(conn uint32, payload []byte) error {
	for off := 0; off < len(payload); off += maxPayload {
		end := off + maxPayload
		if end > len(payload) {
			end = len(payload)
		}
		frame := make([]byte, 4+end-off)
		binary.BigEndian.PutUint32(frame, conn)
		copy(frame[4:], payload[off:end])
		if err := s.enqueue(outFrame{kind: websocket.BinaryMessage, payload: frame}); err != nil {
			return err
		}
	}
	return nil
}

// enqueue puts one item on the ordered data path, waiting while it is full.
func (s *session) enqueue(f outFrame) error {
	select {
	case s.data <- f:
		return nil
	case <-s.done:
		return io.ErrClosedPipe
	}
}

// sendEnd half-closes a connection after every byte already queued for it.
func (s *session) sendEnd(conn uint32) {
	raw, err := json.Marshal(controlMessage{Type: "end", Conn: conn})
	if err != nil {
		return
	}
	_ = s.enqueue(outFrame{kind: websocket.TextMessage, payload: raw})
}

// writer is the only goroutine that writes to the socket. Control messages
// go first so a ping reply is never stuck behind a queue of data.
func (s *session) writer() {
	defer s.wg.Done()
	write := func(kind int, payload []byte) bool {
		_ = s.conn.SetWriteDeadline(time.Now().Add(writeTimeout))
		if err := s.conn.WriteMessage(kind, payload); err != nil {
			s.finish(endInfo{kind: endRetry, err: fmt.Errorf("write to the bridge: %w", err)})
			return false
		}
		return true
	}
	for {
		select {
		case <-s.done:
			return
		default:
		}
		select {
		case msg := <-s.ctrl:
			if !write(websocket.TextMessage, msg) {
				return
			}
			continue
		default:
		}
		select {
		case msg := <-s.ctrl:
			if !write(websocket.TextMessage, msg) {
				return
			}
		case frame := <-s.data:
			if !write(frame.kind, frame.payload) {
				return
			}
		case <-s.done:
			return
		}
	}
}

func (s *session) closeAllChannels() {
	s.mu.Lock()
	chans := make([]*channel, 0, len(s.channels))
	for _, ch := range s.channels {
		chans = append(chans, ch)
	}
	s.mu.Unlock()
	for _, ch := range chans {
		ch.abort("", false)
	}
}

func (s *session) remove(ch *channel) {
	s.mu.Lock()
	if s.channels[ch.id] == ch {
		delete(s.channels, ch.id)
	}
	s.mu.Unlock()
}

// channel is one public connection as seen over the bridge. It is a net.Conn
// so crypto/tls can run on it directly: reads come from the server's data
// frames and writes become data frames back.
type channel struct {
	s  *session
	id uint32

	mu      sync.Mutex
	cond    *sync.Cond
	queue   [][]byte
	queued  int
	eof     bool // the server half-closed: no more inbound data
	closed  bool // torn down: reads and writes fail
	sentEnd bool
}

func newChannel(s *session, id uint32) *channel {
	ch := &channel{s: s, id: id}
	ch.cond = sync.NewCond(&ch.mu)
	return ch
}

// push appends inbound data. It never blocks: a connection that falls too far
// behind is reset instead of stalling the whole bridge.
func (c *channel) push(payload []byte) {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return
	}
	if c.queued+len(payload) > stalledBytes {
		c.mu.Unlock()
		c.abort(codeUpstreamIO, true)
		return
	}
	c.queue = append(c.queue, payload)
	c.queued += len(payload)
	c.mu.Unlock()
	c.cond.Signal()
}

func (c *channel) endInbound() {
	c.mu.Lock()
	c.eof = true
	c.mu.Unlock()
	c.cond.Broadcast()
}

func (c *channel) Read(p []byte) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for len(c.queue) == 0 && !c.eof && !c.closed {
		c.cond.Wait()
	}
	if len(c.queue) > 0 {
		n := copy(p, c.queue[0])
		c.queue[0] = c.queue[0][n:]
		c.queued -= n
		if len(c.queue[0]) == 0 {
			c.queue = c.queue[1:]
		}
		return n, nil
	}
	if c.closed {
		return 0, io.ErrClosedPipe
	}
	return 0, io.EOF
}

func (c *channel) Write(p []byte) (int, error) {
	c.mu.Lock()
	closed := c.closed
	c.mu.Unlock()
	if closed {
		return 0, io.ErrClosedPipe
	}
	if err := c.s.sendData(c.id, p); err != nil {
		return 0, err
	}
	return len(p), nil
}

// closeWrite tells the server this side is done sending.
func (c *channel) closeWrite() {
	c.mu.Lock()
	send := !c.sentEnd && !c.closed
	c.sentEnd = true
	c.mu.Unlock()
	if send {
		c.s.sendEnd(c.id)
	}
}

// abort tears the connection down, telling the server when asked to.
func (c *channel) abort(code string, notify bool) {
	c.mu.Lock()
	wasClosed := c.closed
	c.closed = true
	c.queue, c.queued = nil, 0
	c.mu.Unlock()
	c.cond.Broadcast()
	if notify && !wasClosed && code != "" {
		c.s.sendControl(controlMessage{Type: "reset", Conn: c.id, Code: code})
	}
	c.s.remove(c)
}

// finish ends a connection that completed normally.
func (c *channel) finish() {
	c.closeWrite()
	c.abort("", false)
}

// Close satisfies net.Conn; crypto/tls calls it when the TLS session ends.
func (c *channel) Close() error {
	c.finish()
	return nil
}

type pipeAddr string

func (a pipeAddr) Network() string { return "opentunnel" }
func (a pipeAddr) String() string  { return string(a) }

func (c *channel) LocalAddr() net.Addr  { return pipeAddr("opentunnel-local") }
func (c *channel) RemoteAddr() net.Addr { return pipeAddr(fmt.Sprintf("opentunnel-conn-%d", c.id)) }

// Deadlines are not needed: the handshake is bounded by HandshakeContext and
// idle connections are reset by the relay.
func (c *channel) SetDeadline(time.Time) error      { return nil }
func (c *channel) SetReadDeadline(time.Time) error  { return nil }
func (c *channel) SetWriteDeadline(time.Time) error { return nil }
