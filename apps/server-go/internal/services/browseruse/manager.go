// Manager owns one browser kernel per chat session: a persistent JavaScript
// runtime with a preinstalled `browser` object, plus the set of tabs that
// kernel opened. The kernel is what /browser-use activates; it lets one
// bounded call navigate, wait, extract and clean up instead of one low-level
// `browser` action per step. See docs/plan/browser-use-kernel.md.
package browseruse

import (
	"sync"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/tools"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/services/jskernel"
)

// maxJSRuntimes caps live session kernels, like CUA. Eviction drops variables
// only: the owned-tab set lives here, so a later reset still closes what an
// evicted kernel opened.
const maxJSRuntimes = 16

// maxOwnedTabs caps the tabs one kernel may own at once. A research workflow
// needs a handful; more than that is usually a leak the reset should clean.
const maxOwnedTabs = 5

// Manager hands out per-session runtimes and tracks kernel-owned tabs.
type Manager struct {
	mu sync.Mutex
	js *jskernel.Cache[*JSRuntime]
	// owned maps a session to the tab ids its kernel opened (browser.open).
	// Closed or reset tabs are removed; the set survives runtime eviction.
	owned map[string]map[string]bool
}

// NewManager returns a manager with no runtimes and no owned tabs.
func NewManager() *Manager {
	return &Manager{js: jskernel.NewCache[*JSRuntime](maxJSRuntimes), owned: map[string]map[string]bool{}}
}

// runtime returns the session's kernel, creating it on first use. The handler
// is the run's bridge to the desktop client; a runtime restored by a later
// run rebinds to that run's handler, so actions always ride the live
// connection.
func (m *Manager) runtime(sessionID string, handler tools.BrowserHandler) (*JSRuntime, error) {
	rt, err := m.js.Get(sessionID, func() (*JSRuntime, error) {
		return NewJSRuntime(m, sessionID, handler)
	})
	if err != nil {
		return nil, err
	}
	rt.setHandler(handler)
	return rt, nil
}

// dropRuntime forgets a session's kernel: variables are gone and the next
// call starts fresh. Owned tabs are kept so reset can still close them.
func (m *Manager) dropRuntime(sessionID string) {
	m.js.Drop(sessionID)
}

// ownTab records a tab as kernel-owned for the session, enforcing the cap.
func (m *Manager) ownTab(sessionID, tabID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	set, ok := m.owned[sessionID]
	if !ok {
		set = map[string]bool{}
		m.owned[sessionID] = set
	}
	if set[tabID] {
		return nil
	}
	if len(set) >= maxOwnedTabs {
		return &jskernel.Error{Msg: errOwnedTabCap}
	}
	set[tabID] = true
	return nil
}

// isOwned reports whether the session's kernel owns the tab.
func (m *Manager) isOwned(sessionID, tabID string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.owned[sessionID][tabID]
}

// forgetTab removes a tab from the owned set (after close or targeted reset).
func (m *Manager) forgetTab(sessionID, tabID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if set, ok := m.owned[sessionID]; ok {
		delete(set, tabID)
		if len(set) == 0 {
			delete(m.owned, sessionID)
		}
	}
}

// ownedTabs lists the session's kernel-owned tab ids.
func (m *Manager) ownedTabs(sessionID string) []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]string, 0, len(m.owned[sessionID]))
	for id := range m.owned[sessionID] {
		out = append(out, id)
	}
	return out
}
