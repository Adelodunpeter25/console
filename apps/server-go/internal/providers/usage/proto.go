// Canonical proto encoding for usage reports. The provider-fetching layer
// keeps its full internal shapes (notes, metadata, raw provider payloads);
// only this file decides what crosses the wire, trimmed to what the UIs
// render (see proto/console/v1/usage.proto).
package usage

import (
	"encoding/json"
	"fmt"
	"time"

	"google.golang.org/protobuf/encoding/protojson"

	consolev1 "github.com/Adelodunpeter25/console/apps/server-go/internal/gen/console/v1"
)

// ToProtoReport converts a fetched report to the canonical wire type,
// dropping debug-only fields (notes, metadata, raw, resetCredits,
// window duration, unread scope fields). Nil in, nil out.
//
// It is also the single place presentation math is derived so every client
// renders identically (clients are dumb renderers of these fields):
//   - Limit.Status: always set — ok/warning/exhausted from the used
//     fraction, "unknown" when no usage data. Never empty.
//   - Amount.UsedPercent: used_fraction clamped to 0..100.
//   - Window.ResetLabel: preformatted "resets in …" countdown when the
//     fetcher did not supply one.
func ToProtoReport(r *Report) *consolev1.UsageReport {
	if r == nil {
		return nil
	}
	nowMs := time.Now().UnixMilli()
	out := &consolev1.UsageReport{Provider: r.Provider, FetchedAt: r.FetchedAt}
	for _, l := range r.Limits {
		scope := &consolev1.UsageScope{Provider: l.Scope.Provider}
		if l.Scope.Tier != "" {
			scope.Tier = &l.Scope.Tier
		}
		if l.Scope.ModelID != "" {
			scope.ModelId = &l.Scope.ModelID
		}
		var window *consolev1.UsageWindow
		if l.Window != nil {
			window = &consolev1.UsageWindow{Id: l.Window.ID, Label: l.Window.Label}
			if l.Window.ResetsAt != nil {
				window.ResetsAt = l.Window.ResetsAt
			}
			label := l.Window.ResetLabel
			if label == "" && l.Window.ResetsAt != nil {
				label = ResetCountdownLabel(*l.Window.ResetsAt, nowMs)
			}
			if label != "" {
				window.ResetLabel = &label
			}
		}
		amount := &consolev1.UsageAmount{Unit: l.Amount.Unit}
		if l.Amount.Used != nil {
			amount.Used = l.Amount.Used
		}
		if l.Amount.Limit != nil {
			amount.Limit = l.Amount.Limit
		}
		if l.Amount.Remaining != nil {
			amount.Remaining = l.Amount.Remaining
		}
		if l.Amount.UsedFraction != nil {
			amount.UsedFraction = l.Amount.UsedFraction
		}
		if l.Amount.RemainingFraction != nil {
			amount.RemainingFraction = l.Amount.RemainingFraction
		}
		if usedPercent, ok := l.Amount.UsedPercent(); ok {
			amount.UsedPercent = &usedPercent
		}
		status := l.Status
		if status == "" {
			status = StatusForFraction(l.Amount.UsedFraction, nil, nil)
		}
		out.Limits = append(out.Limits, &consolev1.UsageLimit{
			Id: l.ID, Label: l.Label, Scope: scope,
			Window: window, Amount: amount, Status: &status,
		})
	}
	return out
}

// ResetCountdownLabel renders "resets in 3h 20m" (or "resets momentarily"
// once past) from an epoch-ms reset time. Desktop's usage page and Android's
// UsageHelpers previously duplicated this; the label is computed once here
// and clients render it verbatim.
func ResetCountdownLabel(resetsAtMs, nowMs int64) string {
	diffSecs := (resetsAtMs - nowMs) / 1000
	switch {
	case diffSecs <= 0:
		return "resets momentarily"
	case diffSecs < 60:
		return fmt.Sprintf("resets in %ds", diffSecs)
	case diffSecs < 3600:
		return fmt.Sprintf("resets in %dm", diffSecs/60)
	case diffSecs < 86400:
		hours, mins := diffSecs/3600, (diffSecs%3600)/60
		if mins > 0 {
			return fmt.Sprintf("resets in %dh %dm", hours, mins)
		}
		return fmt.Sprintf("resets in %dh", hours)
	default:
		days, hours := diffSecs/86400, (diffSecs%86400)/3600
		if hours > 0 {
			return fmt.Sprintf("resets in %dd %dh", days, hours)
		}
		return fmt.Sprintf("resets in %dd", days)
	}
}

// EncodeReportValue encodes one GetAllUsage map entry (or single-report
// response): nil stays JSON null (logged-out provider), anything else is
// the trimmed protojson report. It takes any because Report is unexported
// outside this package; values that are not reports also encode as null.
func EncodeReportValue(v any) (json.RawMessage, error) {
	r, ok := v.(*Report)
	if !ok || r == nil {
		return json.RawMessage("null"), nil
	}
	return protojson.Marshal(ToProtoReport(r))
}
