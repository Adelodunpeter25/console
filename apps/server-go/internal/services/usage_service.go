// Usage quota service. The per-provider quota fetchers live with the
// providers (Phase 3); until then valid providers return null reports with
// the same wire shape (aggregated map, per-provider null).
package services

import (
	"fmt"
)

var usageProviders = []string{"antigravity", "codex", "claude"}

type UsageService struct{}

func NewUsageService() *UsageService { return &UsageService{} }

// IsValidUsageProvider mirrors the TS route check.
func IsValidUsageProvider(provider string) bool {
	for _, p := range usageProviders {
		if p == provider {
			return true
		}
	}
	return false
}

// GetAllUsage returns {provider: report|null} for all quota-backed providers.
func (s *UsageService) GetAllUsage() map[string]any {
	out := make(map[string]any)
	for _, p := range usageProviders {
		out[p] = nil
	}
	return out
}

// GetUsage returns the report for one provider (null until Phase 3 lands).
func (s *UsageService) GetUsage(provider string) (any, error) {
	if !IsValidUsageProvider(provider) {
		return nil, fmt.Errorf("Invalid provider '%s' for usage.", provider)
	}
	return nil, nil
}
