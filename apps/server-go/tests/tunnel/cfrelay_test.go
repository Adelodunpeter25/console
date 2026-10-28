// The relay client against a stand-in relay that speaks the apps/relay protocol.
package tests

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/fasthttp/websocket"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/tunnel"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/tunnel/cfrelay"
)

type fakeCFRelay struct {
	server *httptest.Server
	mu     sync.Mutex
	conn   *websocket.Conn
	frames chan []byte
	regs   int
	taken  map[string]bool
	attach chan struct{}
}

func newFakeCFRelay(t *testing.T) *fakeCFRelay {
	t.Helper()
	f := &fakeCFRelay{frames: make(chan []byte, 64), taken: map[string]bool{}, attach: make(chan struct{}, 8)}
	upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	mux := http.NewServeMux()
	mux.HandleFunc("/register", func(w http.ResponseWriter, r *http.Request) {
		var body struct{ Name string }
		_ = json.NewDecoder(r.Body).Decode(&body)
		f.mu.Lock()
		defer f.mu.Unlock()
		name := body.Name
		if name == "" {
			name = "quiet-otter-42"
		}
		if f.taken[name] {
			http.Error(w, `{"error":"name taken"}`, http.StatusConflict)
			return
		}
		f.taken[name] = true
		f.regs++
		_ = json.NewEncoder(w).Encode(map[string]string{"name": name, "token": "tok-" + name, "url": f.server.URL + "/" + name})
	})
	mux.HandleFunc("/connect/", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer tok-"+strings.TrimPrefix(r.URL.Path, "/connect/") {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		c, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		f.mu.Lock()
		f.conn = c
		f.mu.Unlock()
		f.attach <- struct{}{}
		for {
			kind, msg, err := c.ReadMessage()
			if err != nil {
				return
			}
			if kind == websocket.BinaryMessage {
				f.frames <- msg
			}
		}
	})
	f.server = httptest.NewServer(mux)
	t.Cleanup(f.server.Close)
	return f
}

func cfFrame(kind byte, sid uint32, payload []byte) []byte {
	out := make([]byte, 5+len(payload))
	out[0] = kind
	binary.BigEndian.PutUint32(out[1:], sid)
	copy(out[5:], payload)
	return out
}

func (f *fakeCFRelay) push(t *testing.T, kind byte, sid uint32, payload []byte) {
	t.Helper()
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.conn.WriteMessage(websocket.BinaryMessage, cfFrame(kind, sid, payload)); err != nil {
		t.Fatal(err)
	}
}

func (f *fakeCFRelay) next(t *testing.T) (byte, uint32, []byte) {
	t.Helper()
	select {
	case m := <-f.frames:
		return m[0], binary.BigEndian.Uint32(m[1:]), m[5:]
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for a frame from the client")
		return 0, 0, nil
	}
}

func (f *fakeCFRelay) waitAttach(t *testing.T) {
	t.Helper()
	select {
	case <-f.attach:
	case <-time.After(5 * time.Second):
		t.Fatal("client never connected")
	}
}

func startCF(t *testing.T, f *fakeCFRelay, dir, target, name string) (*cfrelay.Manager, string) {
	t.Helper()
	ready := make(chan string, 4)
	m := &cfrelay.Manager{API: f.server.URL, Dir: dir, Target: target, Name: name, OnReady: func(u string) { ready <- u }}
	if err := m.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	select {
	case u := <-ready:
		return m, u
	case <-time.After(5 * time.Second):
		t.Fatal("never ready")
		return nil, ""
	}
}

func localServer(t *testing.T) string {
	t.Helper()
	up := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if websocket.IsWebSocketUpgrade(r) {
			c, err := up.Upgrade(w, r, nil)
			if err != nil {
				return
			}
			defer c.Close()
			for {
				k, m, err := c.ReadMessage()
				if err != nil {
					return
				}
				_ = c.WriteMessage(k, m)
			}
		}
		b, _ := io.ReadAll(r.Body)
		w.Header().Set("X-Seen-Path", r.URL.RequestURI())
		w.Header().Set("X-Seen-Host", r.Host)
		w.Write([]byte(r.Method + ":" + string(b)))
	}))
	t.Cleanup(srv.Close)
	return srv.Listener.Addr().(*net.TCPAddr).String()
}

