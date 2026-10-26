// The OpenTunnel client against a stand-in tunnel service. The stand-in issues
// real certificates from a test CA and acts as a visitor over the bridge, so a
// request travels the whole path: TLS handshake through the bridge, TLS
// termination in the client, and forwarding to a local HTTP server.
package tests

import (
	"bufio"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/binary"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/fasthttp/websocket"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/tunnel/opentunnel"
)

// relayConn is a visitor's connection as the stand-in relay sees it: bytes the
// visitor writes become binary frames to the client, and frames back from the
// client become bytes to read.
type relayConn struct {
	relay *fakeRelay
	id    uint32

	mu    sync.Mutex
	cond  *sync.Cond
	buf   []byte
	eof   bool
	reset string
}

func (c *relayConn) Read(p []byte) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for len(c.buf) == 0 && !c.eof && c.reset == "" {
		c.cond.Wait()
	}
	if len(c.buf) > 0 {
		n := copy(p, c.buf)
		c.buf = c.buf[n:]
		return n, nil
	}
	if c.reset != "" {
		return 0, fmt.Errorf("reset: %s", c.reset)
	}
	return 0, io.EOF
}

func (c *relayConn) Write(p []byte) (int, error) {
	frame := make([]byte, 4+len(p))
	binary.BigEndian.PutUint32(frame, c.id)
	copy(frame[4:], p)
	if err := c.relay.write(websocket.BinaryMessage, frame); err != nil {
		return 0, err
	}
	return len(p), nil
}

func (c *relayConn) Close() error                     { return nil }
func (c *relayConn) LocalAddr() net.Addr              { return &net.TCPAddr{} }
func (c *relayConn) RemoteAddr() net.Addr             { return &net.TCPAddr{} }
func (c *relayConn) SetDeadline(time.Time) error      { return nil }
func (c *relayConn) SetReadDeadline(time.Time) error  { return nil }
func (c *relayConn) SetWriteDeadline(time.Time) error { return nil }

// fakeRelay is a stand-in for the hosted OpenTunnel service.
type fakeRelay struct {
	t      *testing.T
	server *httptest.Server

	caCert *x509.Certificate
	caKey  *ecdsa.PrivateKey
	caPEM  []byte

	mu          sync.Mutex
	created     int
	createCalls int
	failCreates int // answer this many creates with 503 first
	gone        bool
	issuingFor  int // certificate polls that answer "issuing" before ready
	polls       int
	hostname    string
	token       string
	csrPEM      string
	certPEM     string
	attaches    int
	controls    chan map[string]any

	wsMu  sync.Mutex
	ws    *websocket.Conn
	conns map[uint32]*relayConn
	next  uint32

	// heartbeat is the interval the relay advertises on attach.
	heartbeatMs int
}

func newFakeRelay(t *testing.T) *fakeRelay {
	t.Helper()
	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "test relay CA"},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(24 * time.Hour),
		IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &caKey.PublicKey, caKey)
	if err != nil {
		t.Fatal(err)
	}
	caCert, _ := x509.ParseCertificate(der)

	f := &fakeRelay{
		t: t, caCert: caCert, caKey: caKey, controls: make(chan map[string]any, 64),
		conns: map[uint32]*relayConn{}, heartbeatMs: 15000,
		caPEM: pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}),
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/api/tunnel", f.handleCreate)
	mux.HandleFunc("/api/tunnel/", f.handleTunnel)
	f.server = httptest.NewServer(mux)
	t.Cleanup(f.server.Close)
	return f
}

func (f *fakeRelay) handleCreate(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	f.createCalls++
	if f.failCreates > 0 {
		f.failCreates--
		f.mu.Unlock()
		http.Error(w, `{"_tag":"ServiceUnavailableError","message":"busy"}`, http.StatusServiceUnavailable)
		return
	}
	f.created++
	n := f.created
	f.hostname = fmt.Sprintf("tun%d.opentunnel.test", n)
	f.token = fmt.Sprintf("token-%d", n)
	f.gone = false
	f.polls = 0
	body := fmt.Sprintf(`{"tunnel":{"id":"tun%d","hostname":%q,"state":"offline"},"token":%q}`, n, f.hostname, f.token)
	f.mu.Unlock()
	w.WriteHeader(http.StatusCreated)
	_, _ = io.WriteString(w, body)
}

