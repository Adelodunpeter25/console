// The hostc client against the real tunnel service.
//
// This test reaches a third-party service and opens a public URL, so it is
// opt-in: set CONSOLE_HOSTC_E2E=1 to run it. Everything else about the
// client is covered by hostc_test.go against a stand-in server.
//
// The point of this test is exactly what the stand-in cannot prove: that
// the protocol implementation matches the real server, and not just a
// reading of the documentation that the stand-in was written from.
package tests

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/tunnel/hostc"
	"github.com/fasthttp/websocket"
)

// hostcServerURL is the public tunnel service.
const hostcServerURL = "https://hostc.dev"

// requireHostcE2E skips unless the operator asked for the real service.
func requireHostcE2E(t *testing.T) {
	t.Helper()
	if os.Getenv("CONSOLE_HOSTC_E2E") != "1" {
		t.Skip("set CONSOLE_HOSTC_E2E=1 to run against the real hostc service")
	}
}

// openRealTunnel opens a real tunnel to a local target.
func openRealTunnel(t *testing.T, target *url.URL) (*hostc.Client, *hostc.Tunnel) {
	t.Helper()
	client := &hostc.Client{
		ServerURL: hostcServerURL,
		Target:    target,
	}
	tunnel, err := client.Open(t.Context())
	if err != nil {
		t.Fatalf("open a real tunnel: %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })
	return client, tunnel
}

// fetchThrough fetches a public URL and returns the body.
func fetchThrough(t *testing.T, rawURL string) (int, string) {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, rawURL, nil)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	// The public request must not be answered by a proxy.
	req.Header.Set("Accept", "*/*")
	resp, err := (&http.Client{Timeout: 30 * time.Second}).Do(req)
	if err != nil {
		t.Fatalf("fetch %s: %v", rawURL, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read %s: %v", rawURL, err)
	}
	return resp.StatusCode, string(body)
}

// A real public request must reach the local server and come back with its
// own body — the whole point of the tunnel.
func TestHostcRealServiceForwardsHTTP(t *testing.T) {
	requireHostcE2E(t)

	const body = "hello from the local server"
	local := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = io.WriteString(w, body)
	}))
	t.Cleanup(local.Close)
	target, err := url.Parse(local.URL)
	if err != nil {
		t.Fatal(err)
	}

	_, tunnel := openRealTunnel(t, target)
	if !strings.HasPrefix(tunnel.URL, "https://") {
		t.Fatalf("the public URL should be https, got %q", tunnel.URL)
	}

	status, got := fetchThrough(t, tunnel.URL)
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}
	if got != body {
		t.Errorf("body = %q, want %q", got, body)
	}
}

// The path and query must survive the trip, and the local server must see
// its own address as Host rather than the public one.
func TestHostcRealServicePreservesThePathAndHost(t *testing.T) {
	requireHostcE2E(t)

	type seen struct {
		path string
		host string
	}
	got := make(chan seen, 4)
	local := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got <- seen{path: r.URL.RequestURI(), host: r.Host}
		_, _ = io.WriteString(w, "ok")
	}))
	t.Cleanup(local.Close)
	target, err := url.Parse(local.URL)
	if err != nil {
		t.Fatal(err)
	}

	_, tunnel := openRealTunnel(t, target)
	status, _ := fetchThrough(t, tunnel.URL+"/api/health?deep=1")
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}

	select {
	case s := <-got:
		if s.path != "/api/health?deep=1" {
			t.Errorf("path = %q", s.path)
		}
		if s.host != target.Host {
			t.Errorf("host = %q, want the local %q", s.host, target.Host)
		}
	case <-time.After(15 * time.Second):
		t.Fatal("the local server was never called")
	}
}

// A WebSocket must survive the tunnel: the console's terminals are
// WebSockets, so a tunnel that cannot carry one is not much use.
func TestHostcRealServiceForwardsWebSocket(t *testing.T) {
	requireHostcE2E(t)

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

	_, tunnel := openRealTunnel(t, target)
	public := strings.Replace(tunnel.URL, "https://", "wss://", 1)

	conn, resp, err := websocket.DefaultDialer.Dial(public, nil)
	if err != nil {
		detail := "no response"
		if resp != nil {
			body, _ := io.ReadAll(resp.Body)
			detail = fmt.Sprintf("status %s, body %q", resp.Status, strings.TrimSpace(string(body)))
		}
		t.Fatalf("dial the public WebSocket: %v (%s)", err, detail)
	}
	defer conn.Close()
	_ = conn.SetReadDeadline(time.Now().Add(30 * time.Second))

	if err := conn.WriteMessage(websocket.TextMessage, []byte("hello")); err != nil {
		t.Fatalf("write: %v", err)
	}
	typ, data, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if typ != websocket.TextMessage {
		t.Fatalf("expected a text message, got type %d", typ)
	}
	if string(data) != "echo:hello" {
		t.Errorf("payload = %q, want %q", data, "echo:hello")
	}
}

// A restart must not cost the public URL: the tunnel is resumed, so a
// client paired with the old URL stays paired.
func TestHostcRealServiceKeepsTheURLAcrossARestart(t *testing.T) {
	requireHostcE2E(t)

	local := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "still here")
	}))
	t.Cleanup(local.Close)
	target, err := url.Parse(local.URL)
	if err != nil {
		t.Fatal(err)
	}
	store := hostc.NewFileStore(filepath.Join(t.TempDir(), "tunnel.json"))

	first := &hostc.Manager{ServerURL: hostcServerURL, Target: target, Store: store}
	if err := first.Start(t.Context()); err != nil {
		t.Fatalf("start: %v", err)
	}
	public := first.PublicURL()
	if public == "" {
		t.Fatal("the manager should have a public URL")
	}
	// Stopping drops the connection, which leaves the tunnel resumable.
	_ = first.Stop()

	second := &hostc.Manager{ServerURL: hostcServerURL, Target: target, Store: store}
	t.Cleanup(func() { _ = second.Stop() })
	if err := second.Start(t.Context()); err != nil {
		t.Fatalf("resume after a restart: %v", err)
	}
	if got := second.PublicURL(); got != public {
		t.Errorf("the public URL changed across a restart: %q, want %q", got, public)
	}
	// And the resumed tunnel still carries traffic.
	status, body := fetchThrough(t, public)
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}
	if body != "still here" {
		t.Errorf("body = %q", body)
	}
}

// A streamed response must arrive as it is produced, not at the end: the
// console's agent runs stream their events over server-sent events.
func TestHostcRealServiceStreamsTheResponse(t *testing.T) {
	requireHostcE2E(t)

	local := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		flusher, _ := w.(http.Flusher)
		for i := 0; i < 3; i++ {
			fmt.Fprintf(w, "data: chunk-%d\n\n", i)
			if flusher != nil {
				flusher.Flush()
			}
			time.Sleep(150 * time.Millisecond)
		}
	}))
	t.Cleanup(local.Close)
	target, err := url.Parse(local.URL)
	if err != nil {
		t.Fatal(err)
	}

	_, tunnel := openRealTunnel(t, target)
	resp, err := (&http.Client{Timeout: 30 * time.Second}).Get(tunnel.URL + "/events")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer resp.Body.Close()

	buf := make([]byte, 4096)
	n, err := resp.Body.Read(buf)
	if err != nil && n == 0 {
		t.Fatalf("read: %v", err)
	}
	if !strings.Contains(string(buf[:n]), "chunk-0") {
		t.Errorf("the first chunk did not arrive promptly: %q", buf[:n])
	}
}
