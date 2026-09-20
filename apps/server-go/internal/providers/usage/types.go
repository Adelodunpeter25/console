// Usage report types. Mirrors packages/types/src/usage.ts JSON shapes so
// /api/usage responses match the TS server wire format.
package usage

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
