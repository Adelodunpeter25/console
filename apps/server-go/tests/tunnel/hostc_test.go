// The hostc client speaks the tunnel protocol against a stand-in tunnel
// server: requests, request bodies, WebSockets and header rewriting are all
// driven end to end without needing the real service.
package tests

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/tunnel/hostc"
	"github.com/fasthttp/websocket"
)

// fakeTunnel is a stand-in for a hostc tunnel server. It hands out a tunnel
// and speaks the protocol over the connect WebSocket.
type fakeTunnel struct {
	server    *httptest.Server
	connected chan struct{}
	finished  chan struct{}
	once      sync.Once
}

func newFakeTunnel(t *testing.T, onConnect func(conn *websocket.Conn)) *fakeTunnel {
	t.Helper()
	f := &fakeTunnel{connected: make(chan struct{}), finished: make(chan struct{})}
	mux := http.NewServeMux()
	mux.HandleFunc("/api/tunnels", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if got := r.Header.Get(hostc.ProtocolHeader); got != "5" {
			http.Error(w, "unsupported protocol version", http.StatusUpgradeRequired)
			return
		}
		w.WriteHeader(http.StatusCreated)
		fmt.Fprintf(w,
			`{"id":"tunnel123","url":"https://tunnel123.example","connectUrl":"ws://%s/api/tunnels/tunnel123/connect","token":"secret"}`,
			r.Host)
	})
	mux.HandleFunc("/api/tunnels/tunnel123/connect", func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer secret" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		f.once.Do(func() { close(f.connected) })
		defer close(f.finished)
		if onConnect != nil {
			onConnect(conn)
		}
	})
	f.server = httptest.NewServer(mux)
	t.Cleanup(f.server.Close)
	return f
}

// wait blocks until the tunnel server's side of the conversation is over.
func (f *fakeTunnel) wait(t *testing.T) {
	t.Helper()
	select {
	case <-f.finished:
	case <-time.After(10 * time.Second):
		t.Error("the tunnel server never finished")
	}
}

// readFrame reads one frame from the tunnel connection.
func readFrame(conn *websocket.Conn) (hostc.Frame, error) {
	_ = conn.SetReadDeadline(time.Now().Add(10 * time.Second))
	typ, data, err := conn.ReadMessage()
	if err != nil {
		return hostc.Frame{}, err
	}
	if typ != websocket.BinaryMessage {
		return hostc.Frame{}, fmt.Errorf("expected a binary frame, got message type %d", typ)
	}
	return hostc.DecodeFrame(data)
}

// writeFrame sends one frame down the tunnel connection.
func writeFrame(conn *websocket.Conn, typ byte, stream uint32, payload []byte) error {
	frame := hostc.Frame{Type: typ, Stream: stream, Payload: payload}
	return conn.WriteMessage(websocket.BinaryMessage, frame.Encode())
}

// writeJSON sends a JSON payload as one frame.
func writeJSON(conn *websocket.Conn, typ byte, stream uint32, value any) error {
	payload, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return writeFrame(conn, typ, stream, payload)
}

// readHead reads a HEAD frame, skipping the WINDOW grants that may be
// interleaved with it.
func readHead(conn *websocket.Conn) (hostc.HeadMessage, error) {
	for {
		frame, err := readFrame(conn)
		if err != nil {
			return hostc.HeadMessage{}, err
		}
		if frame.Type == hostc.FrameWindow {
			continue
		}
		if frame.Type != hostc.FrameHead {
			return hostc.HeadMessage{}, fmt.Errorf("expected a HEAD frame, got type %d", frame.Type)
		}
		var head hostc.HeadMessage
		if err := json.Unmarshal(frame.Payload, &head); err != nil {
			return hostc.HeadMessage{}, err
		}
		return head, nil
	}
}

// readData reads a DATA frame, skipping WINDOW grants.
func readData(conn *websocket.Conn) (hostc.Frame, error) {
	for {
		frame, err := readFrame(conn)
		if err != nil {
			return hostc.Frame{}, err
		}
		if frame.Type == hostc.FrameWindow {
			continue
		}
		return frame, nil
	}
}

// openClient connects a client to a fake tunnel pointed at target.
func openClient(t *testing.T, tunnel *fakeTunnel, target *url.URL) *hostc.Client {
	t.Helper()
	client := &hostc.Client{
		ServerURL: tunnel.server.URL,
		Target:    target,
	}
	tunnelInfo, err := client.Open(t.Context())
	if err != nil {
		t.Fatalf("open tunnel: %v", err)
	}
	if tunnelInfo.URL != "https://tunnel123.example" {
		t.Errorf("tunnel URL = %q", tunnelInfo.URL)
	}
	t.Cleanup(func() { _ = client.Close() })
	return client
}

