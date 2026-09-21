// Usage quota service. Port of apps/server/api/src/services/usage.service.ts
// (codex slice): 60s cached reports with in-flight dedup and a 5s upstream
// cap. Lives in providers/usage because services cannot import the codex
// provider (codex → loop → services cycle).
package usage

import (
	"context"
	"net/http"
	"sync"
	"time"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/providers/antigravity"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/providers/claude"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/providers/codex"
)

var usageProviders = []string{"antigravity", "codex", "claude"}

const cacheTTL = 60 * time.Second
const upstreamTimeout = 5 * time.Second

type cacheEntry struct {
	report    *Report
	fetchedAt time.Time
}

type call struct {
	done   chan struct{}
	report *Report
}

// Service aggregates per-provider quota reports.
type Service struct {
	mu       sync.Mutex
	cache    map[string]cacheEntry
	inflight map[string]*call
	// BaseURL overrides the Codex endpoint (tests).
	BaseURL string
	// HTTPClient overrides the default client (tests).
	HTTPClient *http.Client
}

func NewService() *Service {
	return &Service{cache: map[string]cacheEntry{}, inflight: map[string]*call{}}
}

// IsValidProvider mirrors the TS route check.
func IsValidProvider(provider string) bool {
	for _, p := range usageProviders {
		if p == provider {
			return true
		}
	}
	return false
}

// GetAllUsage returns {provider: report|null} for quota-backed providers.
func (s *Service) GetAllUsage(ctx context.Context) map[string]any {
	out := make(map[string]any, len(usageProviders))
	var wg sync.WaitGroup
	var mu sync.Mutex
	for _, p := range usageProviders {
		p := p
		wg.Add(1)
		go func() {
			defer wg.Done()
			report, _ := s.GetUsage(ctx, p)
			mu.Lock()
			out[p] = report
			mu.Unlock()
		}()
	}
	wg.Wait()
	return out
}

// GetUsage returns the cached or freshly fetched report (nil when logged
// out, unsupported, or upstream fails). Unknown providers return an error.
func (s *Service) GetUsage(ctx context.Context, provider string) (*Report, error) {
	if !IsValidProvider(provider) {
		return nil, &invalidProviderError{provider}
	}
	if provider != "codex" && provider != "claude" && provider != "antigravity" {
		return nil, nil
	}
	s.mu.Lock()
	if entry, ok := s.cache[provider]; ok && time.Since(entry.fetchedAt) < cacheTTL {
		report := entry.report
		s.mu.Unlock()
		return report, nil
	}
	if in, ok := s.inflight[provider]; ok {
		s.mu.Unlock()
		select {
		case <-in.done:
			return in.report, nil
		case <-ctx.Done():
			return nil, nil
		}
	}
	c := &call{done: make(chan struct{})}
	s.inflight[provider] = c
	s.mu.Unlock()

	switch provider {
	case "claude":
		c.report = s.fetchClaude(ctx)
	case "antigravity":
		c.report = s.fetchAntigravity(ctx)
	default:
		c.report = s.fetchCodex(ctx)
	}
	s.mu.Lock()
	s.cache[provider] = cacheEntry{report: c.report, fetchedAt: time.Now()}
	delete(s.inflight, provider)
	s.mu.Unlock()
	close(c.done)
	return c.report, nil
}

func (s *Service) fetchCodex(ctx context.Context) *Report {
	cred, err := codex.LoadCredential()
	if err != nil {
		return nil
	}
	cred, err = codex.RefreshIfNeeded(nil, cred)
	if err != nil {
		return nil
	}
	base := codex.BaseURL()
	if s.BaseURL != "" {
		base = s.BaseURL
	}
	timeoutCtx, cancel := context.WithTimeout(ctx, upstreamTimeout)
	defer cancel()
	report, _ := FetchCodexUsage(timeoutCtx, nil, base, cred.AccessToken, cred.AccountID, cred.Email, cred.ExpiresAtMs)
	return report
}

func (s *Service) fetchClaude(ctx context.Context) *Report {
	cred, err := claude.LoadCredential()
	if err != nil {
		return nil
	}
	cred, err = claude.RefreshIfNeeded(nil, cred)
	if err != nil {
		return nil
	}
	base := claude.BaseURL()
	if s.BaseURL != "" {
		base = s.BaseURL
	}
	timeoutCtx, cancel := context.WithTimeout(ctx, upstreamTimeout)
	defer cancel()
	report, _ := FetchClaudeUsage(timeoutCtx, nil, base, cred.AccessToken, cred.Email, cred.ExpiresAtMs)
	return report
}

func (s *Service) fetchAntigravity(ctx context.Context) *Report {
	cred, err := antigravity.LoadCredential()
	if err != nil {
		return nil
	}
	cred, err = antigravity.RefreshIfNeeded(nil, cred)
	if err != nil {
		return nil
	}
	base := ""
	if s.BaseURL != "" {
		base = s.BaseURL
	}
	timeoutCtx, cancel := context.WithTimeout(ctx, upstreamTimeout)
	defer cancel()
	report, _ := FetchAntigravityUsage(timeoutCtx, nil, base, cred.AccessToken, cred.ProjectID, "", cred.Email, cred.ExpiresAtMs)
	return report
}

// Invalidate drops cached reports (all or one provider).
func (s *Service) Invalidate(provider ...string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(provider) == 0 {
		s.cache = map[string]cacheEntry{}
		return
	}
	for _, p := range provider {
		delete(s.cache, p)
	}
}

type invalidProviderError struct{ provider string }

func (e *invalidProviderError) Error() string {
	return "Invalid provider '" + e.provider + "' for usage."
}
