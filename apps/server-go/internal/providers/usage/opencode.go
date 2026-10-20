// OpenCode Go quota fetcher: GET {base}/usage with the Go API key, parsed
// into a UsageReport with the three allowance windows Go meters.
//
// Go meters the included usage of a subscription in three windows: a rolling
// 5-hour window (20% of the monthly allowance), a weekly one (50%) and the
// monthly one (100%). The endpoint reports how much of each is used as a
// percentage, and when the window resets.
//
// The endpoint is not part of the published Go documentation, so the parser
// stays defensive: a window it cannot read is dropped rather than rendered as
// a zero-percent window, and an unreadable payload yields no report at all.
package usage

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/providers/opencode"
)

const openCodeUsagePath = "usage"

// openCodeWindow is one of the allowance windows Go reports.
type openCodeWindow struct {
	// key is the payload field name.
	key string
	// id and label describe the window in the report.
	id    string
	label string
}

// openCodeWindows are the windows Go meters, in the order they are shown.
//
// The rolling window is the 5-hour one: Go's docs define the 5-hour window as
// 20% of the monthly allowance, and the resetsAt it reports lines up with a
// five-hour roll rather than a fixed clock hour.
var openCodeWindows = []openCodeWindow{
	{key: "rolling", id: "5h", label: "5 hours"},
	{key: "weekly", id: "weekly", label: "Weekly"},
	{key: "monthly", id: "monthly", label: "Monthly"},
}

// OpenCodeUsageURL appends the usage path to a Go inference base URL.
func OpenCodeUsageURL(baseURL string) string {
	if strings.TrimSpace(baseURL) == "" {
		baseURL = opencode.DefaultBaseURL
	}
	return strings.TrimRight(baseURL, "/") + "/" + openCodeUsagePath
}

// openCodeStatus maps Go's own status word onto the report status vocabulary.
// An unknown word falls back to the shared threshold mapping, so a new
// upstream status cannot leave a window unclassified.
func openCodeStatus(upstream string, usedFraction *float64) string {
	switch upstream {
	case "ok", "warning", "exhausted", "unknown":
		return upstream
	}
	return StatusForFraction(usedFraction, nil, nil)
}

// parseOpenCodeResetsAt reads a reset timestamp. Go sends RFC 3339 strings;
// a numeric epoch is accepted too so a shape change does not lose the reset.
func parseOpenCodeResetsAt(value any) *int64 {
	switch v := value.(type) {
	case string:
		trimmed := strings.TrimSpace(v)
		if trimmed == "" {
			return nil
		}
		ts, err := time.Parse(time.RFC3339, trimmed)
		if err != nil {
			return nil
		}
		ms := ts.UnixMilli()
		return &ms
	case float64:
		ms := int64(v)
		if ms <= 1_000_000_000_000 {
			ms *= 1000
		}
		return &ms
	}
	return nil
}

// ParseOpenCodePayload converts a Go usage payload to a Report. Returns nil
// when the payload carries no readable window.
func ParseOpenCodePayload(payload any, nowMs int64) *Report {
	root, ok := Record(payload)
	if !ok {
		return nil
	}
	usageBlock, ok := Record(root["usage"])
	if !ok {
		return nil
	}

	var limits []Limit
	for _, window := range openCodeWindows {
		block, ok := Record(usageBlock[window.key])
		if !ok {
			continue
		}
		percent, hasPercent := Number(block["percent"])
		if !hasPercent {
			continue
		}
		amount := BuildPercentAmount(&percent)
		status := ""
		if s, ok := block["status"].(string); ok {
			status = s
		}
		reportWindow := Window{ID: window.id, Label: window.label}
		if resetsAt := parseOpenCodeResetsAt(block["resetsAt"]); resetsAt != nil {
			reportWindow.ResetsAt = resetsAt
		}
		limits = append(limits, Limit{
			ID: "opencode:" + window.id, Label: window.label,
			Scope:  Scope{Provider: "opencode", WindowID: window.id},
			Window: &reportWindow, Amount: amount,
			Status: openCodeStatus(status, amount.UsedFraction),
		})
	}
	if len(limits) == 0 {
		return nil
	}
	return &Report{Provider: "opencode", FetchedAt: nowMs, Limits: limits, Raw: payload}
}

// FetchOpenCodeUsage GETs the Go usage endpoint and parses the report.
// Returns (nil, nil) when no key is configured or the upstream call fails —
// Go has no anonymous tier, so an unconfigured server has no usage to show.
func FetchOpenCodeUsage(ctx context.Context, client *http.Client, baseURL, apiKey string) (*Report, error) {
	if strings.TrimSpace(apiKey) == "" {
		return nil, nil
	}
	nowMs := time.Now().UnixMilli()
	if client == nil {
		client = &http.Client{Timeout: 5 * time.Second}
	}
	endpoint := OpenCodeUsageURL(baseURL)
	req, err := http.NewRequestWithContext(ctx, "GET", endpoint, nil)
	if err != nil {
		return nil, nil
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("User-Agent", opencode.UserAgent())
	resp, err := client.Do(req)
	if err != nil {
		return nil, nil
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, nil
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, nil
	}
	var payload any
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, nil
	}
	return ParseOpenCodePayload(payload, nowMs), nil
}