func TestHostcTunnelForwardsHTTPGet(t *testing.T) {
	type seen struct {
		method string
		path   string
		host   string
	}
	got := make(chan seen, 1)
	local := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got <- seen{method: r.Method, path: r.URL.RequestURI(), host: r.Host}
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte("hello from the local server"))
	}))
	t.Cleanup(local.Close)
	target, err := url.Parse(local.URL)
	if err != nil {
		t.Fatal(err)
	}

	tunnel := newFakeTunnel(t, func(conn *websocket.Conn) {
		if err := writeJSON(conn, hostc.FrameOpen, 1, hostc.OpenMessage{
			Method:  "GET",
			Path:    "/api/health?deep=1",
			Headers: hostc.HeaderList{{"host", "tunnel123.example"}, {"accept", "text/plain"}},
		}); err != nil {
			t.Errorf("write OPEN: %v", err)
			return
		}
		head, err := readHead(conn)
		if err != nil {
			t.Errorf("read HEAD: %v", err)
			return
		}
		if head.Status != 200 {
			t.Errorf("status = %d, want 200", head.Status)
		}
		if !head.Body {
			t.Error("expected the response to carry a body")
		}
		data, err := readData(conn)
		if err != nil {
			t.Errorf("read DATA: %v", err)
			return
		}
		if data.Type != hostc.FrameData {
			t.Errorf("expected DATA, got type %d", data.Type)
		}
		if string(data.Payload) != "hello from the local server" {
			t.Errorf("body = %q", data.Payload)
		}
		end, err := readData(conn)
		if err != nil {
			t.Errorf("read END: %v", err)
			return
		}
		if end.Type != hostc.FrameEnd {
			t.Errorf("expected END, got type %d", end.Type)
		}
	})
	openClient(t, tunnel, target)
	tunnel.wait(t)

	select {
	case s := <-got:
		if s.method != "GET" {
			t.Errorf("method = %q", s.method)
		}
		if s.path != "/api/health?deep=1" {
			t.Errorf("path = %q", s.path)
		}
		// The local server must see its own address as Host, not the
		// public one, or dev servers that validate it refuse the request.
		if s.host != target.Host {
			t.Errorf("host = %q, want the local %q", s.host, target.Host)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the local server was never called")
	}
}

func TestHostcTunnelForwardsRequestBody(t *testing.T) {
	body := make(chan string, 1)
	local := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		buf := make([]byte, 1024)
		n, _ := r.Body.Read(buf)
		body <- string(buf[:n])
		_, _ = w.Write([]byte("ok"))
	}))
	t.Cleanup(local.Close)
	target, err := url.Parse(local.URL)
	if err != nil {
		t.Fatal(err)
	}

	tunnel := newFakeTunnel(t, func(conn *websocket.Conn) {
		if err := writeJSON(conn, hostc.FrameOpen, 1, hostc.OpenMessage{
			Method:  "POST",
			Path:    "/submit",
			Headers: hostc.HeaderList{{"host", "tunnel123.example"}},
			Body:    true,
		}); err != nil {
			t.Errorf("write OPEN: %v", err)
			return
		}
		if err := writeFrame(conn, hostc.FrameData, 1, []byte("payload bytes")); err != nil {
			t.Errorf("write DATA: %v", err)
			return
		}
		if err := writeFrame(conn, hostc.FrameEnd, 1, nil); err != nil {
			t.Errorf("write END: %v", err)
			return
		}
		head, err := readHead(conn)
		if err != nil {
			t.Errorf("read HEAD: %v", err)
			return
		}
		if head.Status != 200 {
			t.Errorf("status = %d, want 200", head.Status)
		}
		if _, err := readData(conn); err != nil {
			t.Errorf("read DATA: %v", err)
		}
	})
	openClient(t, tunnel, target)
	tunnel.wait(t)

	select {
	case got := <-body:
		if got != "payload bytes" {
			t.Errorf("body = %q", got)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the local server never received the body")
	}
}

