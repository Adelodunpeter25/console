// Canonical proto encoding for usage reports. The provider-fetching layer
// keeps its full internal shapes (notes, metadata, raw provider payloads);
// only this file decides what crosses the wire, trimmed to what the UIs
// render (see proto/console/v1/usage.proto).
package usage

import (
	"encoding/json"

	"google.golang.org/protobuf/encoding/protojson"

	consolev1 "github.com/Adelodunpeter25/console/apps/server-go/internal/gen/console/v1"
)

// ToProtoReport converts a fetched report to the canonical wire type,
// dropping debug-only fields (notes, metadata, raw, resetCredits,
// window duration, unread scope fields). Nil in, nil out.
func ToProtoReport(r *Report) *consolev1.UsageReport {
	if r == nil {
		return nil
	}
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
			if l.Window.ResetLabel != "" {
				window.ResetLabel = &l.Window.ResetLabel
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
		var status *string
		if l.Status != "" {
			status = &l.Status
		}
		out.Limits = append(out.Limits, &consolev1.UsageLimit{
			Id: l.ID, Label: l.Label, Scope: scope,
			Window: window, Amount: amount, Status: status,
		})
	}
	return out
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
