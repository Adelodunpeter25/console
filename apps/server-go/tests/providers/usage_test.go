// Codex usage quota coverage: payload parsing, URL helpers, and the
// cached service against a mock wham/usage endpoint.
package tests

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/providers/claude"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/providers/codex"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/providers/usage"
)

func usageFixture() map[string]any {
	return map[string]any{
		"plan_type": "pro",
		"rate_limit": map[string]any{
			"allowed": true, "limit_reached": false,
			"primary_window": map[string]any{
				"used_percent": 80.0, "limit_window_seconds": 18000.0, "reset_after_seconds": 3600.0,
			},
			"secondary_window": map[string]any{
				"used_percent": 10.0, "limit_window_seconds": 604800.0,
			},
		},
		"additional_rate_limits": []any{
			map[string]any{
				"limit_name": "codex-spark", "metered_feature": "spark",
				"rate_limit": map[string]any{
					"allowed":        true,
					"primary_window": map[string]any{"used_percent": 100.0, "limit_window_seconds": 86400.0},
				},
			},
		},
		"rate_limit_reset_credits": map[string]any{"available_count": 3.0},
	}
}

func TestParseCodexUsage(t *testing.T) {
	now := time.Now().UnixMilli()
	report := usage.ParseCodexPayload(usageFixture(), "acc-1", "u@example.com", now)
	if report == nil {
		t.Fatal("expected report")
	}
	if report.Provider != "codex" || report.FetchedAt != now {
		t.Fatalf("report: %+v", report)
	}
	if len(report.Limits) != 3 {
		t.Fatalf("limits: %d", len(report.Limits))
	}
	primary := report.Limits[0]
	if primary.ID != "codex:primary" || primary.Status != "warning" {
		t.Fatalf("primary: %+v", primary)
	}
	if primary.Amount.Used == nil || *primary.Amount.Used != 80 || primary.Amount.Unit != "percent" {
		t.Fatalf("amount: %+v", primary.Amount)
	}
	if primary.Window == nil || primary.Window.ID != "5h" || primary.Window.DurationMs == nil {
		t.Fatalf("window: %+v", primary.Window)
	}
	if primary.Window.ResetsAt == nil || *primary.Window.ResetsAt != now+3600_000 {
		t.Fatalf("reset: %+v", primary.Window)
	}
	secondary := report.Limits[1]
	if secondary.ID != "codex:secondary" || secondary.Status != "ok" {
		t.Fatalf("secondary: %+v", secondary)
	}
	spark := report.Limits[2]
	if spark.ID != "codex:spark:primary" || spark.Status != "exhausted" {
		t.Fatalf("spark: %+v", spark)
	}
	if report.ResetCredits == nil || report.ResetCredits.AvailableCount != 3 {
		t.Fatalf("credits: %+v", report.ResetCredits)
	}
	meta := report.Metadata
	if meta["planType"] != "pro" || meta["accountId"] != "acc-1" || meta["email"] != "u@example.com" {
		t.Fatalf("metadata: %v", meta)
	}
}

func TestParseCodexUsageEmpty(t *testing.T) {
	now := time.Now().UnixMilli()
	if usage.ParseCodexPayload(map[string]any{}, "a", "", now) != nil {
		t.Fatal("empty payload must return nil")
	}
	if usage.ParseCodexPayload(nil, "a", "", now) != nil {
		t.Fatal("nil payload must return nil")
	}
	// Rate limit block without windows still yields metadata-only report.
	report := usage.ParseCodexPayload(map[string]any{
		"rate_limit": map[string]any{"allowed": true},
	}, "a", "", now)
	if report == nil || len(report.Limits) != 0 {
		t.Fatalf("allowed-only: %+v", report)
	}
}

