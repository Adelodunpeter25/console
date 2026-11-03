// Usage quota service: 60s cached reports with in-flight dedup and a 5s upstream
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
	"github.com/Adelodunpeter25/console/apps/server-go/internal/providers/opencode"
)

var usageProviders = []string{"antigravity", "codex", "claude", "opencode"}

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
	// Last known reports, served while a background refresh re-reads the
	// freshly activated account's quota (stale-while-revalidate).
	stale map[string]*Report
	// BaseURL overrides the Codex endpoint (tests).
	BaseURL string
	// HTTPClient overrides the default client (tests).
	HTTPClient *http.Client
}

func NewService() *Service {
	return &Service{cache: map[string]cacheEntry{}, inflight: map[string]*call{}, stale: map[string]*Report{}}
}

// IsValidProvider reports whether provider is a supported usage provider.
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
// out, unsupported, or upstream fails). After MarkStale (account switch) it
// returns the previous account's report immediately and refreshes in the
// background. Unknown providers return an error.
func (s *Service) GetUsage(ctx context.Context, provider string) (*Report, error) {
	if !IsValidProvider(provider) {
		return nil, &invalidProviderError{provider}
	}
	if provider != "codex" && provider != "claude" && provider != "antigravity" && provider != "opencode" {
		return nil, nil
	}
	s.mu.Lock()
	if entry, ok := s.cache[provider]; ok && time.Since(entry.fetchedAt) < cacheTTL {
		report := entry.report
		s.mu.Unlock()
		return report, nil
	}
	if stale, ok := s.stale[provider]; ok {
		s.mu.Unlock()
		s.refreshAsync(provider)
		return stale, nil
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
	s.mu.Unlock()
	return s.refreshSync(ctx, provider), nil
}

// fetch dispatches one upstream report fetch.
func (s *Service) fetch(ctx context.Context, provider string) *Report {
	switch provider {
	case "claude":
		return s.fetchClaude(ctx)
	case "antigravity":
		return s.fetchAntigravity(ctx)
	case "opencode":
		return s.fetchOpenCode(ctx)
	default:
		return s.fetchCodex(ctx)
	}
}

// store records a fetch outcome and clears the stale fallback on success.
func (s *Service) store(provider string, report *Report) {
	s.mu.Lock()
	s.cache[provider] = cacheEntry{report: report, fetchedAt: time.Now()}
	delete(s.inflight, provider)
	if report != nil {
		delete(s.stale, provider)
	}
	s.mu.Unlock()
}

// refreshSync fetches inline under the shared in-flight dedup.
func (s *Service) refreshSync(ctx context.Context, provider string) *Report {
	s.mu.Lock()
	if in, ok := s.inflight[provider]; ok {
		s.mu.Unlock()
		select {
		case <-in.done:
			return in.report
		case <-ctx.Done():
			return nil
		}
	}
	c := &call{done: make(chan struct{})}
	s.inflight[provider] = c
	s.mu.Unlock()

	c.report = s.fetch(ctx, provider)
	s.store(provider, c.report)
	close(c.done)
	return c.report
}

// refreshAsync re-fetches off the request path (single-flight), replacing
// the stale report when it lands. A failed fetch keeps the stale report
// served; a logged-out fetch stores nil, ending the fallback.
func (s *Service) refreshAsync(provider string) {
	s.mu.Lock()
	if _, ok := s.inflight[provider]; ok {
		s.mu.Unlock()
		return
	}
	c := &call{done: make(chan struct{})}
	s.inflight[provider] = c
	s.mu.Unlock()

	go func() {
		c.report = s.fetch(context.Background(), provider)
		s.store(provider, c.report)
		close(c.done)
	}()
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

// fetchOpenCode reads the Go allowance windows with the provider's API key.
//
// The base URL and key both come from the opencode provider package, so the
// env overrides (OPENCODE_BASE_URL, OPENCODE_API_KEY) and the stored-key
// lookup work exactly as they do for inference. There is no OAuth refresh
// here: a Go key is pasted once and never renewed by this server.
func (s *Service) fetchOpenCode(ctx context.Context) *Report {
	key, err := opencode.APIKey()
	if err != nil {
		return nil
	}
	timeoutCtx, cancel := context.WithTimeout(ctx, upstreamTimeout)
	defer cancel()
	report, _ := FetchOpenCodeUsage(timeoutCtx, s.HTTPClient, opencode.BaseURL(), key)
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
		s.stale = map[string]*Report{}
		return
	}
	for _, p := range provider {
		delete(s.cache, p)
		delete(s.stale, p)
	}
}

// MarkStale keeps the provider's last report for stale-while-revalidate
// serving after an account switch: the next read returns the old report
// immediately while a background fetch re-reads the new account's quota.
func (s *Service) MarkStale(provider string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if entry, ok := s.cache[provider]; ok {
		s.stale[provider] = entry.report
		delete(s.cache, provider)
	}
}

type invalidProviderError struct{ provider string }

func (e *invalidProviderError) Error() string {
	return "Invalid provider '" + e.provider + "' for usage."
}
