// Per-run account pinning (decision 7 of docs/plan/provider-accounts.md):
// a run resolves each provider's active account once, on its first turn,
// and keeps that pin for the run's lifetime. An account switched mid-run
// therefore affects new runs only, and the pinned credential loads and
// refreshes in its own file — never through the active path.
package run

import (
	"sync"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/accounts"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/providers/claude"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/providers/codex"
)

// accountPins pins one account id per provider for a single run. Pins are
// resolved lazily (some runs never touch a provider the title/smol roles
// would consult) and cached as resolved, including the empty pin for a
// provider with no active account: within one run, "no account" stays
// "no account" rather than adopting a selection made mid-run.
type accountPins struct {
	mu         sync.Mutex
	byProvider map[string]string
}

// forProvider returns the run's pinned account id for providerID. A nil
// receiver (tests, paths that never pinned) resolves nothing.
func (p *accountPins) forProvider(providerID string) string {
	if p == nil {
		return ""
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.byProvider == nil {
		p.byProvider = map[string]string{}
	}
	if id, ok := p.byProvider[providerID]; ok {
		return id
	}
	id := ""
	// While an env token owns the provider, file accounts are not what
	// runs use — the env account is a synthetic active (plan §4.4), so the
	// run resolves to the ordinary loader, exactly as an unpinned one does.
	if !envOwned(providerID) {
		if resolved, err := accounts.EnabledID(providerID); err == nil {
			id = resolved
		}
		// An unconfigured or erroring store resolves empty: the provider's
		// legacy-path behavior.
	}
	p.byProvider[providerID] = id
	return id
}

// envOwned reports whether an environment token currently owns the
// provider's credential resolution (same rule the status API reports).
func envOwned(providerID string) bool {
	switch providerID {
	case "claude":
		return claude.EnvToken() != ""
	case "codex":
		return codex.EnvToken() != ""
	default:
		return false
	}
}