func TestSharedUsageBuilders(t *testing.T) {
	if id, label := usage.WindowLabel(18000); id != "5h" || label != "5 hours" {
		t.Fatalf("window: %s %s", id, label)
	}
	if id, label := usage.WindowLabel(86400); id != "1d" || label != "1 day" {
		t.Fatalf("window: %s %s", id, label)
	}
	full, half, none := 1.0, 0.5, 0.0
	yes, no := true, false
	if got := usage.StatusForFraction(&full, &yes, &no); got != "warning" {
		t.Fatalf("explicit allowed at 100%%: %s", got)
	}
	if got := usage.StatusForFraction(&full, &yes, nil); got != "exhausted" {
		t.Fatalf("absent limitReached at 100%%: %s", got)
	}
	if got := usage.StatusForFraction(&full, nil, nil); got != "exhausted" {
		t.Fatalf("unknown at 100%%: %s", got)
	}
	if got := usage.StatusForFraction(&half, nil, nil); got != "warning" {
		t.Fatalf("half: %s", got)
	}
	if got := usage.StatusForFraction(&none, nil, nil); got != "ok" {
		t.Fatalf("empty: %s", got)
	}
	if got := usage.StatusForFraction(nil, nil, nil); got != "unknown" {
		t.Fatalf("nil: %s", got)
	}
	amount := usage.BuildPercentAmount(nil)
	if amount.Unit != "percent" || amount.UsedFraction != nil {
		t.Fatalf("unit-only: %+v", amount)
	}
	if _, ok := usage.Number("80"); !ok {
		t.Fatal("numeric string must parse")
	}
	if _, ok := usage.Number("x"); ok {
		t.Fatal("non-numeric must fail")
	}
}

func TestCodexUsageURLs(t *testing.T) {
	if got := usage.NormalizeCodexUsageBaseURL("https://chatgpt.com/backend-api/", "def"); got != "https://chatgpt.com/backend-api" {
		t.Fatalf("base: %s", got)
	}
	if got := usage.NormalizeCodexUsageBaseURL("https://evil.example.com/x", "def"); got != "def" {
		t.Fatalf("untrusted host: %s", got)
	}
	if got := usage.NormalizeCodexUsageBaseURL("", "def"); got != "def" {
		t.Fatalf("empty: %s", got)
	}
	if got := usage.CodexUsageURL("https://chatgpt.com/backend-api/"); got != "https://chatgpt.com/backend-api/wham/usage" {
		t.Fatalf("url: %s", got)
	}
}

func claudeUsageFixture() map[string]any {
	return map[string]any{
		"five_hour": map[string]any{"utilization": 8.0, "resets_at": "2026-09-21T19:00:00Z"},
		"seven_day": map[string]any{"utilization": 24.0, "resets_at": "2026-09-25T15:00:00Z"},
		"limits": []any{
			map[string]any{
				"kind": "weekly_scoped", "percent": 40.0, "resets_at": "2026-09-25T15:00:00Z",
				"scope": map[string]any{"model": map[string]any{"display_name": "Fable"}},
			},
		},
		"spend": map[string]any{
			"enabled": true, "limit": map[string]any{"amount_minor": 5000.0, "exponent": 2.0, "currency": "USD"},
			"used": map[string]any{"amount_minor": 1000.0, "exponent": 2.0, "currency": "USD"},
		},
	}
}

func TestParseClaudeUsage(t *testing.T) {
	now := time.Now().UnixMilli()
	report := usage.ParseClaudePayload(claudeUsageFixture(), "claude", "", "u@example.com", "https://api.anthropic.com/api/oauth/usage", now)
	if report == nil {
		t.Fatal("expected report")
	}
	if report.Provider != "claude" || report.FetchedAt != now {
		t.Fatalf("report: %+v", report)
	}
	if len(report.Limits) != 4 {
		t.Fatalf("limits: %d %+v", len(report.Limits), report.Limits)
	}
	fiveHour := report.Limits[0]
	if fiveHour.ID != "claude:5h" || fiveHour.Status != "ok" {
		t.Fatalf("5h: %+v", fiveHour)
	}
	if fiveHour.Amount.Used == nil || *fiveHour.Amount.Used != 8 || fiveHour.Amount.Unit != "percent" {
		t.Fatalf("5h amount: %+v", fiveHour.Amount)
	}
	sevenDay := report.Limits[1]
	if sevenDay.ID != "claude:7d" || sevenDay.Status != "ok" {
		t.Fatalf("7d: %+v", sevenDay)
	}
	fable := report.Limits[len(report.Limits)-2]
	if fable.ID != "claude:7d:fable" || fable.Amount.Used == nil || *fable.Amount.Used != 40 {
		t.Fatalf("fable: %+v", fable)
	}
	extra := report.Limits[len(report.Limits)-1]
	if extra.ID != "claude:extra" || extra.Amount.Used == nil || *extra.Amount.Used != 10 || extra.Amount.Unit != "usd" {
		t.Fatalf("extra: %+v", extra)
	}
	if report.Metadata["email"] != "u@example.com" {
		t.Fatalf("metadata: %v", report.Metadata)
	}
}

