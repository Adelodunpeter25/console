// The tunnel manager: a dropped connection is re-established on the same
// tunnel, a restart resumes the same public URL, and a tunnel that has
// expired is replaced.
package tests

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/tunnel/hostc"
	"github.com/fasthttp/websocket"
)

// managerTunnel is a stand-in tunnel server for the manager tests. It can
// drop connections and expire tunnels on demand.
type managerTunnel struct {
	server *httptest.Server

	mu sync.Mutex
	// created counts the tunnels handed out.
	created int
	// expired holds the tunnel ids the server has forgotten. A tunnel that
	// was not created yet is never expired, so a new one still works.
	expired map[string]bool
	// connections receives every accepted tunnel connection.
	connections chan *websocket.Conn
	// ids records the tunnel ids that were connected to.
	ids []string
}

func newManagerTunnel(t *testing.T) *managerTunnel {
	t.Helper()
	f := &managerTunnel{connections: make(chan *websocket.Conn, 16), expired: map[string]bool{}}
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
		f.mu.Lock()
		f.created++
		id := fmt.Sprintf("tunnel%02d", f.created)
		f.mu.Unlock()
		w.WriteHeader(http.StatusCreated)
		fmt.Fprintf(w,
			`{"id":%q,"url":"https://%s.hostc.app","connectUrl":"ws://%s/api/tunnels/%s/connect","token":"secret-%s"}`,
			id, id, r.Host, id, id)
	})
	mux.HandleFunc("/api/tunnels/", func(w http.ResponseWriter, r *http.Request) {
		id := r.URL.Path[len("/api/tunnels/"):]
		id = id[:len(id)-len("/connect")]
		f.mu.Lock()
		gone := f.expired[id]
		if !gone {
			f.ids = append(f.ids, id)
		}
		f.mu.Unlock()
		if gone {
			http.Error(w, "Tunnel not found", http.StatusNotFound)
			return
		}
		if got := r.Header.Get("Authorization"); got != "Bearer secret-"+id {
			http.Error(w, "Invalid tunnel token", http.StatusUnauthorized)
			return
		}
		upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		select {
		case f.connections <- conn:
		default:
		}
		// Hold the connection open until the client goes away.
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	})
	f.server = httptest.NewServer(mux)
	t.Cleanup(f.server.Close)
	return f
}

// dropAll closes every open tunnel connection, as a network drop would.
func (f *managerTunnel) dropAll() {
	for {
		select {
		case conn := <-f.connections:
			_ = conn.Close()
		default:
			return
		}
	}
}

// expire makes the server forget the tunnels it has handed out so far. A
// tunnel created afterwards is still served, as a real server would.
func (f *managerTunnel) expire() {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, id := range f.ids {
		f.expired[id] = true
	}
}

// createdCount is how many tunnels the server handed out.
func (f *managerTunnel) createdCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.created
}

// connectedIDs are the tunnel ids a client connected to, in order.
func (f *managerTunnel) connectedIDs() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.ids...)
}

// waitFor polls until the condition holds, or the test gives up.
func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

// A dropped connection must be re-established on the same tunnel, so the
// public URL does not change.
func TestManagerReconnectsToTheSameTunnel(t *testing.T) {
	server := newManagerTunnel(t)
	target, _ := url.Parse("http://127.0.0.1:1")
	manager := &hostc.Manager{ServerURL: server.server.URL, Target: target}
	t.Cleanup(func() { _ = manager.Stop() })

	if err := manager.Start(t.Context()); err != nil {
		t.Fatalf("start: %v", err)
	}
	first := manager.PublicURL()
	if first == "" {
		t.Fatal("the manager should have a public URL")
	}

	server.dropAll()
	waitFor(t, "the manager to reconnect", func() bool {
		return len(server.connectedIDs()) >= 2
	})

	// The same tunnel, so the URL is unchanged.
	ids := server.connectedIDs()
	if ids[0] != ids[1] {
		t.Errorf("reconnected to %q, want the same tunnel %q", ids[1], ids[0])
	}
	if got := manager.PublicURL(); got != first {
		t.Errorf("the public URL changed to %q, want %q", got, first)
	}
	if got := server.createdCount(); got != 1 {
		t.Errorf("created %d tunnels, want 1", got)
	}
}