func (f *fakeRelay) authorized(w http.ResponseWriter, r *http.Request) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.gone {
		http.Error(w, `{"_tag":"TunnelNotFoundError","message":"tunnel not found"}`, http.StatusNotFound)
		return false
	}
	if r.Header.Get("Authorization") != "Bearer "+f.token {
		http.Error(w, `{"_tag":"UnauthorizedError","message":"bad token"}`, http.StatusUnauthorized)
		return false
	}
	return true
}

func (f *fakeRelay) handleTunnel(w http.ResponseWriter, r *http.Request) {
	switch {
	case strings.HasSuffix(r.URL.Path, "/connect"):
		f.handleConnect(w, r)
	case strings.HasSuffix(r.URL.Path, "/certificate") && r.Method == http.MethodPost:
		if !f.authorized(w, r) {
			return
		}
		var body struct{ CSR string }
		_ = json.NewDecoder(r.Body).Decode(&body)
		f.mu.Lock()
		f.csrPEM = body.CSR
		f.mu.Unlock()
		w.WriteHeader(http.StatusAccepted)
		_, _ = io.WriteString(w, `{"id":"cert-1","state":{"type":"issuing"}}`)
	case strings.HasSuffix(r.URL.Path, "/certificate"):
		if !f.authorized(w, r) {
			return
		}
		f.mu.Lock()
		f.polls++
		pending := f.polls <= f.issuingFor
		csr := f.csrPEM
		f.mu.Unlock()
		if pending {
			_, _ = io.WriteString(w, `{"id":"cert-1","state":{"type":"issuing"}}`)
			return
		}
		leaf, expiry := f.sign(csr)
		out, _ := json.Marshal(map[string]any{"id": "cert-1", "state": map[string]any{
			"type": "ready", "certificate": leaf, "chain": string(f.caPEM), "expiry": expiry,
		}})
		_, _ = w.Write(out)
	default:
		http.NotFound(w, r)
	}
}

// sign issues a certificate for exactly the names in the request.
func (f *fakeRelay) sign(csrPEM string) (string, string) {
	block, _ := pem.Decode([]byte(csrPEM))
	if block == nil {
		f.t.Errorf("relay: the certificate request is not PEM")
		return "", ""
	}
	csr, err := x509.ParseCertificateRequest(block.Bytes)
	if err != nil || csr.CheckSignature() != nil {
		f.t.Errorf("relay: bad certificate request: %v", err)
		return "", ""
	}
	expiry := time.Now().Add(90 * 24 * time.Hour)
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(time.Now().UnixNano()), Subject: csr.Subject, DNSNames: csr.DNSNames,
		NotBefore: time.Now().Add(-time.Minute), NotAfter: expiry,
		KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, f.caCert, csr.PublicKey, f.caKey)
	if err != nil {
		f.t.Fatal(err)
	}
	leaf := string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}))
	f.mu.Lock()
	f.certPEM = leaf
	f.mu.Unlock()
	return leaf, expiry.UTC().Format(time.RFC3339)
}

