// Shared quota-report builders for provider usage fetchers (codex first,
// claude/antigravity next). The math — window labels, percent amounts,
// ok/warning/exhausted thresholds — mirrors the TS usage providers so
// reports stay wire-compatible across backends.
package usage

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"
)

// Number extracts a finite float64 from JSON-ish values (numbers and
// numeric strings, mirroring the TS toNumber helper).
func Number(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		if math.IsNaN(n) || math.IsInf(n, 0) {
			return 0, false
		}
		return n, true
	case float32:
		return float64(n), true
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	case json.Number:
		f, err := n.Float64()
		return f, err == nil
	case string:
		s := strings.TrimSpace(n)
		if s == "" {
			return 0, false
		}
		var f float64
		if _, err := fmt.Sscanf(s, "%g", &f); err != nil {
			return 0, false
		}
		if math.IsNaN(f) || math.IsInf(f, 0) {
			return 0, false
		}
		return f, true
	default:
		return 0, false
	}
}

// Bool extracts a bool, reporting whether the value was present.
func Bool(v any) (*bool, bool) {
	b, ok := v.(bool)
	if !ok {
		return nil, false
	}
	return &b, true
}

// Record asserts a JSON object.
func Record(v any) (map[string]any, bool) {
	m, ok := v.(map[string]any)
	return m, ok
}

// WindowTimes are the raw numeric fields of a quota window.
type WindowTimes struct {
	LimitWindowSeconds *float64
	ResetAt            *float64
	ResetAfterSeconds  *float64
}

// WindowLabel renders a duration id + label ("5h", "5 hours").
func WindowLabel(seconds float64) (id, label string) {
	const daySeconds = 86400.0
	if seconds >= daySeconds {
		days := int(math.Round(seconds / daySeconds))
		unit := "days"
		if days == 1 {
			unit = "day"
		}
		return fmt.Sprintf("%dd", days), fmt.Sprintf("%d %s", days, unit)
	}
	hours := int(math.Max(1, math.Round(seconds/3600)))
	unit := "hours"
	if hours == 1 {
		unit = "hour"
	}
	return fmt.Sprintf("%dh", hours), fmt.Sprintf("%d %s", hours, unit)
}

// ResolveResetMs converts reset_at (s or ms) or reset_after_seconds to an
// epoch-ms timestamp.
func ResolveResetMs(t WindowTimes, nowMs int64) *int64 {
	if t.ResetAt != nil {
		ms := *t.ResetAt
		if ms <= 1_000_000_000_000 {
			ms *= 1000
		}
		if !math.IsNaN(ms) && !math.IsInf(ms, 0) {
			out := int64(ms)
			return &out
		}
	}
	if t.ResetAfterSeconds != nil {
		out := nowMs + int64(*t.ResetAfterSeconds*1000)
		return &out
	}
	return nil
}

// BuildWindow renders a Window for key ("primary"/"secondary"), falling
// back to "<Key> window" labels without duration data.
func BuildWindow(key string, t WindowTimes, nowMs int64) Window {
	out := Window{ID: key}
	if t.LimitWindowSeconds != nil {
		id, label := WindowLabel(*t.LimitWindowSeconds)
		out.ID, out.Label = id, label
		dur := int64(*t.LimitWindowSeconds * 1000)
		out.DurationMs = &dur
	} else if key == "primary" {
		out.Label = "Primary window"
	} else {
		out.Label = "Secondary window"
	}
	out.ResetsAt = ResolveResetMs(t, nowMs)
	return out
}

// BuildPercentAmount renders a 0-100 percent Amount (unit-only when the
// backend reports no used percent).
func BuildPercentAmount(usedPercent *float64) Amount {
	out := Amount{Unit: "percent"}
	if usedPercent == nil {
		return out
	}
	clamped := math.Min(math.Max(*usedPercent, 0), 100)
	frac := clamped / 100
	rem := math.Max(0, 100-clamped)
	remFrac := math.Max(0, 1-frac)
	out.Used, out.Limit, out.Remaining = &clamped, ptrFloat(100), &rem
	out.UsedFraction, out.RemainingFraction = &frac, &remFrac
	return out
}

// StatusForFraction maps a used fraction to ok/warning/exhausted/unknown.
// explicitlyAllowed mirrors the TS buildUsageStatus rule: allowed===true
// AND limitReached===false (absent is not false).
func StatusForFraction(usedFraction *float64, allowed, limitReached *bool) string {
	if usedFraction == nil {
		return "unknown"
	}
	f := *usedFraction
	explicit := allowed != nil && *allowed && limitReached != nil && !*limitReached
	if f >= 1 {
		if explicit {
			return "warning"
		}
		return "exhausted"
	}
	if f >= 0.5 {
		return "warning"
	}
	return "ok"
}

func ptrFloat(v float64) *float64 { return &v }