func TestHostcTunnelForwardsWebSocket(t *testing.T) {
	local := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		for {
			typ, data, err := conn.ReadMessage()
			if err != nil {
				return
			}
			if typ == websocket.TextMessage {
				_ = conn.WriteMessage(websocket.TextMessage, []byte("echo:"+string(data)))
			}
		}
	}))
	t.Cleanup(local.Close)
	target, err := url.Parse(local.URL)
	if err != nil {
		t.Fatal(err)
	}

	tunnel := newFakeTunnel(t, func(conn *websocket.Conn) {
		offered := []string{"chat"}
		if err := writeJSON(conn, hostc.FrameOpen, 1, hostc.OpenMessage{
			Method:    "GET",
			Path:      "/ws",
			Headers:   hostc.HeaderList{{"host", "tunnel123.example"}},
			WebSocket: &offered,
		}); err != nil {
			t.Errorf("write OPEN: %v", err)
			return
		}
		head, err := readHead(conn)
		if err != nil {
			t.Errorf("read HEAD: %v", err)
			return
		}
		if head.Status != 101 {
			t.Errorf("status = %d, want 101", head.Status)
			return
		}
		if err := writeFrame(conn, hostc.FrameText, 1, []byte("hello")); err != nil {
			t.Errorf("write TEXT: %v", err)
			return
		}
		text, err := readData(conn)
		if err != nil {
			t.Errorf("read TEXT: %v", err)
			return
		}
		if text.Type != hostc.FrameText {
			t.Errorf("expected TEXT, got type %d", text.Type)
		}
		if string(text.Payload) != "echo:hello" {
			t.Errorf("payload = %q", text.Payload)
		}
	})
	openClient(t, tunnel, target)
	tunnel.wait(t)
}

func TestHostcTunnelForwardsWebSocketWithNoSubprotocols(t *testing.T) {
	local := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		for {
			typ, data, err := conn.ReadMessage()
			if err != nil {
				return
			}
			if typ == websocket.TextMessage {
				_ = conn.WriteMessage(websocket.TextMessage, []byte("echo:"+string(data)))
			}
		}
	}))
	t.Cleanup(local.Close)
	target, err := url.Parse(local.URL)
	if err != nil {
		t.Fatal(err)
	}

	tunnel := newFakeTunnel(t, func(conn *websocket.Conn) {
		// A public client that offers no subprotocol sends an empty list.
		// The key's presence still marks the stream as a WebSocket upgrade,
		// so it must not be handled as a plain HTTP request.
		offered := []string{}
		if err := writeJSON(conn, hostc.FrameOpen, 1, hostc.OpenMessage{
			Method:    "GET",
			Path:      "/ws",
			Headers:   hostc.HeaderList{{"host", "tunnel123.example"}},
			WebSocket: &offered,
		}); err != nil {
			t.Errorf("write OPEN: %v", err)
			return
		}
		head, err := readHead(conn)
		if err != nil {
			t.Errorf("read HEAD: %v", err)
			return
		}
		if head.Status != 101 {
			t.Errorf("status = %d, want 101", head.Status)
			return
		}
		if err := writeFrame(conn, hostc.FrameText, 1, []byte("hello")); err != nil {
			t.Errorf("write TEXT: %v", err)
			return
		}
		text, err := readData(conn)
		if err != nil {
			t.Errorf("read TEXT: %v", err)
			return
		}
		if string(text.Payload) != "echo:hello" {
			t.Errorf("payload = %q", text.Payload)
		}
	})
	openClient(t, tunnel, target)
	tunnel.wait(t)
}

func TestHostcTunnelRewritesOriginToTheLocalOrigin(t *testing.T) {
	origin := make(chan string, 1)
	local := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin <- r.Header.Get("Origin")
		_, _ = w.Write([]byte("ok"))
	}))
	t.Cleanup(local.Close)
	target, err := url.Parse(local.URL)
	if err != nil {
		t.Fatal(err)
	}

	tunnel := newFakeTunnel(t, func(conn *websocket.Conn) {
		if err := writeJSON(conn, hostc.FrameOpen, 1, hostc.OpenMessage{
			Method: "GET",
			Path:   "/page",
			Headers: hostc.HeaderList{
				{"host", "tunnel123.example"},
				{"origin", "https://tunnel123.example"},
			},
		}); err != nil {
			t.Errorf("write OPEN: %v", err)
			return
		}
		if _, err := readHead(conn); err != nil {
			t.Errorf("read HEAD: %v", err)
		}
	})
	openClient(t, tunnel, target)
	tunnel.wait(t)

	select {
	case got := <-origin:
		if got != target.Scheme+"://"+target.Host {
			t.Errorf("origin = %q, want the local origin %q", got, target.Scheme+"://"+target.Host)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the local server was never called")
	}
}

func TestHostcTunnelRewritesLocationToThePublicURL(t *testing.T) {
	local := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Location", "http://"+r.Host+"/elsewhere")
		w.WriteHeader(http.StatusFound)
	}))
	t.Cleanup(local.Close)
	target, err := url.Parse(local.URL)
	if err != nil {
		t.Fatal(err)
	}

	tunnel := newFakeTunnel(t, func(conn *websocket.Conn) {
		if err := writeJSON(conn, hostc.FrameOpen, 1, hostc.OpenMessage{
			Method:  "GET",
			Path:    "/redirect",
			Headers: hostc.HeaderList{{"host", "tunnel123.example"}},
		}); err != nil {
			t.Errorf("write OPEN: %v", err)
			return
		}
		head, err := readHead(conn)
		if err != nil {
			t.Errorf("read HEAD: %v", err)
			return
		}
		var location string
		for _, h := range head.Headers {
			if strings.EqualFold(h[0], "location") {
				location = h[1]
			}
		}
		if location != "https://tunnel123.example/elsewhere" {
			t.Errorf("location = %q, want the public URL", location)
		}
	})
	openClient(t, tunnel, target)
	tunnel.wait(t)
}