func TestCFRelayRegistersAndKeepsItsURL(t *testing.T) {
	f := newFakeCFRelay(t)
	dir := t.TempDir()
	target := localServer(t)

	m, url1 := startCF(t, f, dir, target, "peter-dev")
	f.waitAttach(t)
	if want := f.server.URL + "/peter-dev"; url1 != want {
		t.Fatalf("url = %q, want %q", url1, want)
	}
	if info, ok := cfrelay.ReadInfo(dir); !ok || info.URL != url1 {
		t.Fatalf("ReadInfo = %+v, %v", info, ok)
	}
	m.Close()

	// A restart resumes the same name without registering again.
	m2, url2 := startCF(t, f, dir, target, "")
	defer m2.Close()
	f.waitAttach(t)
	if url2 != url1 || f.regs != 1 {
		t.Fatalf("restart: url=%q (want %q), registrations=%d (want 1)", url2, url1, f.regs)
	}
}

func TestCFRelayNameTaken(t *testing.T) {
	f := newFakeCFRelay(t)
	target := localServer(t)
	m, _ := startCF(t, f, t.TempDir(), target, "peter-dev")
	defer m.Close()

	m2 := &cfrelay.Manager{API: f.server.URL, Dir: t.TempDir(), Target: target, Name: "peter-dev"}
	if err := m2.Start(context.Background()); err == nil {
		m2.Close()
		t.Fatal("expected the duplicate name to be refused")
	}
}

func TestCFRelayForwardsHTTP(t *testing.T) {
	f := newFakeCFRelay(t)
	m, _ := startCF(t, f, t.TempDir(), localServer(t), "")
	defer m.Close()
	f.waitAttach(t)

	head, _ := json.Marshal(map[string]any{"method": "POST", "path": "/api/x?y=1", "headers": [][2]string{{"x-forwarded-host", "relay.test"}}})
	f.push(t, 1, 7, head)
	f.push(t, 2, 7, []byte("hel"))
	f.push(t, 2, 7, []byte("lo"))
	f.push(t, 3, 7, nil)

	kind, sid, payload := f.next(t)
	if kind != 4 || sid != 7 {
		t.Fatalf("first frame = %d/%d, want response head for 7", kind, sid)
	}
	var res struct {
		Status  int
		Headers [][2]string
	}
	_ = json.Unmarshal(payload, &res)
	seen := map[string]string{}
	for _, h := range res.Headers {
		seen[h[0]] = h[1]
	}
	if res.Status != 200 || seen["X-Seen-Path"] != "/api/x?y=1" || strings.Contains(seen["X-Seen-Host"], "relay.test") {
		t.Fatalf("response = %+v seen=%v", res, seen)
	}
	var body []byte
	for {
		kind, _, p := f.next(t)
		if kind == 3 {
			break
		}
		body = append(body, p...)
	}
	if string(body) != "POST:hello" {
		t.Fatalf("body = %q", body)
	}
}

func TestCFRelayForwardsWebSocket(t *testing.T) {
	f := newFakeCFRelay(t)
	m, _ := startCF(t, f, t.TempDir(), localServer(t), "")
	defer m.Close()
	f.waitAttach(t)

	head, _ := json.Marshal(map[string]any{"path": "/ws", "headers": [][2]string{}})
	f.push(t, 5, 9, head)
	f.push(t, 6, 9, []byte("ping-me"))
	kind, sid, payload := f.next(t)
	if kind != 6 || sid != 9 || string(payload) != "ping-me" {
		t.Fatalf("echo = %d/%d/%q", kind, sid, payload)
	}
	f.push(t, 7, 9, []byte{1, 2, 3})
	kind, _, payload = f.next(t)
	if kind != 7 || len(payload) != 3 {
		t.Fatalf("binary echo = %d/%v", kind, payload)
	}
}

func TestRelaySelection(t *testing.T) {
	t.Setenv("CONSOLE_RELAY", "")
	if tunnel.RelayEnabled() {
		t.Fatal("OpenTunnel must stay the default")
	}
	t.Setenv("CONSOLE_RELAY", "1")
	if !tunnel.RelayEnabled() {
		t.Fatal("CONSOLE_RELAY=1 must select the relay")
	}
	dir := t.TempDir()
	if err := (cfrelay.Store{Dir: dir}).Save(cfrelay.Identity{Name: "a", URL: "https://r/a", API: "https://r", Token: "t"}); err != nil {
		t.Fatal(err)
	}
	if info, ok := tunnel.ReadInfo(dir); !ok || info.URL != "https://r/a" || !info.Ready {
		t.Fatalf("ReadInfo = %+v, %v", info, ok)
	}
}
