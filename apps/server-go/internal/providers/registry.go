// Provider registry: id → streaming backend. Port of the provider lookup
// in apps/server/agent/src/commands/provider-registry.ts. Implementations
// plug in here without run-package changes.
package providers

import (
	"fmt"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/loop"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/providers/antigravity"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/providers/claude"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/providers/codex"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/providers/opencode"
)

// Lookup returns the streaming backend for a provider id.
func Lookup(id string) (loop.Provider, error) {
	switch id {
	case "codex":
		return &codex.Provider{}, nil
	case "claude":
		return &claude.Provider{}, nil
	case "antigravity":
		return &antigravity.Provider{}, nil
	case "opencode":
		return &opencode.Provider{}, nil
	default:
		return nil, fmt.Errorf("Unknown provider '%s'.", id)
	}
}