func TestHostcTunnelRefusesARequestWithoutTheProtocolHeader(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/tunnels", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "unsupported protocol version", http.StatusUpgradeRequired)
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	target, _ := url.Parse("http://127.0.0.1:1")
	client := &hostc.Client{ServerURL: server.URL, Target: target}
	_, err := client.Open(t.Context())
	if err == nil {
		t.Fatal("expected the tunnel server's refusal to fail the open")
	}
	if !strings.Contains(err.Error(), "426") {
		t.Errorf("error should mention the status, got: %v", err)
	}
}

func TestHostcTunnelWaitsForTheWindowBeforeSendingMore(t *testing.T) {
	// A body larger than one window: the client may have only 256 KiB in
	// flight until the tunnel server grants more.
	const bodySize = 400 * 1024
	local := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/octet-stream")
		_, _ = w.Write(make([]byte, bodySize))
	}))
	t.Cleanup(local.Close)
	target, err := url.Parse(local.URL)
	if err != nil {
		t.Fatal(err)
	}

	tunnel := newFakeTunnel(t, func(conn *websocket.Conn) {
		if err := writeJSON(conn, hostc.FrameOpen, 1, hostc.OpenMessage{
			Method:  "GET",
			Path:    "/big",
			Headers: hostc.HeaderList{{"host", "tunnel123.example"}},
		}); err != nil {
			t.Errorf("write OPEN: %v", err)
			return
		}
		if _, err := readHead(conn); err != nil {
			t.Errorf("read HEAD: %v", err)
			return
		}
		// Read until the window is exhausted, then check that nothing more
		// arrives before the tunnel server grants more room. The frames are
		// collected in the background: a read timeout would break the
		// connection for good, so the stall is observed by waiting instead.
		frames := make(chan hostc.Frame, 64)
		go func() {
			defer close(frames)
			for {
				frame, err := readFrame(conn)
				if err != nil {
					return
				}
				frames <- frame
			}
		}()

		received := 0
	wait:
		for {
			select {
			case frame, ok := <-frames:
				if !ok {
					t.Error("the tunnel closed early")
					return
				}
				if frame.Type == hostc.FrameData {
					received += len(frame.Payload)
				}
			case <-time.After(600 * time.Millisecond):
				break wait
			}
		}
		if received != hostc.StreamWindowBytes {
			t.Errorf("the client sent %d bytes without a grant, want %d", received, hostc.StreamWindowBytes)
		}
		if err := writeFrame(conn, hostc.FrameWindow, 1, hostc.EncodeWindow(hostc.StreamWindowBytes)); err != nil {
			t.Errorf("write WINDOW: %v", err)
			return
		}
		// With the window granted the rest of the body must arrive.
		rest := 0
		for rest < bodySize-hostc.StreamWindowBytes {
			select {
			case frame, ok := <-frames:
				if !ok {
					t.Error("the tunnel closed early")
					return
				}
				if frame.Type == hostc.FrameData {
					rest += len(frame.Payload)
				}
			case <-time.After(10 * time.Second):
				t.Errorf("only %d of %d bytes arrived after the grant", rest, bodySize-hostc.StreamWindowBytes)
				return
			}
		}
	})
	openClient(t, tunnel, target)
	tunnel.wait(t)
}

func TestHostcFrameCodec(t *testing.T) {
	frame := hostc.Frame{Type: hostc.FrameData, Stream: 42, Payload: []byte("body")}
	decoded, err := hostc.DecodeFrame(frame.Encode())
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if decoded.Type != hostc.FrameData || decoded.Stream != 42 || string(decoded.Payload) != "body" {
		t.Errorf("round trip gave %+v", decoded)
	}

	if _, err := hostc.DecodeFrame([]byte{1, 2}); err == nil {
		t.Error("a short frame should be rejected")
	}
	if _, err := hostc.DecodeFrame([]byte{99, 0, 0, 0, 1}); err == nil {
		t.Error("an unknown frame type should be rejected")
	}
	if _, err := hostc.DecodeFrame([]byte{1, 0, 0, 0, 0}); err == nil {
		t.Error("stream id 0 should be rejected")
	}
}
