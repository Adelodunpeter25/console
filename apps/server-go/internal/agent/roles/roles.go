// Model roles and thinking-level validation. Port of
// apps/server/agent/src/service/model-roles.ts (resolution),
// role-resolver.ts (role entry point), and validate-thinking.ts.
// Configured refs come from settings as "provider/model" or bare ids;
// unknown ids synthesize a fallback entry like the TS resolver.
package roles

import (
	"fmt"
	"strings"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/providers"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/types"
)

// Roles known to settings (mirrors ConsoleModelRole).
const (
	Vision = "vision"
	Smol   = "smol"
)

// IsRole reports whether a name is a known model role.
func IsRole(value string) bool {
	return value == Vision || value == Smol
}

// HasConfiguredRole reports whether settings map a role to a model.
func HasConfiguredRole(roles map[string]string, role string) bool {
	return strings.TrimSpace(roles[role]) != ""
}

// ParseReference splits "provider/model" or bare ids (same provider as
// fallback), mirroring parseModelReference.
func ParseReference(reference, fallbackProvider, fallbackModelID string) (provider, modelID string) {
	if i := strings.Index(reference, "/"); i > 0 {
		return reference[:i], reference[i+1:]
	}
	return fallbackProvider, reference
}

// InferThinkingLevels gives best-effort level support for models missing
// from the catalog, mirroring inferThinkingLevels.
func InferThinkingLevels(provider, modelID string) (levels []string, def string) {
	switch provider {
	case "codex":
		return []string{"none", "minimal", "low", "medium", "high", "xhigh", "max"}, "low"
	case "claude":
		return []string{"low", "medium", "high", "xhigh", "max"}, "low"
	case "antigravity":
		if strings.HasPrefix(modelID, "gemini-") || strings.HasPrefix(modelID, "claude-") {
			return []string{"minimal", "low", "medium", "high"}, "low"
		}
	}
	return nil, ""
}

// ResolveRoleModel maps a configured role to a model, falling back to the
// active model. Unknown ids synthesize a 128k entry with inferred levels;
// unknown providers fall back entirely (TS parity).
func ResolveRoleModel(role string, fallback types.Model, configuredRef string) types.Model {
	reference := strings.TrimSpace(configuredRef)
	if reference == "" {
		return fallback
	}
	providerID, modelID := ParseReference(reference, fallback.Provider, fallback.ID)
	if !providers.IsCatalogProvider(providerID) {
		return fallback
	}
	if found, ok := providers.FindModel(providerID, modelID); ok {
		return found
	}
	levels, def := InferThinkingLevels(providerID, modelID)
	return types.Model{
		ID: modelID, Provider: providerID, ContextWindow: 128_000,
		ThinkingLevels: levels, DefaultThinking: def,
	}
}

// ValidateLevel rejects thinking levels a model does not support.
// Omitted levels are always valid (model default applies).
func ValidateLevel(model types.Model, level string) error {
	if level == "" {
		return nil
	}
	if len(model.ThinkingLevels) == 0 {
		return fmt.Errorf("Model \"%s\" does not support thinking levels. "+
			"Only providers with built-in reasoning (Codex, Claude, Gemini) support this feature.", model.ID)
	}
	for _, supported := range model.ThinkingLevels {
		if supported == level {
			return nil
		}
	}
	return fmt.Errorf("Model \"%s\" does not support thinking level \"%s\". Supported levels: %s.",
		model.ID, level, strings.Join(model.ThinkingLevels, ", "))
}
