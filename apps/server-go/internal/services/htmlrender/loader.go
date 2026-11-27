package htmlrender

import (
	"sync"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/tools"
)

// Loader brings render and preview into a run when /render is active.
type Loader struct {
	opts    Options
	browser tools.BrowserHandler
	mu      sync.Mutex
	loaded  bool
}

// NewLoader returns a loader bound to one session's workspace and browser bridge.
func NewLoader(opts Options, browser tools.BrowserHandler) *Loader {
	return &Loader{opts: opts, browser: browser}
}

// Loaded reports whether the tools have been brought into some run.
func (l *Loader) Loaded() bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.loaded
}

// Load appends render and preview to registry and marks the group loaded.
func (l *Loader) Load(registry *tools.Registry) (int, error) {
	registry.Add(NewRenderTool(l.opts))
	registry.Add(NewPreviewTool(l.opts, l.browser))
	l.mu.Lock()
	l.loaded = true
	l.mu.Unlock()
	return 2, nil
}
