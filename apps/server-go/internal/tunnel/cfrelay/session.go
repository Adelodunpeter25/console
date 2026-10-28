package cfrelay

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/fasthttp/websocket"
)

const (
	userAgent      = "console-cfrelay/1"
	connectTimeout = 15 * time.Second
	pingEvery      = 25 * time.Second
	// The relay answers each ping, so silence this long means a dead link.
	readTimeout = 80 * time.Second
	// wsBacklog bounds visitor websocket messages queued while the local
	// websocket is still being dialled.
	wsBacklog = 256
)

const (
	endShutdown = iota
	endRetry
	endFatal
)

type endInfo struct {
	kind int
	err  error
}

type sessionConfig struct {
	api        *url.URL
	name       string
	token      string
	target     string
	log        *slog.Logger
	onAttached func()
}

// stream is one proxied request or websocket.
type stream struct {
	// HTTP request body, written as DATA frames arrive.
	body   *io.PipeWriter
	cancel context.CancelFunc
	// websocket messages for the local socket.
	ws chan wsMessage
}

type wsMessage struct {
	kind    int
	payload []byte
	closing *closeBody
}

type session struct {
	cfg    sessionConfig
	conn   *websocket.Conn
	client *http.Client

	writeMu sync.Mutex
	mu      sync.Mutex
	streams map[uint32]*stream
	done    chan struct{}
	once    sync.Once
	end     endInfo
	wg      sync.WaitGroup
}

func newSession(cfg sessionConfig) *session {
	return &session{
		cfg: cfg,
		client: &http.Client{
			// Responses pass through exactly as the server wrote them.
			Transport:     &http.Transport{DisableCompression: true, Proxy: nil, MaxIdleConnsPerHost: 16},
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		},
		streams: make(map[uint32]*stream),
		done:    make(chan struct{}),
	}
}

func (s *session) finish(end endInfo) {
	s.once.Do(func() {
		s.end = end
		close(s.done)
		if s.conn != nil {
			_ = s.conn.Close()
		}
	})
}

func (s *session) send(kind byte, sid uint32, payload []byte) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	_ = s.conn.SetWriteDeadline(time.Now().Add(30 * time.Second))
	return s.conn.WriteMessage(websocket.BinaryMessage, encodeFrame(kind, sid, payload))
}

// run connects and serves until the connection ends.
func (s *session) run(ctx context.Context) endInfo {
	u := *s.cfg.api
	if u.Scheme == "https" {
		u.Scheme = "wss"
	} else {
		u.Scheme = "ws"
	}
	u.Path = strings.TrimRight(u.Path, "/") + "/connect/" + url.PathEscape(s.cfg.name)

	dialer := websocket.Dialer{Proxy: http.ProxyFromEnvironment, HandshakeTimeout: connectTimeout}
	dialCtx, cancel := context.WithTimeout(ctx, connectTimeout)
	conn, resp, err := dialer.DialContext(dialCtx, u.String(), http.Header{
		"Authorization": {"Bearer " + s.cfg.token},
		"User-Agent":    {userAgent},
	})
	cancel()
	if err != nil {
		if ctx.Err() != nil {
			return endInfo{kind: endShutdown}
		}
		if resp != nil && (resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusNotFound) {
			return endInfo{kind: endFatal, err: fmt.Errorf("the relay refused this tunnel (HTTP %d)", resp.StatusCode)}
		}
		return endInfo{kind: endRetry, err: fmt.Errorf("connect to the relay: %w", err)}
	}
	conn.SetReadLimit(4 << 20)
	s.conn = conn
	_ = conn.SetReadDeadline(time.Now().Add(readTimeout))

	stop := context.AfterFunc(ctx, func() { s.finish(endInfo{kind: endShutdown}) })
	defer stop()

	s.wg.Add(1)
	go s.pinger()
	if s.cfg.onAttached != nil {
		s.cfg.onAttached()
	}

	s.readLoop()
	s.finish(endInfo{kind: endRetry, err: errors.New("the relay connection closed")})
	s.abortAll()
	s.wg.Wait()
	return s.end
}

func (s *session) pinger() {
	defer s.wg.Done()
	t := time.NewTicker(pingEvery)
	defer t.Stop()
	for {
		select {
		case <-s.done:
			return
		case <-t.C:
			s.writeMu.Lock()
			_ = s.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			err := s.conn.WriteMessage(websocket.TextMessage, []byte("ping"))
			s.writeMu.Unlock()
			if err != nil {
				s.finish(endInfo{kind: endRetry, err: err})
				return
			}
		}
	}
}