func (f *fakeRelay) handleConnect(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	gone := f.gone
	f.mu.Unlock()
	if gone {
		http.Error(w, `{"_tag":"TunnelNotFoundError","message":"tunnel not found"}`, http.StatusNotFound)
		return
	}
	upgrader := websocket.Upgrader{Subprotocols: []string{"opentunnel"}, CheckOrigin: func(*http.Request) bool { return true }}
	ws, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer ws.Close()

	// The first message must be an attach carrying the token.
	_, raw, err := ws.ReadMessage()
	if err != nil {
		return
	}
	var attach struct {
		Type   string   `json:"type"`
		Token  string   `json:"token"`
		Routes []string `json:"routes"`
		Client struct {
			MaxConns int `json:"max_conns"`
		} `json:"client"`
	}
	_ = json.Unmarshal(raw, &attach)
	f.mu.Lock()
	token := f.token
	hb := f.heartbeatMs
	f.mu.Unlock()
	if attach.Type != "attach" || attach.Token != token {
		_ = ws.WriteJSON(map[string]any{"type": "attach_error", "code": "bad_token"})
		return
	}
	if len(attach.Routes) != 1 || attach.Routes[0] != "@" || attach.Client.MaxConns != 256 {
		f.t.Errorf("relay: unexpected attach: %s", raw)
	}
	_ = ws.WriteJSON(map[string]any{
		"type": "attached", "session": "s1", "routes": attach.Routes,
		"heartbeat_ms": hb, "idle_timeout_ms": 45000,
	})

	f.wsMu.Lock()
	f.ws = ws
	f.wsMu.Unlock()
	f.mu.Lock()
	f.attaches++
	f.mu.Unlock()

	for {
		kind, payload, err := ws.ReadMessage()
		if err != nil {
			f.wsMu.Lock()
			if f.ws == ws {
				f.ws = nil
			}
			f.wsMu.Unlock()
			return
		}
		switch kind {
		case websocket.TextMessage:
			var msg map[string]any
			if json.Unmarshal(payload, &msg) != nil {
				continue
			}
			f.dispatchControl(msg)
			select {
			case f.controls <- msg:
			default:
			}
		case websocket.BinaryMessage:
			if len(payload) < 4 {
				continue
			}
			f.wsMu.Lock()
			rc := f.conns[binary.BigEndian.Uint32(payload)]
			f.wsMu.Unlock()
			if rc != nil {
				rc.mu.Lock()
				rc.buf = append(rc.buf, payload[4:]...)
				rc.mu.Unlock()
				rc.cond.Broadcast()
			}
		}
	}
}

func (f *fakeRelay) dispatchControl(msg map[string]any) {
	id, _ := msg["conn"].(float64)
	f.wsMu.Lock()
	rc := f.conns[uint32(id)]
	f.wsMu.Unlock()
	if rc == nil {
		return
	}
	switch msg["type"] {
	case "end":
		rc.mu.Lock()
		rc.eof = true
		rc.mu.Unlock()
		rc.cond.Broadcast()
	case "reset":
		rc.mu.Lock()
		rc.reset, _ = msg["code"].(string)
		rc.mu.Unlock()
		rc.cond.Broadcast()
	}
}

func (f *fakeRelay) write(kind int, payload []byte) error {
	f.wsMu.Lock()
	defer f.wsMu.Unlock()
	if f.ws == nil {
		return io.ErrClosedPipe
	}
	return f.ws.WriteMessage(kind, payload)
}

func (f *fakeRelay) writeJSON(v any) error {
	raw, _ := json.Marshal(v)
	return f.write(websocket.TextMessage, raw)
}

// dropBridge closes the client's bridge, as a network drop would.
func (f *fakeRelay) dropBridge() {
	f.wsMu.Lock()
	ws := f.ws
	f.wsMu.Unlock()
	if ws != nil {
		_ = ws.Close()
	}
}

func (f *fakeRelay) attachCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.attaches
}

func (f *fakeRelay) createdCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.created
}

func (f *fakeRelay) currentHostname() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.hostname
}

// open announces a visitor connection for the given SNI.
func (f *fakeRelay) open(sni string) *relayConn {
	f.wsMu.Lock()
	f.next++
	rc := &relayConn{relay: f, id: f.next}
	rc.cond = sync.NewCond(&rc.mu)
	f.conns[rc.id] = rc
	f.wsMu.Unlock()
	_ = f.writeJSON(map[string]any{"type": "open", "conn": rc.id, "peer": "203.0.113.9:5555", "sni": sni, "alpn": ""})
	return rc
}

