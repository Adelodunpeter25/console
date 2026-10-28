// The wire derivation in ToProtoReport: status is always set,
// used_percent is the clamped bar percentage, and reset_label is the
// preformatted countdown. Clients render these verbatim.
package tests

import (
	"testing"
	"time"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/providers/usage"
)

func TestToProtoReportDerivesStatusAndPercent(t *testing.T) {
	frac := 0.42
	report := &usage.Report{
		Provider:  "claude",
		FetchedAt: time.Now().UnixMilli(),
		Limits: []usage.Limit{{
			ID:    "session",
			Label: "Session",
			Amount: usage.Amount{
				UsedFraction: &frac,
				Unit:         "percent",
			},
		}},
	}
	proto := usage.ToProtoReport(report)
	if proto == nil || len(proto.Limits) != 1 {
		t.Fatalf("proto: %+v", proto)
	}
	limit := proto.Limits[0]
	if limit.Status == nil || *limit.Status != "ok" {
		t.Fatalf("status: %v", limit.Status)
	}
	if limit.Amount.UsedPercent == nil || *limit.Amount.UsedPercent != 42.0 {
		t.Fatalf("used_percent: %v", limit.Amount.UsedPercent)
	}
}

func TestToProtoReportStatusAlwaysSet(t *testing.T) {
	report := &usage.Report{
		Provider: "claude",
		Limits:   []usage.Limit{{ID: "x", Label: "X"}},
	}
	proto := usage.ToProtoReport(report)
	limit := proto.Limits[0]
	if limit.Status == nil || *limit.Status != "unknown" {
		t.Fatalf("empty status not derived: %v", limit.Status)
	}
	if limit.Amount.UsedPercent != nil {
		t.Fatalf("used_percent without usage data: %v", limit.Amount.UsedPercent)
	}
}

func TestToProtoReportPercentFallbackChain(t *testing.T) {
	// used/limit fallback.
	pct := 80.0
	cap := 200.0
	chain := []struct {
		name   string
		amount usage.Amount
		want   float64
	}{
		{"used_over_limit", usage.Amount{Used: &pct, Limit: &cap, Unit: "tokens"}, 40.0},
		{"percent_unit", usage.Amount{Used: &pct, Unit: "percent"}, 80.0},
	}
	for _, c := range chain {
		report := &usage.Report{Provider: "p", Limits: []usage.Limit{{ID: "x", Amount: c.amount}}}
		proto := usage.ToProtoReport(report)
		got := proto.Limits[0].Amount.UsedPercent
		if got == nil || *got != c.want {
			t.Fatalf("%s: used_percent %v, want %v", c.name, got, c.want)
		}
	}
}

func TestToProtoReportUsedPercentClamped(t *testing.T) {
	frac := 1.7
	report := &usage.Report{
		Provider: "p",
		Limits:   []usage.Limit{{ID: "x", Amount: usage.Amount{UsedFraction: &frac}}},
	}
	proto := usage.ToProtoReport(report)
	got := proto.Limits[0].Amount.UsedPercent
	if got == nil || *got != 100.0 {
		t.Fatalf("clamped: %v", got)
	}
}

func TestToProtoReportDerivesResetLabel(t *testing.T) {
	now := time.Now().UnixMilli()
	in3h := now + 3*3600*1000 + 20*60*1000
	report := &usage.Report{
		Provider: "claude",
		Limits: []usage.Limit{{
			ID:     "x",
			Window: &usage.Window{ID: "5h", Label: "5 hours", ResetsAt: &in3h},
		}},
	}
	proto := usage.ToProtoReport(report)
	label := proto.Limits[0].Window.ResetLabel
	if label == nil || *label != "resets in 3h 20m" {
		t.Fatalf("reset_label: %v", label)
	}
}

func TestToProtoReportResetLabelPast(t *testing.T) {
	now := time.Now().UnixMilli()
	past := now - 1000
	report := &usage.Report{
		Provider: "claude",
		Limits: []usage.Limit{{
			ID:     "x",
			Window: &usage.Window{ID: "5h", Label: "5 hours", ResetsAt: &past},
		}},
	}
	proto := usage.ToProtoReport(report)
	label := proto.Limits[0].Window.ResetLabel
	if label == nil || *label != "resets momentarily" {
		t.Fatalf("reset_label: %v", label)
	}
}

func TestToProtoReportPreservesFetcherResetLabel(t *testing.T) {
	custom := "custom label"
	report := &usage.Report{
		Provider: "claude",
		Limits: []usage.Limit{{
			ID:     "x",
			Window: &usage.Window{ID: "w", Label: "W", ResetLabel: custom},
		}},
	}
	proto := usage.ToProtoReport(report)
	if label := proto.Limits[0].Window.ResetLabel; label == nil || *label != custom {
		t.Fatalf("reset_label: %v", label)
	}
}

func TestResetCountdownLabel(t *testing.T) {
	now := time.Now().UnixMilli()
	cases := []struct {
		offsetSecs int64
		want       string
	}{
		{-10, "resets momentarily"},
		{0, "resets momentarily"},
		{30, "resets in 30s"},
		{5 * 60, "resets in 5m"},
		{2*3600 + 5*60, "resets in 2h 5m"},
		{2 * 3600, "resets in 2h"},
		{3*86400 + 4*3600, "resets in 3d 4h"},
		{3 * 86400, "resets in 3d"},
	}
	for _, c := range cases {
		got := usage.ResetCountdownLabel(now+c.offsetSecs*1000, now)
		if got != c.want {
			t.Fatalf("offset %ds: got %q, want %q", c.offsetSecs, got, c.want)
		}
	}
}
