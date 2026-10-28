// Usage report types: the JSON shapes returned by /api/usage.
package usage

import "math"

type Window struct {
	ID         string `json:"id"`
	Label      string `json:"label"`
	DurationMs *int64 `json:"durationMs,omitempty"`
	ResetsAt   *int64 `json:"resetsAt,omitempty"`
	ResetLabel string `json:"resetLabel,omitempty"`
}

type Amount struct {
	Used              *float64 `json:"used,omitempty"`
	Limit             *float64 `json:"limit,omitempty"`
	Remaining         *float64 `json:"remaining,omitempty"`
	UsedFraction      *float64 `json:"usedFraction,omitempty"`
	RemainingFraction *float64 `json:"remainingFraction,omitempty"`
	Unit              string   `json:"unit"`
}

// UsedPercent resolves the bar percentage (0..100) from the same fallback
// chain the clients used to run: used_fraction, used/limit, percent-unit
// used, 1-remaining_fraction. Ok=false when the amount carries no usage
// data.
func (a Amount) UsedPercent() (float64, bool) {
	if a.UsedFraction != nil {
		return clampPercent(*a.UsedFraction * 100), true
	}
	if a.Used != nil && a.Limit != nil && *a.Limit > 0 {
		return clampPercent(*a.Used / *a.Limit * 100), true
	}
	if a.Unit == "percent" && a.Used != nil {
		return clampPercent(*a.Used), true
	}
	if a.RemainingFraction != nil {
		return clampPercent((1 - *a.RemainingFraction) * 100), true
	}
	return 0, false
}

func clampPercent(v float64) float64 { return math.Min(math.Max(v, 0), 100) }

type Scope struct {
	Provider  string `json:"provider"`
	AccountID string `json:"accountId,omitempty"`
	ProjectID string `json:"projectId,omitempty"`
	OrgID     string `json:"orgId,omitempty"`
	ModelID   string `json:"modelId,omitempty"`
	Tier      string `json:"tier,omitempty"`
	WindowID  string `json:"windowId,omitempty"`
	Shared    bool   `json:"shared,omitempty"`
}

type Limit struct {
	ID     string   `json:"id"`
	Label  string   `json:"label"`
	Scope  Scope    `json:"scope"`
	Window *Window  `json:"window,omitempty"`
	Amount Amount   `json:"amount"`
	Status string   `json:"status,omitempty"`
	Notes  []string `json:"notes,omitempty"`
}

type ResetCredits struct {
	AvailableCount int `json:"availableCount"`
}

type Report struct {
	Provider     string         `json:"provider"`
	FetchedAt    int64          `json:"fetchedAt"`
	Limits       []Limit        `json:"limits"`
	ResetCredits *ResetCredits  `json:"resetCredits,omitempty"`
	Notes        []string       `json:"notes,omitempty"`
	Metadata     map[string]any `json:"metadata,omitempty"`
	Raw          any            `json:"raw,omitempty"`
}