// get makes an HTTPS request through the tunnel, verifying the certificate
// the client presents against the test CA.
func (f *fakeRelay) get(host, path string) (*http.Response, error) {
	pool := x509.NewCertPool()
	pool.AddCert(f.caCert)
	rc := f.open(host)
	conn := tls.Client(rc, &tls.Config{ServerName: host, RootCAs: pool, NextProtos: []string{"http/1.1"}})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := conn.HandshakeContext(ctx); err != nil {
		return nil, fmt.Errorf("handshake: %w", err)
	}
	req, _ := http.NewRequest(http.MethodGet, "https://"+host+path, nil)
	req.Header.Set("Connection", "close")
	if err := req.Write(conn); err != nil {
		return nil, err
	}
	return http.ReadResponse(bufio.NewReader(conn), req)
}

func waitUntil(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(15 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

// localApp is the server being exposed.
func localApp(t *testing.T) (target string, hits *atomic.Int32) {
	t.Helper()
	hits = &atomic.Int32{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		_, _ = fmt.Fprintf(w, "hello from the local server: %s", r.URL.Path)
	}))
	t.Cleanup(srv.Close)
	return srv.Listener.Addr().String(), hits
}

func newManager(t *testing.T, relay *fakeRelay, dir, target string) (*opentunnel.Manager, chan string) {
	t.Helper()
	ready := make(chan string, 8)
	m := &opentunnel.Manager{
		API: relay.server.URL, Dir: dir, Target: target, PollInterval: 10 * time.Millisecond,
		OnReady: func(u string) { ready <- u },
	}
	return m, ready
}

func startManager(t *testing.T, m *opentunnel.Manager) {
	t.Helper()
	if err := m.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(m.Close)
}

func readyURL(t *testing.T, ready chan string) string {
	t.Helper()
	select {
	case u := <-ready:
		return u
	case <-time.After(10 * time.Second):
		t.Fatal("the tunnel never became ready")
		return ""
	}
}

// A first start creates the tunnel, gets a certificate for exactly its names,
// and serves a real HTTPS request through the bridge to the local server.
func TestOpenTunnelFirstStartServesTraffic(t *testing.T) {
	relay := newFakeRelay(t)
	relay.issuingFor = 2
	target, hits := localApp(t)
	m, ready := newManager(t, relay, t.TempDir(), target)
	startManager(t, m)

	url := readyURL(t, ready)
	host := relay.currentHostname()
	if url != "https://"+host || m.URL() != url {
		t.Fatalf("url = %q (manager %q), want https://%s", url, m.URL(), host)
	}
	waitUntil(t, "the bridge to attach", func() bool { return relay.attachCount() == 1 })

	resp, err := relay.get(host, "/ping")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 || string(body) != "hello from the local server: /ping" {
		t.Fatalf("response = %d %q", resp.StatusCode, body)
	}
	if hits.Load() != 1 {
		t.Fatalf("the local server saw %d requests, want 1", hits.Load())
	}

	// The request was for the tunnel's own names only, signed by the client's key.
	block, _ := pem.Decode([]byte(relay.csrPEM))
	csr, err := x509.ParseCertificateRequest(block.Bytes)
	if err != nil || csr.CheckSignature() != nil {
		t.Fatalf("certificate request: %v", err)
	}
	if csr.Subject.CommonName != host || len(csr.DNSNames) != 2 || csr.DNSNames[0] != host || csr.DNSNames[1] != "*."+host {
		t.Fatalf("certificate request names = %q %v", csr.Subject.CommonName, csr.DNSNames)
	}
}

