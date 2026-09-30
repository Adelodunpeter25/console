// Antigravity usage quota coverage: 5h vs weekly Google windows staying
// visible as separate rows, canonical hour-window classification, and
// reported fractions beating fabricated zeros in dedup.
package tests

import (
	"testing"
	"time"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/providers/usage"
)

func agISO(t *testing.T, in time.Duration) string {
	t.Helper()
	return time.Now().Add(in).UTC().Format(time.RFC3339)
}

func agGoogleModel(quota map[string]any) map[string]any {
	return map[string]any{
		"modelProvider": "MODEL_PROVIDER_GOOGLE",
		"apiProvider":   "API_PROVIDER_GOOGLE_GEMINI",
		"quotaInfo":     quota,
	}
}

func agFindLimit(limits []usage.Limit, id string) *usage.Limit {
	for i := range limits {
		if limits[i].ID == id {
			return &limits[i]
		}
	}
	return nil
}

// Explicit 5-hour window and long window must surface as two rows with
// distinct ids and window-named labels — never merged.
func TestParseAntigravityFiveHourAndWeekly(t *testing.T) {
	now := time.Now().UnixMilli()
	payload := map[string]any{"models": map[string]any{
		"gemini-short": agGoogleModel(map[string]any{
			"windowLabel": "5 hours", "resetTime": agISO(t, 2*time.Hour), "remainingFraction": 0.6,
		}),
		"gemini-long": agGoogleModel(map[string]any{
			"resetTime": agISO(t, 40*time.Hour), "remainingFraction": 0.1,
		}),
	}}
	report := usage.ParseAntigravityPayload(payload, "antigravity", "", "", "", now)
	if report == nil {
		t.Fatal("expected report")
	}
	if len(report.Limits) != 2 {
		t.Fatalf("limits: %d %+v", len(report.Limits), report.Limits)
	}
	short := agFindLimit(report.Limits, "antigravity:google:default:5h")
	if short == nil {
		t.Fatalf("missing 5h row: %+v", report.Limits)
	}
	if short.Label != "Usage (Google) · 5 hours" {
		t.Fatalf("5h label: %q", short.Label)
	}
	if short.Window == nil || short.Window.DurationMs == nil || *short.Window.DurationMs != 5*3600*1000 {
		t.Fatalf("5h window: %+v", short.Window)
	}
	if short.Amount.RemainingFraction == nil || *short.Amount.RemainingFraction != 0.6 {
		t.Fatalf("5h amount: %+v", short.Amount)
	}
	long := agFindLimit(report.Limits, "antigravity:google:default:weekly")
	if long == nil {
		t.Fatalf("missing weekly row: %+v", report.Limits)
	}
	if long.Label != "Usage (Google) · Weekly" {
		t.Fatalf("weekly label: %q", long.Label)
	}
}

// Raw hour spellings canonicalize to the same 5h identity.
func TestParseAntigravityHourSpellings(t *testing.T) {
	for _, raw := range []string{"5h", "5 hour", "5-hour", "5_hours"} {
		now := time.Now().UnixMilli()
		payload := map[string]any{"models": map[string]any{
			"gemini-a": agGoogleModel(map[string]any{
				"windowId": raw, "resetTime": agISO(t, 2*time.Hour), "remainingFraction": 0.5,
			}),
		}}
		report := usage.ParseAntigravityPayload(payload, "antigravity", "", "", "", now)
		if report == nil || len(report.Limits) != 1 {
			t.Fatalf("%q: %+v", raw, report)
		}
		if report.Limits[0].ID != "antigravity:google:default:5h" {
			t.Fatalf("%q id: %q", raw, report.Limits[0].ID)
		}
	}
}

// Unlabeled Google entries without a fraction are the weekly window, no
// matter how close the reset is — reset distance must never relabel them.
func TestParseAntigravityUnlabeledShortReset(t *testing.T) {
	now := time.Now().UnixMilli()
	payload := map[string]any{"models": map[string]any{
		"gemini-a": agGoogleModel(map[string]any{"resetTime": agISO(t, 2*time.Hour)}),
	}}
	report := usage.ParseAntigravityPayload(payload, "antigravity", "", "", "", now)
	if report == nil || len(report.Limits) != 1 {
		t.Fatalf("report: %+v", report)
	}
	if report.Limits[0].ID != "antigravity:google:default:weekly" {
		t.Fatalf("id: %q", report.Limits[0].ID)
	}
}