// A tunnel the server has forgotten has to be replaced, and the URL changes
// with it.
func TestManagerReplacesAnExpiredTunnel(t *testing.T) {
	server := newManagerTunnel(t)
	target, _ := url.Parse("http://127.0.0.1:1")
	changed := make(chan string, 4)
	manager := &hostc.Manager{
		ServerURL:    server.server.URL,
		Target:       target,
		OnURLChanged: func(u string) { changed <- u },
	}
	t.Cleanup(func() { _ = manager.Stop() })

	if err := manager.Start(t.Context()); err != nil {
		t.Fatalf("start: %v", err)
	}
	first := manager.PublicURL()

	// The tunnel expires and the connection drops.
	server.expire()
	server.dropAll()

	select {
	case got := <-changed:
		if got == first {
			t.Errorf("the new URL is the same as the old one: %q", got)
		}
	case <-time.After(15 * time.Second):
		t.Fatal("the manager never reported a URL change")
	}
	if got := manager.PublicURL(); got == first {
		t.Errorf("the public URL did not change: %q", got)
	}
	if got := server.createdCount(); got != 2 {
		t.Errorf("created %d tunnels, want 2", got)
	}
}

// A restart must resume the same public URL, which is what a stored tunnel
// is for.
func TestManagerResumesAStoredTunnel(t *testing.T) {
	server := newManagerTunnel(t)
	target, _ := url.Parse("http://127.0.0.1:1")
	path := filepath.Join(t.TempDir(), "tunnel.json")
	store := hostc.NewFileStore(path)

	first := &hostc.Manager{ServerURL: server.server.URL, Target: target, Store: store}
	if err := first.Start(t.Context()); err != nil {
		t.Fatalf("start: %v", err)
	}
	url := first.PublicURL()
	_ = first.Stop()

	// The tunnel file holds the id and token needed to resume it.
	saved, err := store.Load()
	if err != nil {
		t.Fatalf("load the saved tunnel: %v", err)
	}
	if saved == nil {
		t.Fatal("the tunnel was not saved")
	}
	if saved.ID == "" || saved.Token == "" || saved.ConnectURL == "" {
		t.Errorf("the saved tunnel is incomplete: %+v", saved)
	}

	// A new manager resumes it rather than creating a new one.
	second := &hostc.Manager{ServerURL: server.server.URL, Target: target, Store: store}
	t.Cleanup(func() { _ = second.Stop() })
	if err := second.Start(t.Context()); err != nil {
		t.Fatalf("resume: %v", err)
	}
	if got := second.PublicURL(); got != url {
		t.Errorf("the resumed URL is %q, want %q", got, url)
	}
	if got := server.createdCount(); got != 1 {
		t.Errorf("created %d tunnels, want 1 — a restart must not mint a new one", got)
	}
}