func TestParseClaudeUsageEmpty(t *testing.T) {
	now := time.Now().UnixMilli()
	if usage.ParseClaudePayload(map[string]any{}, "claude", "", "", "", now) != nil {
		t.Fatal("empty payload must return nil")
	}
	if usage.ParseClaudePayload(nil, "claude", "", "", "", now) != nil {
		t.Fatal("nil payload must return nil")
	}
}

func saveClaudeUsageCred(t *testing.T, dir string) {
	t.Helper()
	t.Setenv("CLAUDE_CREDENTIALS_PATH", filepath.Join(dir, "claude-creds.json"))
	cred := claude.OAuthCredential{
		AccessToken: "tok", RefreshToken: "ref",
		ExpiresAt: time.Now().UnixMilli() + 3600_000, Email: "u@example.com",
	}
	if err := claude.SaveCredential(cred); err != nil {
		t.Fatal(err)
	}
}

func TestUsageServiceClaudeFetchAndCache(t *testing.T) {
	var hits atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		if r.URL.Path != "/api/oauth/usage" {
			t.Errorf("path: %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer tok" {
			t.Errorf("auth header: %q", r.Header.Get("Authorization"))
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"five_hour":{"utilization":8,"resets_at":"2026-09-21T19:00:00Z"}}`)
	}))
	defer srv.Close()

	dir := t.TempDir()
	saveClaudeUsageCred(t, dir)
	svc := usage.NewService()
	svc.BaseURL = srv.URL

	first, err := svc.GetUsage(context.Background(), "claude")
	if err != nil || first == nil || len(first.Limits) != 1 {
		t.Fatalf("fetch: %+v %v", first, err)
	}
	second, err := svc.GetUsage(context.Background(), "claude")
	if err != nil || second == nil {
		t.Fatalf("cached: %+v %v", second, err)
	}
	if hits.Load() != 1 {
		t.Fatalf("expected 1 upstream hit, got %d", hits.Load())
	}
}

func saveUsageCred(t *testing.T, dir string) {
	t.Helper()
	t.Setenv("CODEX_CREDENTIALS_PATH", filepath.Join(dir, "codex-creds.json"))
	cred := codex.OAuthCredential{
		AccessToken: "tok", RefreshToken: "ref",
		ExpiresAt: time.Now().UnixMilli() + 3600_000, AccountID: "acc-1",
	}
	if err := codex.SaveCredential(cred); err != nil {
		t.Fatal(err)
	}
}

func TestUsageServiceLoggedOut(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CODEX_CREDENTIALS_PATH", filepath.Join(dir, "missing.json"))
	t.Setenv("OPENAI_CODEX_OAUTH_TOKEN", "")
	svc := usage.NewService()
	report, err := svc.GetUsage(context.Background(), "codex")
	if err != nil || report != nil {
		t.Fatalf("logged out: %+v %v", report, err)
	}
	if _, err := svc.GetUsage(context.Background(), "nope"); err == nil {
		t.Fatal("invalid provider must error")
	}
	all := svc.GetAllUsage(context.Background())
	for _, p := range []string{"antigravity", "codex", "claude"} {
		if _, ok := all[p]; !ok {
			t.Fatalf("missing key %s: %v", p, all)
		}
	}
}

func TestUsageServiceFetchAndCache(t *testing.T) {
	var hits atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		if r.URL.Path != "/wham/usage" {
			t.Errorf("path: %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer tok" {
			t.Errorf("auth header: %q", r.Header.Get("Authorization"))
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"plan_type":"pro","rate_limit":{"allowed":true,"primary_window":{"used_percent":25,"limit_window_seconds":3600}}}`)
	}))
	defer srv.Close()

	dir := t.TempDir()
	saveUsageCred(t, dir)
	svc := usage.NewService()
	svc.BaseURL = srv.URL

	first, err := svc.GetUsage(context.Background(), "codex")
	if err != nil || first == nil || len(first.Limits) != 1 {
		t.Fatalf("fetch: %+v %v", first, err)
	}
	second, err := svc.GetUsage(context.Background(), "codex")
	if err != nil || second == nil {
		t.Fatalf("cached: %+v %v", second, err)
	}
	if hits.Load() != 1 {
		t.Fatalf("expected 1 upstream hit, got %d", hits.Load())
	}
	svc.Invalidate("codex")
	if _, err := svc.GetUsage(context.Background(), "codex"); err != nil {
		t.Fatal(err)
	}
	if hits.Load() != 2 {
		t.Fatalf("expected refetch after invalidate, got %d", hits.Load())
	}
}