// The identity is written for the owner only, in the protocol's layout.
func TestOpenTunnelStoresIdentityPrivately(t *testing.T) {
	relay := newFakeRelay(t)
	dir := t.TempDir()
	target, _ := localApp(t)
	m, ready := newManager(t, relay, dir, target)
	startManager(t, m)
	readyURL(t, ready)

	for _, name := range []string{"tunnel.json", "token", "private-key.pem", "certificate.pem", "chain.pem"} {
		info, err := os.Stat(filepath.Join(dir, name))
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if info.Mode().Perm() != 0o600 {
			t.Errorf("%s mode = %v, want 0600", name, info.Mode().Perm())
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "pending.json")); !os.IsNotExist(err) {
		t.Errorf("pending.json should be gone once the certificate is ready")
	}
	info, ok := opentunnel.ReadInfo(dir)
	if !ok || !info.Ready || info.URL != m.URL() {
		t.Fatalf("ReadInfo = %+v %v, want ready %s", info, ok, m.URL())
	}
}

// A restart resumes the same tunnel: same URL, no new tunnel created.
func TestOpenTunnelRestartKeepsTheURL(t *testing.T) {
	relay := newFakeRelay(t)
	dir := t.TempDir()
	target, _ := localApp(t)

	first, ready := newManager(t, relay, dir, target)
	if err := first.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	url1 := readyURL(t, ready)
	waitUntil(t, "the first attach", func() bool { return relay.attachCount() == 1 })
	first.Close()

	second, ready2 := newManager(t, relay, dir, target)
	startManager(t, second)
	url2 := readyURL(t, ready2)
	if url2 != url1 {
		t.Fatalf("url changed across a restart: %s -> %s", url1, url2)
	}
	if relay.createdCount() != 1 {
		t.Fatalf("created %d tunnels, want 1", relay.createdCount())
	}
	waitUntil(t, "the second attach", func() bool { return relay.attachCount() == 2 })
	if resp, err := relay.get(relay.currentHostname(), "/again"); err != nil {
		t.Fatal(err)
	} else {
		resp.Body.Close()
	}
}

// A restart in the middle of provisioning resumes the pending tunnel instead
// of leaking another one.
func TestOpenTunnelResumesInterruptedProvisioning(t *testing.T) {
	relay := newFakeRelay(t)
	relay.issuingFor = 1 << 20 // the certificate never becomes ready
	dir := t.TempDir()
	target, _ := localApp(t)

	first, _ := newManager(t, relay, dir, target)
	if err := first.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	if info, ok := opentunnel.ReadInfo(dir); !ok || info.Ready {
		t.Fatalf("ReadInfo = %+v %v, want a pending tunnel", info, ok)
	}
	first.Close()

	relay.mu.Lock()
	relay.issuingFor = 0
	relay.polls = 0
	relay.mu.Unlock()
	second, ready := newManager(t, relay, dir, target)
	startManager(t, second)
	readyURL(t, ready)
	if relay.createdCount() != 1 {
		t.Fatalf("created %d tunnels, want 1", relay.createdCount())
	}
}

// A dropped bridge reconnects on the same tunnel.
func TestOpenTunnelReconnectsAfterADrop(t *testing.T) {
	relay := newFakeRelay(t)
	target, _ := localApp(t)
	m, ready := newManager(t, relay, t.TempDir(), target)
	startManager(t, m)
	url := readyURL(t, ready)
	waitUntil(t, "the first attach", func() bool { return relay.attachCount() == 1 })

	relay.dropBridge()
	waitUntil(t, "a reconnect", func() bool { return relay.attachCount() == 2 })
	if m.URL() != url || relay.createdCount() != 1 {
		t.Fatalf("the tunnel changed on reconnect: %s, created %d", m.URL(), relay.createdCount())
	}
	resp, err := relay.get(relay.currentHostname(), "/after")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
}

// A tunnel the service forgot is replaced, and only then does the URL change.
func TestOpenTunnelIsReplacedWhenTheServiceForgetsIt(t *testing.T) {
	relay := newFakeRelay(t)
	target, _ := localApp(t)
	m, ready := newManager(t, relay, t.TempDir(), target)
	startManager(t, m)
	url1 := readyURL(t, ready)
	waitUntil(t, "the first attach", func() bool { return relay.attachCount() == 1 })

	relay.mu.Lock()
	relay.gone = true
	relay.mu.Unlock()
	relay.dropBridge()
	waitUntil(t, "the replacement tunnel", func() bool { return relay.createdCount() == 2 })

	url2 := readyURL(t, ready)
	if url2 == url1 {
		t.Fatalf("the url should change when the tunnel is replaced")
	}
	if url2 != "https://"+relay.currentHostname() {
		t.Fatalf("url = %s, want the new tunnel's", url2)
	}
}