func (s *session) readLoop() {
	for {
		kind, msg, err := s.conn.ReadMessage()
		if err != nil {
			return
		}
		_ = s.conn.SetReadDeadline(time.Now().Add(readTimeout))
		if kind != websocket.BinaryMessage {
			continue // the "pong" keepalive reply
		}
		ft, sid, payload, err := decodeFrame(msg)
		if err != nil {
			continue
		}
		s.handle(ft, sid, payload)
	}
}

func (s *session) handle(ft byte, sid uint32, payload []byte) {
	switch ft {
	case frameReq:
		s.startRequest(sid, payload)
	case frameData:
		if st := s.get(sid); st != nil && st.body != nil {
			// Writes block until the local server reads; that is the backpressure.
			_, _ = st.body.Write(append([]byte(nil), payload...))
		}
	case frameEnd:
		if st := s.get(sid); st != nil && st.body != nil {
			_ = st.body.Close()
		}
	case frameWSOpen:
		s.startWebSocket(sid, payload)
	case frameWSText, frameWSBin:
		if st := s.get(sid); st != nil && st.ws != nil {
			k := websocket.TextMessage
			if ft == frameWSBin {
				k = websocket.BinaryMessage
			}
			st.push(wsMessage{kind: k, payload: append([]byte(nil), payload...)})
		}
	case frameClose:
		if st := s.get(sid); st != nil && st.ws != nil {
			cb := closeBody{Code: websocket.CloseNormalClosure}
			_ = jsonUnmarshal(payload, &cb)
			st.push(wsMessage{closing: &cb})
		}
	case frameReset:
		if st := s.get(sid); st != nil {
			st.abort(errors.New(string(payload)))
			s.drop(sid)
		}
	}
}

func (st *stream) push(m wsMessage) {
	select {
	case st.ws <- m:
	default:
		st.cancel() // a visitor too far ahead of the local socket is dropped
	}
}

func (st *stream) abort(err error) {
	if st.body != nil {
		_ = st.body.CloseWithError(err)
	}
	if st.cancel != nil {
		st.cancel()
	}
}

func (s *session) get(sid uint32) *stream {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.streams[sid]
}

func (s *session) drop(sid uint32) {
	s.mu.Lock()
	delete(s.streams, sid)
	s.mu.Unlock()
}

func (s *session) abortAll() {
	s.mu.Lock()
	all := s.streams
	s.streams = make(map[uint32]*stream)
	s.mu.Unlock()
	for _, st := range all {
		st.abort(errors.New("relay connection lost"))
	}
}

func (s *session) startRequest(sid uint32, payload []byte) {
	var head requestHead
	if err := jsonUnmarshal(payload, &head); err != nil {
		_ = s.send(frameReset, sid, []byte("bad request head"))
		return
	}
	pr, pw := io.Pipe()
	ctx, cancel := context.WithCancel(context.Background())
	st := &stream{body: pw, cancel: cancel}
	s.mu.Lock()
	s.streams[sid] = st
	s.mu.Unlock()

	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		defer cancel()
		defer s.drop(sid)
		// Unblocks the read loop if the server answers before reading the body.
		defer pr.Close()
		s.serveRequest(ctx, sid, head, pr)
	}()
}

