// Localhost port discovery + reverse proxy. Port of
// apps/server/api/src/services/port-registry.service.ts: observe terminal
// and job output for localhost URLs, probe liveness, allocate a proxy port
// per entry (HTTP + WebSocket passthrough), reap dead entries.
package services

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/types"
)

const (
	defaultProxyStart = 45000
	defaultProxyEnd   = 45999
	probeTimeout      = 2 * time.Second
)

var (
	candidatePattern = regexp.MustCompile(`(?i)(?:https?://)?(?:localhost|127\.0\.0\.1|0\.0\.0\.0|\[::1\])(?::|\s*:\s*)(\d{1,5})`)
	osc8Pattern      = regexp.MustCompile("\x1b]8;[^\x1b\x07]*?(?:\x07|\x1b\\\\)([\\s\\S]*?)\x1b]8;[^\x1b\x07]*?(?:\x07|\x1b\\\\)")
	oscPattern       = regexp.MustCompile("\x1b][^\x07]*(?:\x07|\x1b\\\\)")
	csiPattern       = regexp.MustCompile("\x1b\\[[0-?]*[ -/]*[@-~]")
)

type PortOwner struct {
	Kind string // terminal | job
	ID   string
}

type portEntry struct {
	Port      int
	ProxyPort int
	Owner     *PortOwner
	Manual    bool
	ProjectID string
	proxy     *http.Server
}

type PortRegistry struct {
	mu         sync.Mutex
	entries    map[int]*portEntry
	ownerBuf   map[string]string
	ownerEpoch map[string]int
	proxyStart int
	proxyEnd   int
	nextProxy  int
	snapshot   []types.ClientPort
	subs       map[chan []types.ClientPort]bool
	reaping    bool
	closed     bool
}

func NewPortRegistry() *PortRegistry {
	return &PortRegistry{
		entries:    make(map[int]*portEntry),
		ownerBuf:   make(map[string]string),
		ownerEpoch: make(map[string]int),
		proxyStart: rangeFromEnv("PROXY_PORT_START", defaultProxyStart),
		proxyEnd:   rangeFromEnv("PROXY_PORT_END", defaultProxyEnd),
		nextProxy:  rangeFromEnv("PROXY_PORT_START", defaultProxyStart),
		subs:       make(map[chan []types.ClientPort]bool),
	}
}

func rangeFromEnv(name string, fallback int) int {
	v, err := strconv.Atoi(osGetenv(name))
	if err == nil && v >= 1024 && v <= 65535 {
		return v
	}
	return fallback
}

// sanitizeTerminalOutput strips CSI/OSC sequences and expands OSC-8
// hyperlinks so piped vite URLs survive.
func sanitizeTerminalOutput(text string) string {
	return csiPattern.ReplaceAllString(oscPattern.ReplaceAllString(
		osc8Pattern.ReplaceAllString(text, "$1 "), ""), "")
}

// ObserveOutput scans output for localhost port candidates and registers
// listening ones. owner identifies the terminal or job emitting it.
func (r *PortRegistry) ObserveOutput(owner PortOwner, text, projectID string) {
	key := owner.Kind + ":" + owner.ID
	r.mu.Lock()
	combined := r.ownerBuf[key] + sanitizeTerminalOutput(text)
	lines := strings.Split(combined, "\n")
	r.ownerBuf[key] = lines[len(lines)-1]
	r.mu.Unlock()

	candidates := make(map[int]bool)
	for _, line := range lines {
		for _, match := range candidatePattern.FindAllStringSubmatch(line, -1) {
			port, _ := strconv.Atoi(match[1])
			if port >= 1024 && port <= 65535 {
				candidates[port] = true
			}
		}
	}
	for port := range candidates {
		r.registerDetected(port, owner, projectID)
	}
}

