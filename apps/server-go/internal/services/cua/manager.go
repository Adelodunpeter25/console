// Manager owns the process-wide Cua Driver runtime and turns its tool
// inventory into harness tools. Cua owns global UI and executor threads, so
// exactly one driver is created, lazily, and kept for the life of the process.
package cua

import (
	"context"
	"sort"
	"sync"
	"time"
)

// Manager hands out the shared driver and lazily materialises its tools.
type Manager struct {
	mu      sync.Mutex
	jsMu    sync.Mutex
	driver  *Driver
	initErr error
	loading bool
	ready   chan struct{}
	// toolNames is the inventory as last observed, so Status can report the
	// tool surface even before any tool has been called.
	toolNames []string
	// stoppedFlag records that a Stop is in force. A Stop both refuses new
	// work and cancels whatever the driver already admitted.
	stoppedFlag bool
	// jsRuntimes is one persistent JavaScript kernel per chat session, in
	// creation order for eviction. Sessions are unbounded over a server
	// lifetime, so the map is capped: evicting a runtime only loses that
	// session's variables, which the model rebuilds by re-observing.
	jsRuntimes map[string]*JSRuntime
	jsOrder    []string
}

// maxJSRuntimes caps live session kernels. Computer use is rare next to
// chat, so this bounds memory without ever triggering in practice.
const maxJSRuntimes = 16

// NewManager returns a manager that opens the driver on first use.
func NewManager() *Manager {
	return &Manager{ready: make(chan struct{})}
}

// Driver returns the shared runtime, opening it on first call. A failed open
// is cached so a missing library does not re-pay the dlopen cost per tool call.
func (m *Manager) Driver() (*Driver, error) {
	m.mu.Lock()
	if m.driver != nil {
		d := m.driver
		m.mu.Unlock()
		return d, nil
	}
	if m.initErr != nil {
		err := m.initErr
		m.mu.Unlock()
		return nil, err
	}
	if m.loading {
		ready := m.ready
		m.mu.Unlock()
		<-ready
		return m.Driver()
	}
	m.loading = true
	ready := m.ready
	m.mu.Unlock()

	driver, err := Open(Options{})
	if driver != nil {
		if names, listErr := driverTools(driver); listErr == nil {
			m.mu.Lock()
			m.toolNames = names
			m.mu.Unlock()
		}
	}

	m.mu.Lock()
	m.loading = false
	m.driver, m.initErr = driver, err
	close(ready)
	m.mu.Unlock()
	return driver, err
}

func driverTools(driver *Driver) ([]string, error) {
	list, err := driver.ListTools()
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(list))
	for _, t := range list {
		names = append(names, t.Name)
	}
	sort.Strings(names)
	return names, nil
}

// sessionRuntime returns the session's JavaScript kernel, creating it on
// first use. Creation opens the driver, so an unavailable library fails here
// rather than at some later, more confusing point.
func (m *Manager) sessionRuntime(sessionID string) (*JSRuntime, error) {
	m.jsMu.Lock()
	defer m.jsMu.Unlock()
	if runtime, ok := m.jsRuntimes[sessionID]; ok {
		m.touchRuntime(sessionID)
		return runtime, nil
	}
	driver, err := m.Driver()
	if err != nil {
		return nil, err
	}
	runtime, err := NewJSRuntime(driver)
	if err != nil {
		return nil, err
	}
	runtime.cancel = m.Stopped
	if m.jsRuntimes == nil {
		m.jsRuntimes = map[string]*JSRuntime{}
	}
	m.jsRuntimes[sessionID] = runtime
	m.jsOrder = append(m.jsOrder, sessionID)
	for len(m.jsOrder) > maxJSRuntimes {
		oldest := m.jsOrder[0]
		m.jsOrder = m.jsOrder[1:]
		// A session holding the runtime elsewhere keeps working; only the
		// map entry goes, so the next call starts that session fresh.
		delete(m.jsRuntimes, oldest)
	}
	return runtime, nil
}

// touchRuntime moves a session to the back of the eviction order.
func (m *Manager) touchRuntime(sessionID string) {
	for i, id := range m.jsOrder {
		if id == sessionID {
			m.jsOrder = append(append(m.jsOrder[:i:i], m.jsOrder[i+1:]...), sessionID)
			return
		}
	}
}

// dropRuntime forgets a session's kernel: variables and bindings are gone,
// and the next call starts fresh. Safe to call for unknown sessions.
func (m *Manager) dropRuntime(sessionID string) {
	m.jsMu.Lock()
	defer m.jsMu.Unlock()
	delete(m.jsRuntimes, sessionID)
	for i, id := range m.jsOrder {
		if id == sessionID {
			m.jsOrder = append(m.jsOrder[:i:i], m.jsOrder[i+1:]...)
			return
		}
	}
}