// A Google fraction that resets soon (or never) is the 5-hour window.
func TestParseAntigravityGoogleFractionShortReset(t *testing.T) {
	now := time.Now().UnixMilli()
	payload := map[string]any{"models": map[string]any{
		"gemini-a": agGoogleModel(map[string]any{
			"resetTime": agISO(t, 2*time.Hour), "remainingFraction": 0.8,
		}),
	}}
	report := usage.ParseAntigravityPayload(payload, "antigravity", "", "", "", now)
	if report == nil || len(report.Limits) != 1 {
		t.Fatalf("report: %+v", report)
	}
	if report.Limits[0].ID != "antigravity:google:default:5h" {
		t.Fatalf("id: %q", report.Limits[0].ID)
	}
	if report.Limits[0].Label != "Usage (Google) · 5 hours" {
		t.Fatalf("label: %q", report.Limits[0].Label)
	}
}

// A reported fraction must beat a fabricated zero under the same window no
// matter which model the map iterator visits first.
func TestParseAntigravityReportedBeatsAssumed(t *testing.T) {
	now := time.Now().UnixMilli()
	reset := agISO(t, 14*time.Hour)
	anthropic := func(quota map[string]any) map[string]any {
		return map[string]any{
			"modelProvider": "MODEL_PROVIDER_ANTHROPIC",
			"apiProvider":   "API_PROVIDER_ANTHROPIC_VERTEX",
			"quotaInfo":     quota,
		}
	}
	payload := map[string]any{"models": map[string]any{
		"claude-a": anthropic(map[string]any{"resetTime": reset}),
		"claude-b": anthropic(map[string]any{"resetTime": reset, "remainingFraction": 0.7}),
	}}
	report := usage.ParseAntigravityPayload(payload, "antigravity", "", "", "", now)
	if report == nil || len(report.Limits) != 1 {
		t.Fatalf("report: %+v", report)
	}
	got := report.Limits[0]
	if got.ID != "antigravity:anthropic:default:daily" {
		t.Fatalf("id: %q", got.ID)
	}
	if got.Amount.RemainingFraction == nil || *got.Amount.RemainingFraction != 0.7 {
		t.Fatalf("assumed zero shadowed real data: %+v", got.Amount)
	}
}

// Shape of the real fetchAvailableModels snapshot: resetTime-only Google
// entries plus fraction-bearing Anthropic ones.
func TestParseAntigravityRealSnapshotShape(t *testing.T) {
	now := time.Now().UnixMilli()
	payload := map[string]any{"models": map[string]any{
		"gemini-a": agGoogleModel(map[string]any{"resetTime": agISO(t, 14*time.Hour)}),
		"gemini-b": agGoogleModel(map[string]any{"resetTime": agISO(t, 14*time.Hour)}),
		"claude-a": map[string]any{
			"modelProvider": "MODEL_PROVIDER_ANTHROPIC",
			"apiProvider":   "API_PROVIDER_ANTHROPIC_VERTEX",
			"quotaInfo":     map[string]any{"resetTime": agISO(t, 40*time.Hour), "remainingFraction": 0.03},
		},
	}}
	report := usage.ParseAntigravityPayload(payload, "antigravity", "", "", "", now)
	if report == nil {
		t.Fatal("expected report")
	}
	google := agFindLimit(report.Limits, "antigravity:google:default:weekly")
	if google == nil {
		t.Fatalf("missing google weekly row: %+v", report.Limits)
	}
	if google.Label != "Usage (Google) · Weekly" {
		t.Fatalf("google label: %q", google.Label)
	}
	anthropic := agFindLimit(report.Limits, "antigravity:anthropic:default:weekly")
	if anthropic == nil {
		t.Fatalf("missing anthropic weekly row: %+v", report.Limits)
	}
}

// The gpt-oss vertex model is dropped — Anthropic only, no OpenAI row.
func TestParseAntigravityOpenAIDropped(t *testing.T) {
	now := time.Now().UnixMilli()
	payload := map[string]any{"models": map[string]any{
		"gpt-a": map[string]any{
			"modelProvider": "MODEL_PROVIDER_OPENAI",
			"apiProvider":   "API_PROVIDER_OPENAI_VERTEX",
			"quotaInfo":     map[string]any{"resetTime": agISO(t, 40*time.Hour), "remainingFraction": 0.5},
		},
		"claude-a": map[string]any{
			"modelProvider": "MODEL_PROVIDER_ANTHROPIC",
			"apiProvider":   "API_PROVIDER_ANTHROPIC_VERTEX",
			"quotaInfo":     map[string]any{"resetTime": agISO(t, 40*time.Hour), "remainingFraction": 0.5},
		},
	}}
	report := usage.ParseAntigravityPayload(payload, "antigravity", "", "", "", now)
	if report == nil {
		t.Fatal("expected report")
	}
	for _, l := range report.Limits {
		if l.Scope.Provider == "antigravity" && (l.ID == "antigravity:openai:default:weekly" || l.Label == "Usage (OpenAI) · Weekly") {
			t.Fatalf("openai row survived: %+v", l)
		}
	}
	if agFindLimit(report.Limits, "antigravity:anthropic:default:weekly") == nil {
		t.Fatalf("anthropic row missing: %+v", report.Limits)
	}
}