// Creating a tunnel retries a busy service instead of failing the start.
func TestOpenTunnelCreateRetriesWhenTheServiceIsBusy(t *testing.T) {
	relay := newFakeRelay(t)
	relay.failCreates = 2
	target, _ := localApp(t)
	m, ready := newManager(t, relay, t.TempDir(), target)
	startManager(t, m)
	readyURL(t, ready)
	relay.mu.Lock()
	calls := relay.createCalls
	relay.mu.Unlock()
	if calls != 3 || relay.createdCount() != 1 {
		t.Fatalf("create calls = %d, created = %d, want 3 calls and 1 tunnel", calls, relay.createdCount())
	}
}

// Only the tunnel's own hostname is served: any other name is refused, and
// the relay is told which connection to drop.
func TestOpenTunnelRefusesUnknownRoutes(t *testing.T) {
	relay := newFakeRelay(t)
	target, hits := localApp(t)
	m, ready := newManager(t, relay, t.TempDir(), target)
	startManager(t, m)
	readyURL(t, ready)
	waitUntil(t, "the bridge to attach", func() bool { return relay.attachCount() == 1 })

	rc := relay.open("api." + relay.currentHostname())
	deadline := time.After(5 * time.Second)
	for {
		select {
		case msg := <-relay.controls:
			if msg["type"] == "reset" && msg["code"] == "unknown_route" && uint32(msg["conn"].(float64)) == rc.id {
				if hits.Load() != 0 {
					t.Fatalf("the local server must not be reached for an unknown route")
				}
				return
			}
		case <-deadline:
			t.Fatal("no unknown_route reset")
		}
	}
}

// The relay's ping is answered with the same timestamp.
func TestOpenTunnelAnswersPings(t *testing.T) {
	relay := newFakeRelay(t)
	target, _ := localApp(t)
	m, ready := newManager(t, relay, t.TempDir(), target)
	startManager(t, m)
	readyURL(t, ready)
	waitUntil(t, "the bridge to attach", func() bool { return relay.attachCount() == 1 })

	if err := relay.writeJSON(map[string]any{"type": "ping", "time_sent": 1234567}); err != nil {
		t.Fatal(err)
	}
	deadline := time.After(5 * time.Second)
	for {
		select {
		case msg := <-relay.controls:
			if msg["type"] == "pong" {
				if msg["time_sent"].(float64) != 1234567 {
					t.Fatalf("pong time_sent = %v", msg["time_sent"])
				}
				return
			}
		case <-deadline:
			t.Fatal("no pong")
		}
	}
}

// Many visitors at once do not interfere with each other.
func TestOpenTunnelServesConcurrentVisitors(t *testing.T) {
	relay := newFakeRelay(t)
	target, hits := localApp(t)
	m, ready := newManager(t, relay, t.TempDir(), target)
	startManager(t, m)
	readyURL(t, ready)
	waitUntil(t, "the bridge to attach", func() bool { return relay.attachCount() == 1 })

	host := relay.currentHostname()
	var wg sync.WaitGroup
	errs := make(chan error, 20)
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			path := fmt.Sprintf("/v%d", i)
			resp, err := relay.get(host, path)
			if err != nil {
				errs <- err
				return
			}
			defer resp.Body.Close()
			body, _ := io.ReadAll(resp.Body)
			if want := "hello from the local server: " + path; string(body) != want {
				errs <- fmt.Errorf("got %q, want %q", body, want)
			}
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Error(err)
	}
	if hits.Load() != 20 {
		t.Fatalf("the local server saw %d requests, want 20", hits.Load())
	}
}