// SessionRuntimeCount reports how many session kernels are live, for tests
// and status surfaces.
func (m *Manager) SessionRuntimeCount() int {
	m.jsMu.Lock()
	defer m.jsMu.Unlock()
	return len(m.jsRuntimes)
}

// Available reports whether the driver loaded, without attempting to open it.
func (m *Manager) Available() bool {
	m.mu.Lock()
	_, err := m.driver, m.initErr
	m.mu.Unlock()
	return m.driver != nil && err == nil
}

// Status describes the runtime for a status endpoint or a CLI probe.
type Status struct {
	Available bool     `json:"available"`
	Error     string   `json:"error,omitempty"`
	Tools     []string `json:"tools,omitempty"`
	// PermissionMode is what the runtime was opened with. Computer use runs
	// unrestricted, so there is no approval gate and no human in the loop.
	PermissionMode string `json:"permissionMode,omitempty"`
	ABIMajor       int    `json:"abiMajor,omitempty"`
	ABIMinor       int    `json:"abiMinor,omitempty"`
	ABIPatch       int    `json:"abiPatch,omitempty"`
	LibraryPath    string `json:"libraryPath,omitempty"`
}

// Status probes the library and reports without failing the caller.
func (m *Manager) Status() Status {
	out := Status{PermissionMode: "unrestricted"}
	if path, err := LibraryPathOverride(); err == nil {
		out.LibraryPath = path
	}
	major, minor, patch, err := ABIVersion()
	if err != nil {
		out.Error = err.Error()
		return out
	}
	out.ABIMajor, out.ABIMinor, out.ABIPatch = major, minor, patch

	m.mu.Lock()
	driver, initErr, names := m.driver, m.initErr, m.toolNames
	m.mu.Unlock()
	if driver == nil {
		// Not opened yet: the ABI probe above is the useful signal, and a
		// library that loads cleanly means computer use is installable.
		out.Available = true
		return out
	}
	out.Available = true
	out.Tools = names
	if initErr != nil {
		out.Error = initErr.Error()
	}
	return out
}

// Stop refuses new work and cancels anything admitted. An action already in
// flight may still take effect; its outcome is reported as unknown rather than
// rolled back, because the driver cannot undo an action it has already sent.
func (m *Manager) Stop() {
	m.mu.Lock()
	m.stoppedFlag = true
	driver := m.driver
	m.mu.Unlock()
	if driver != nil {
		driver.Stop()
	}
}

// Resume clears a stop so the driver accepts work again.
func (m *Manager) Resume() {
	m.mu.Lock()
	m.stoppedFlag = false
	driver := m.driver
	m.mu.Unlock()
	if driver != nil {
		driver.Resume()
	}
}

// Close shuts the runtime down. It is safe to call more than once.
func (m *Manager) Close() error {
	m.mu.Lock()
	driver := m.driver
	m.driver = nil
	m.mu.Unlock()
	if driver == nil {
		return nil
	}
	return driver.Shutdown()
}

// EvalOnce runs code in a throwaway runtime: nothing persists afterwards.
// This is the CLI path (one process per invocation has no session to persist
// to) and the shape a single question takes when no follow-up needs the
// bindings. Sessions that need persistence go through Manager instead.
func (m *Manager) EvalOnce(ctx context.Context, code string, timeout time.Duration) (any, error) {
	driver, err := m.Driver()
	if err != nil {
		return nil, &ErrNoDriver{Cause: err}
	}
	runtime, err := NewJSRuntime(driver)
	if err != nil {
		return nil, err
	}
	return runtime.Eval(ctx, code, timeout)
}

// Stopped reports whether a stop is in force. This is the per-call cancel hook
// handed to the driver, so a Stop interrupts the action already in flight
// rather than only refusing the next one.
func (m *Manager) Stopped() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.stoppedFlag
}

// ErrNoDriver reports that computer use is unavailable, with the reason.
type ErrNoDriver struct{ Cause error }

func (e *ErrNoDriver) Error() string {
	if e.Cause == nil {
		return "computer use is unavailable"
	}
	return "computer use is unavailable: " + e.Cause.Error()
}

func (e *ErrNoDriver) Unwrap() error { return e.Cause }

// maxResultText caps text pulled out of one result, so a large observation
// cannot blow up the context by itself.
const maxResultText = 100_000
