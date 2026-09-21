// Provider registry: id → streaming backend. Port of the provider lookup
// in apps/server/agent/src/commands/provider-registry.ts (initial slice:
// codex only; claude/antigravity/devin plug in here as their Go ports
// land, without run-package changes).
package providers

import (
	"fmt"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/loop"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/providers/claude"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/providers/codex"
)

// Lookup returns the streaming backend for a provider id.
func Lookup(id string) (loop.Provider, error) {
	switch id {
	case "codex":
		return &codex.Provider{}, nil
	case "claude":
		return &claude.Provider{}, nil
	default:
		return nil, fmt.Errorf("Unknown provider '%s'.", id)
	}
}