// A tunnel file that is corrupt must not stop the manager: it starts a new
// tunnel instead.
func TestManagerStartsOverWhenTheStoredTunnelIsCorrupt(t *testing.T) {
	server := newManagerTunnel(t)
	target, _ := url.Parse("http://127.0.0.1:1")
	path := filepath.Join(t.TempDir(), "tunnel.json")
	if err := os.WriteFile(path, []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	manager := &hostc.Manager{
		ServerURL: server.server.URL,
		Target:    target,
		Store:     hostc.NewFileStore(path),
	}
	t.Cleanup(func() { _ = manager.Stop() })
	if err := manager.Start(t.Context()); err != nil {
		t.Fatalf("start: %v", err)
	}
	if manager.PublicURL() == "" {
		t.Error("the manager should have a public URL")
	}
}

// Another client taking the tunnel over must stop the manager rather than
// start a tug of war over it.
func TestManagerStopsWhenTheTunnelIsReplaced(t *testing.T) {
	server := newManagerTunnel(t)
	target, _ := url.Parse("http://127.0.0.1:1")
	failed := make(chan error, 1)
	manager := &hostc.Manager{
		ServerURL: server.server.URL,
		Target:    target,
		OnFailure: func(err error) { failed <- err },
	}
	t.Cleanup(func() { _ = manager.Stop() })

	if err := manager.Start(t.Context()); err != nil {
		t.Fatalf("start: %v", err)
	}

	// The server closes the connection with the "replaced" code.
	waitFor(t, "the connection to be accepted", func() bool {
		return len(server.connectedIDs()) >= 1
	})
	select {
	case conn := <-server.connections:
		_ = conn.WriteMessage(websocket.CloseMessage,
			websocket.FormatCloseMessage(hostc.CloseReplaced, "replaced by a new connection"))
	case <-time.After(5 * time.Second):
		t.Fatal("no connection to close")
	}

	select {
	case err := <-failed:
		if err == nil {
			t.Error("expected a failure to be reported")
		}
	case <-time.After(10 * time.Second):
		t.Fatal("the manager never reported the failure")
	}
	// And it must not reconnect: the tunnel belongs to someone else now.
	before := len(server.connectedIDs())
	time.Sleep(1500 * time.Millisecond)
	if after := len(server.connectedIDs()); after != before {
		t.Errorf("the manager reconnected %d times after being replaced", after-before)
	}
}

// The file store keeps the tunnel and clears it.
func TestFileStoreRoundTripsAndClears(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "tunnel.json")
	store := hostc.NewFileStore(path)

	loaded, err := store.Load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if loaded != nil {
		t.Error("a missing file should load as no tunnel")
	}

	tunnel := &hostc.Tunnel{ID: "abc", URL: "https://abc.hostc.app", ConnectURL: "wss://hostc.dev/x", Token: "secret"}
	if err := store.Save(tunnel); err != nil {
		t.Fatalf("save: %v", err)
	}
	loaded, err = store.Load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if loaded == nil || loaded.ID != tunnel.ID || loaded.Token != tunnel.Token {
		t.Fatalf("round trip gave %+v", loaded)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("the tunnel file holds a token and should be 0600, got %o", perm)
	}

	if err := store.Clear(); err != nil {
		t.Fatalf("clear: %v", err)
	}
	loaded, err = store.Load()
	if err != nil {
		t.Fatalf("load after clear: %v", err)
	}
	if loaded != nil {
		t.Error("the tunnel should be gone after Clear")
	}
}

// A tunnel file that holds something unusable is ignored rather than fatal.
func TestFileStoreIgnoresAnIncompleteTunnel(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tunnel.json")
	if err := os.WriteFile(path, []byte(`{"id":"abc"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	store := hostc.NewFileStore(path)
	loaded, err := store.Load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if loaded != nil {
		t.Errorf("an incomplete tunnel should load as none, got %+v", loaded)
	}
}

// The saved tunnel has to be readable JSON with the fields the client needs.
func TestFileStoreWritesTheTunnelFields(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tunnel.json")
	store := hostc.NewFileStore(path)
	tunnel := &hostc.Tunnel{ID: "abc", URL: "https://abc.hostc.app", ConnectURL: "wss://hostc.dev/x", Token: "secret"}
	if err := store.Save(tunnel); err != nil {
		t.Fatalf("save: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("the tunnel file is not JSON: %v", err)
	}
	for _, field := range []string{"id", "url", "connectUrl", "token"} {
		if _, ok := raw[field]; !ok {
			t.Errorf("the tunnel file is missing %q: %s", field, data)
		}
	}
}