func (s *session) serveRequest(ctx context.Context, sid uint32, head requestHead, body io.Reader) {
	target := url.URL{Scheme: "http", Host: s.cfg.target}
	req, err := http.NewRequestWithContext(ctx, head.Method, target.String()+head.Path, body)
	if err != nil {
		_ = s.send(frameReset, sid, []byte("bad request"))
		return
	}
	for _, h := range head.Headers {
		switch strings.ToLower(h[0]) {
		case "host", "content-length":
			// Host stays the local target (the original is in X-Forwarded-Host);
			// the body is streamed, so its length is unknown here.
			if strings.EqualFold(h[0], "content-length") {
				if n, ok := parseLen(h[1]); ok {
					req.ContentLength = n
				}
			}
			continue
		}
		req.Header.Add(h[0], h[1])
	}
	if req.ContentLength == 0 && head.Method != http.MethodGet && head.Method != http.MethodHead {
		req.ContentLength = -1
	}
	if req.ContentLength == 0 {
		req.Body = http.NoBody
	}
	resp, err := s.client.Do(req)
	if err != nil {
		if ctx.Err() == nil {
			_ = s.send(frameReset, sid, []byte("the console server did not answer: "+shortErr(err)))
		}
		return
	}
	defer resp.Body.Close()

	rh := responseHead{Status: resp.StatusCode}
	for k, vs := range resp.Header {
		for _, v := range vs {
			rh.Headers = append(rh.Headers, [2]string{k, v})
		}
	}
	if err := s.send(frameRes, sid, jsonBytes(rh)); err != nil {
		return
	}
	buf := make([]byte, chunkSize)
	for {
		n, rerr := resp.Body.Read(buf)
		if n > 0 {
			if err := s.send(frameData, sid, buf[:n]); err != nil {
				return
			}
		}
		if rerr != nil {
			if rerr == io.EOF {
				_ = s.send(frameEnd, sid, nil)
			} else if ctx.Err() == nil {
				_ = s.send(frameReset, sid, []byte("response interrupted"))
			}
			return
		}
	}
}

func (s *session) startWebSocket(sid uint32, payload []byte) {
	var head requestHead
	if err := jsonUnmarshal(payload, &head); err != nil {
		_ = s.send(frameClose, sid, jsonBytes(closeBody{Code: websocket.CloseInternalServerErr, Reason: "bad request"}))
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	st := &stream{cancel: cancel, ws: make(chan wsMessage, wsBacklog)}
	s.mu.Lock()
	s.streams[sid] = st
	s.mu.Unlock()

	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		defer cancel()
		defer s.drop(sid)
		s.serveWebSocket(ctx, sid, head, st)
	}()
}

func (s *session) serveWebSocket(ctx context.Context, sid uint32, head requestHead, st *stream) {
	header := http.Header{}
	for _, h := range head.Headers {
		switch strings.ToLower(h[0]) {
		case "host", "origin", "sec-websocket-key", "sec-websocket-version", "sec-websocket-extensions":
			continue
		}
		header.Add(h[0], h[1])
	}
	// The local server sees the request as coming from itself, as a browser
	// on the same machine would.
	header.Set("Origin", "http://"+s.cfg.target)
	dialer := websocket.Dialer{HandshakeTimeout: connectTimeout}
	local, _, err := dialer.DialContext(ctx, "ws://"+s.cfg.target+head.Path, header)
	if err != nil {
		_ = s.send(frameClose, sid, jsonBytes(closeBody{Code: websocket.CloseInternalServerErr, Reason: "could not reach the console server"}))
		return
	}
	defer local.Close()

	stop := context.AfterFunc(ctx, func() { _ = local.Close() })
	defer stop()

	// Local -> visitor.
	go func() {
		for {
			kind, msg, err := local.ReadMessage()
			if err != nil {
				cb := closeBody{Code: websocket.CloseNormalClosure}
				var ce *websocket.CloseError
				if errors.As(err, &ce) {
					cb = closeBody{Code: ce.Code, Reason: ce.Text}
				}
				_ = s.send(frameClose, sid, jsonBytes(cb))
				st.cancel()
				return
			}
			ft := frameWSBin
			if kind == websocket.TextMessage {
				ft = frameWSText
			}
			if s.send(ft, sid, msg) != nil {
				return
			}
		}
	}()

	// Visitor -> local.
	for {
		select {
		case <-ctx.Done():
			return
		case m := <-st.ws:
			if m.closing != nil {
				_ = local.WriteControl(websocket.CloseMessage,
					websocket.FormatCloseMessage(safeCloseCode(m.closing.Code), m.closing.Reason), time.Now().Add(time.Second))
				return
			}
			if err := local.WriteMessage(m.kind, m.payload); err != nil {
				return
			}
		}
	}
}

func safeCloseCode(c int) int {
	switch {
	case c == 1000, c >= 3000 && c < 5000, c >= 1001 && c <= 1014 && c != 1004 && c != 1005 && c != 1006:
		return c
	}
	return websocket.CloseNormalClosure
}

func shortErr(err error) string {
	var ne net.Error
	if errors.As(err, &ne) && ne.Timeout() {
		return "timed out"
	}
	return "connection failed"
}