func isListening(port int) bool {
	// Newer Node/Vite builds bind the IPv6 loopback ([::1]) on macOS while
	// older ones bind 127.0.0.1 — probe both before declaring the port dead.
	if conn, err := net.DialTimeout("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)), probeTimeout); err == nil {
		conn.Close()
		return true
	}
	conn, err := net.DialTimeout("tcp", net.JoinHostPort("[::1]", strconv.Itoa(port)), probeTimeout)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

func (r *PortRegistry) registerDetected(port int, owner PortOwner, projectID string) {
	key := owner.Kind + ":" + owner.ID
	r.mu.Lock()
	epoch := r.ownerEpoch[key]
	_, exists := r.entries[port]
	r.mu.Unlock()
	if exists {
		return
	}
	if !isListening(port) {
		return
	}
	// Late probe guard: owner may have exited mid-probe.
	r.mu.Lock()
	if r.ownerEpoch[key] != epoch {
		r.mu.Unlock()
		return
	}
	r.mu.Unlock()
	_, _ = r.createEntry(port, &PortOwner{Kind: owner.Kind, ID: owner.ID}, false, projectID)
}

// Forward manually registers a listening port.
func (r *PortRegistry) Forward(port int, projectID string) (types.ClientPort, error) {
	if port < 1024 || port > 65535 {
		return types.ClientPort{}, fmt.Errorf("Port must be between 1024 and 65535.")
	}
	if !isListening(port) {
		return types.ClientPort{}, fmt.Errorf("Port %d is not listening on 127.0.0.1.", port)
	}
	r.mu.Lock()
	existing := r.entries[port]
	r.mu.Unlock()
	if existing != nil {
		if existing.ProjectID == "" && projectID != "" {
			r.mu.Lock()
			existing.ProjectID = projectID
			r.rebuildSnapshotLocked()
			r.mu.Unlock()
			r.notify()
		}
		return r.clientEntry(existing, "localhost"), nil
	}
	entry, err := r.createEntry(port, nil, true, projectID)
	if err != nil {
		return types.ClientPort{}, err
	}
	return r.clientEntry(entry, "localhost"), nil
}

func (r *PortRegistry) createEntry(port int, owner *PortOwner, manual bool, projectID string) (*portEntry, error) {
	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		return nil, fmt.Errorf("registry closed")
	}
	proxyPort := 0
	for p := r.nextProxy; p <= r.proxyEnd; p++ {
		if _, taken := r.entriesByProxy(p); !taken {
			proxyPort = p
			break
		}
	}
	if proxyPort == 0 {
		r.mu.Unlock()
		return nil, fmt.Errorf("No free proxy ports in range %d-%d.", r.proxyStart, r.proxyEnd)
	}
	r.nextProxy++
	if r.nextProxy > r.proxyEnd {
		r.nextProxy = r.proxyStart
	}
	r.mu.Unlock()

	target, _ := url.Parse(fmt.Sprintf("http://127.0.0.1:%d", port))
	proxy := &httputil.ReverseProxy{Rewrite: func(pr *httputil.ProxyRequest) {
		pr.SetURL(target)
		pr.Out.Host = pr.In.Host
	}}
	server := &http.Server{
		Addr:              fmt.Sprintf("0.0.0.0:%d", proxyPort),
		Handler:           proxy, // ReverseProxy passes WebSocket upgrades through
		ReadHeaderTimeout: 10 * time.Second,
	}
	entry := &portEntry{
		Port: port, ProxyPort: proxyPort, Owner: owner, Manual: manual, ProjectID: projectID, proxy: server,
	}
	go func() {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Warn("port proxy stopped", "port", port, "proxy", proxyPort, "error", err)
		}
	}()

	r.mu.Lock()
	r.entries[port] = entry
	r.rebuildSnapshotLocked()
	r.mu.Unlock()
	r.notify()
	return entry, nil
}

// entriesByProxy is called with the mutex held.
func (r *PortRegistry) entriesByProxy(proxyPort int) (*portEntry, bool) {
	for _, e := range r.entries {
		if e.ProxyPort == proxyPort {
			return e, true
		}
	}
	return nil, false
}

// List probes liveness and drops dead entries (like the TS list()).
func (r *PortRegistry) List(host, projectID string) []types.ClientPort {
	r.mu.Lock()
	entries := make([]*portEntry, 0, len(r.entries))
	for _, e := range r.entries {
		entries = append(entries, e)
	}
	r.mu.Unlock()
	for _, e := range entries {
		if !isListening(e.Port) {
			r.Remove(e.Port, "")
		}
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]types.ClientPort, 0)
	for _, e := range r.entries {
		if matchesProject(e.ProjectID, projectID) {
			out = append(out, clientEntryOf(e, host))
		}
	}
	sortPorts(out)
	return out
}

// Snapshot returns the cached, probe-free list (for SSE streams).
func (r *PortRegistry) Snapshot(host, projectID string) []types.ClientPort {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]types.ClientPort, 0, len(r.snapshot))
	for _, p := range r.snapshot {
		if p.ProjectID == nil || projectID == "" || *p.ProjectID == projectID {
			out = append(out, p)
		}
	}
	sortPorts(out)
	return out
}

