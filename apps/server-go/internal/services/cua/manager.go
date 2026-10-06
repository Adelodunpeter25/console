// Manager owns the process-wide Cua Driver runtime and turns its tool
// inventory into harness tools. Cua owns global UI and executor threads, so
// exactly one driver is created, lazily, and kept for the life of the process.
package cua

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/tools"
)

// Manager hands out the shared driver and lazily materialises its tools.
type Manager struct {
	mu      sync.Mutex
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
}

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

// stopped reports whether a stop is in force, for the per-call cancel hook.
// Stopped reports whether a stop is in force. This is the per-call cancel hook
// handed to the driver, so a Stop interrupts the action already in flight
// rather than only refusing the next one.
func (m *Manager) Stopped() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.stoppedFlag
}

// Tools materialises the driver's inventory as harness tools.
//
// Each tool keeps Cua's own input schema, so the model sees the real argument
// names rather than a flattened guess. Tiering comes from Cua's risk metadata
// when the driver advertises it, because Cua's classes do not follow the name
// prefixes Console's heuristic assumes: kill_app is not an exec-by-name tool,
// it is R3 with a bespoke rule.
func (m *Manager) Tools() ([]tools.Tool, error) {
	driver, err := m.Driver()
	if err != nil {
		return nil, err
	}
	inventory, err := driver.ListTools()
	if err != nil {
		return nil, err
	}
	out := make([]tools.Tool, 0, len(inventory))
	for _, def := range inventory {
		if def.Name == "" {
			continue
		}
		out = append(out, NewTool(m, driver, def))
	}
	return out, nil
}

// ToolName maps a Cua tool name onto a harness-safe name. Cua's names are
// already snake_case and valid, so this only guards against a name the
// provider would reject and against the 64-char cap.
func ToolName(cuaName string) string {
	name := "cua__" + unsafeName.ReplaceAllString(cuaName, "_")
	if len(name) > 64 {
		name = name[:64]
	}
	return name
}

// Tier maps one advertised tool onto a Console tool tier, using Cua's own
// assessment. readOnlyHint wins when present because it is the driver's own
// statement about the tool rather than an inference.
//
// r0 is metadata with no user payload, r1 is local reversible control, and r2
// and above reveal or change state the user cares about. Unclassified is
// deliberately exec: a tool Cua has not reviewed must not be waved through.
func Tier(def ToolDef) tools.ToolTier {
	if def.ReadOnly() {
		return tools.TierRead
	}
	switch def.RiskClass() {
	case "r0":
		return tools.TierRead
	case "r1":
		return tools.TierWrite
	case "r2", "r3", "r4":
		return tools.TierExec
	case "unclassified", "":
		// No reviewed classification: treat as exec rather than trusting a
		// name heuristic. Cua itself fails closed on this.
		return tools.TierExec
	default:
		return tools.TierExec
	}
}

// ToolResultParts converts a Cua result into the harness content-part shape:
// text stays text, images become native image parts carrying raw bytes, and
// structured content is used when there is nothing else to describe what
// happened. Exported because the same conversion is needed by any surface that
// replays a stored result.
func ToolResultParts(result *ToolResult) []map[string]any {
	if result == nil {
		return []map[string]any{{"type": "text", "text": "(no output)"}}
	}
	var text []string
	var out []map[string]any
	for _, part := range result.Content {
		switch part.Type {
		case "text":
			if strings.TrimSpace(part.Text) != "" {
				text = append(text, part.Text)
			}
		case "image":
			if len(part.Data) == 0 {
				continue
			}
			mimeType := part.MIMEType
			if mimeType == "" {
				mimeType = "image/png"
			}
			out = append(out, map[string]any{
				"type":     "image",
				"data":     base64Encode(part.Data),
				"mimeType": mimeType,
			})
		case "audio":
			text = append(text, "[audio content omitted]")
		case "resource_link":
			text = append(text, fmt.Sprintf("[resource %s: %s]", part.Name, part.URI))
		}
	}
	// Structured content is the driver's typed view of the same observation.
	// When there is no text at all it is the only description of what happened,
	// so it must not be dropped.
	if len(text) == 0 && len(out) == 0 && len(result.StructuredContent) > 0 {
		if raw, err := json.Marshal(result.StructuredContent); err == nil {
			text = append(text, string(raw))
		}
	}
	if len(text) == 0 && len(out) == 0 {
		text = append(text, "(no output)")
	}
	joined := strings.Join(text, "\n")
	if len(joined) > maxResultText {
		joined = joined[:maxResultText] + "\n… [truncated]"
	}
	return append([]map[string]any{{"type": "text", "text": joined}}, out...)
}

const maxResultText = 100_000

// ErrNoDriver reports that computer use is unavailable, with the reason.
type ErrNoDriver struct{ Cause error }

func (e *ErrNoDriver) Error() string {
	if e.Cause == nil {
		return "computer use is unavailable"
	}
	return "computer use is unavailable: " + e.Cause.Error()
}

func (e *ErrNoDriver) Unwrap() error { return e.Cause }

// runTimeout bounds one tool call. Cua's own reference hosts rely on the
// driver's own cancellation rather than a harness deadline, so this is generous
// and exists only to stop a wedged call from pinning a run forever.
const runTimeout = 5 * time.Minute
