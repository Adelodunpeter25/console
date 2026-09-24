package bench

import (
	"context"
	"fmt"
	"time"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/providers/usage"
)

// UsageSource fetches a fresh subscription usage report.
type UsageSource interface {
	Fetch(ctx context.Context) (*usage.Report, error)
}

// ServiceSource adapts usage.Service, invalidating its cache per fetch so
// every check sees current numbers.
type ServiceSource struct {
	Service  *usage.Service
	Provider string
}

func (s ServiceSource) Fetch(ctx context.Context) (*usage.Report, error) {
	s.Service.Invalidate(s.Provider)
	return s.Service.GetUsage(ctx, s.Provider)
}

// Budget refuses to start or continue once any percent limit reaches
// MaxPct (Task 1.7).
type Budget struct {
	Source UsageSource
	MaxPct float64
	// RetryDelay separates the retry after an empty report (default 3s).
	RetryDelay time.Duration
	// MinInterval spaces real fetches (default 3m): the usage endpoint
	// rate-limits frequent polling, so checks in between reuse the last
	// good report.
	MinInterval time.Duration
	// MaxStale is how long the last good report may stand in for a failed
	// fetch (default 15m).
	MaxStale time.Duration

	last   *usage.Report
	lastAt time.Time
}

func (b *Budget) retryDelay() time.Duration {
	if b.RetryDelay > 0 {
		return b.RetryDelay
	}
	return 3 * time.Second
}

func (b *Budget) minInterval() time.Duration {
	if b.MinInterval > 0 {
		return b.MinInterval
	}
	return 3 * time.Minute
}

func (b *Budget) maxStale() time.Duration {
	if b.MaxStale > 0 {
		return b.MaxStale
	}
	return 15 * time.Minute
}

// Headroom returns the highest used percent across limits and an error
// when it is at or above MaxPct.
func (b *Budget) Headroom(ctx context.Context) (float64, error) {
	if b == nil || b.Source == nil {
		return 0, nil
	}
	report, err := b.fetch(ctx)
	if err != nil {
		return 0, err
	}
	highest, label := 0.0, ""
	for _, limit := range report.Limits {
		if limit.Amount.Unit != "percent" || limit.Amount.Used == nil {
			continue
		}
		if *limit.Amount.Used > highest {
			highest, label = *limit.Amount.Used, limit.Label
		}
	}
	if highest >= b.MaxPct {
		return highest, fmt.Errorf("usage limit %q at %.0f%% (max %.0f%%)", label, highest, b.MaxPct)
	}
	return highest, nil
}

// fetch returns a usage report, reusing the last good one within
// MinInterval. The usage service reports failures (e.g. a 429) as a nil
// report: retry once, then fall back to the last good report while it is
// younger than MaxStale, else stop rather than run blind.
func (b *Budget) fetch(ctx context.Context) (*usage.Report, error) {
	if b.last != nil && time.Since(b.lastAt) < b.minInterval() {
		return b.last, nil
	}
	report, err := b.Source.Fetch(ctx)
	if err == nil && report == nil {
		select {
		case <-time.After(b.retryDelay()):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
		report, err = b.Source.Fetch(ctx)
	}
	if err == nil && report != nil {
		b.last, b.lastAt = report, time.Now()
		return report, nil
	}
	if b.last != nil && time.Since(b.lastAt) < b.maxStale() {
		return b.last, nil
	}
	if err != nil {
		return nil, fmt.Errorf("usage check failed: %w", err)
	}
	return nil, fmt.Errorf("usage check failed: no usage report (the usage endpoint may be rate-limiting; retry in a few minutes or pass --max-usage-pct 0)")
}