func (r *PortRegistry) Remove(port int, projectID string) bool {
	r.mu.Lock()
	entry := r.entries[port]
	if entry == nil {
		r.mu.Unlock()
		return false
	}
	if projectID != "" && entry.ProjectID != "" && entry.ProjectID != projectID {
		r.mu.Unlock()
		return false
	}
	delete(r.entries, port)
	r.rebuildSnapshotLocked()
	r.mu.Unlock()
	if entry.proxy != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = entry.proxy.Shutdown(ctx)
	}
	r.notify()
	return true
}

// RemoveOwner drops every entry owned by a terminal/job and bumps its epoch
// so in-flight probes can't resurrect them.
func (r *PortRegistry) RemoveOwner(owner PortOwner) {
	r.mu.Lock()
	doomed := make([]*portEntry, 0)
	for _, e := range r.entries {
		if e.Owner != nil && e.Owner.Kind == owner.Kind && e.Owner.ID == owner.ID {
			doomed = append(doomed, e)
		}
	}
	key := owner.Kind + ":" + owner.ID
	delete(r.ownerBuf, key)
	r.ownerEpoch[key]++
	r.mu.Unlock()
	for _, e := range doomed {
		r.Remove(e.Port, "")
	}
}

func (r *PortRegistry) CloseAll() {
	r.mu.Lock()
	ports := make([]int, 0, len(r.entries))
	for p := range r.entries {
		ports = append(ports, p)
	}
	r.closed = true
	r.mu.Unlock()
	for _, p := range ports {
		r.Remove(p, "")
	}
}

// StartReaper runs the background liveness sweep.
func (r *PortRegistry) StartReaper(interval time.Duration) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for range ticker.C {
			r.reap()
		}
	}()
}

func (r *PortRegistry) reap() {
	r.mu.Lock()
	if r.reaping || len(r.entries) == 0 {
		r.mu.Unlock()
		return
	}
	r.reaping = true
	ports := make([]int, 0, len(r.entries))
	for p := range r.entries {
		ports = append(ports, p)
	}
	r.mu.Unlock()
	// Probe concurrently so one slow port (filtered connection, 2s dial
	// timeout) can't stretch the whole sweep — sequential probing let
	// removals lag far behind the reaper interval.
	var wg sync.WaitGroup
	for _, p := range ports {
		wg.Add(1)
		go func(port int) {
			defer wg.Done()
			if !isListening(port) {
				r.Remove(port, "")
			}
		}(p)
	}
	wg.Wait()
	r.mu.Lock()
	r.reaping = false
	r.mu.Unlock()
}

// rebuildSnapshotLocked must be called with the mutex held.
func (r *PortRegistry) rebuildSnapshotLocked() {
	snap := make([]types.ClientPort, 0, len(r.entries))
	for _, e := range r.entries {
		snap = append(snap, clientEntryOf(e, "localhost"))
	}
	r.snapshot = snap
}

func (r *PortRegistry) clientEntry(e *portEntry, host string) types.ClientPort {
	r.mu.Lock()
	defer r.mu.Unlock()
	return clientEntryOf(e, host)
}

func clientEntryOf(e *portEntry, host string) types.ClientPort {
	var pid *string
	if e.ProjectID != "" {
		id := e.ProjectID
		pid = &id
	}
	host = strings.TrimSuffix(host, regexp.MustCompile(`:\d+$`).FindString(host))
	return types.ClientPort{Port: e.Port, URL: fmt.Sprintf("http://%s:%d", host, e.ProxyPort), ProjectID: pid}
}

func matchesProject(entryProject, filter string) bool {
	if filter == "" || entryProject == "" {
		return true
	}
	return entryProject == filter
}

func sortPorts(ports []types.ClientPort) {
	for i := 1; i < len(ports); i++ {
		for j := i; j > 0 && ports[j].Port < ports[j-1].Port; j-- {
			ports[j], ports[j-1] = ports[j-1], ports[j]
		}
	}
}

func (r *PortRegistry) notify() {
	r.mu.Lock()
	snap := r.snapshot
	subs := make([]chan []types.ClientPort, 0, len(r.subs))
	for ch := range r.subs {
		subs = append(subs, ch)
	}
	r.mu.Unlock()
	for _, ch := range subs {
		select {
		case ch <- snap:
		default:
		}
	}
}

func (r *PortRegistry) Subscribe() chan []types.ClientPort {
	ch := make(chan []types.ClientPort, 16)
	r.mu.Lock()
	r.subs[ch] = true
	r.mu.Unlock()
	return ch
}

func (r *PortRegistry) Unsubscribe(ch chan []types.ClientPort) {
	r.mu.Lock()
	delete(r.subs, ch)
	r.mu.Unlock()
}
