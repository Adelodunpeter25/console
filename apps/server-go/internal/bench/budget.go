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
}

func (b *Budget) retryDelay() time.Duration {
	if b.RetryDelay > 0 {
		return b.RetryDelay
	}
	return 3 * time.Second
}

// Headroom returns the highest used percent across limits and an error
// when it is at or above MaxPct.
func (b *Budget) Headroom(ctx context.Context) (float64, error) {
	if b == nil || b.Source == nil {
		return 0, nil
	}
	// The usage service reports fetch failures as a nil report; retry
	// once, then stop rather than run blind.
	report, err := b.Source.Fetch(ctx)
	if err == nil && report == nil {
		select {
		case <-time.After(b.retryDelay()):
		case <-ctx.Done():
			return 0, ctx.Err()
		}
		report, err = b.Source.Fetch(ctx)
	}
	if err != nil {
		return 0, fmt.Errorf("usage check failed: %w", err)
	}
	if report == nil {
		return 0, fmt.Errorf("usage check failed: no usage report")
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
