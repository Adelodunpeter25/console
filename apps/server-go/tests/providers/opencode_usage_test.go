// OpenCode Go usage quota coverage: payload parsing, URL helpers, and the
// cached service against a mock /usage endpoint.
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

	"github.com/Adelodunpeter25/console/apps/server-go/internal/providers/usage"
)

func openCodeUsageFixture() map[string]any {
	return map[string]any{
		"usage": map[string]any{
			"rolling": map[string]any{
				"status": "ok", "percent": 0.0, "resetsAt": "2026-10-20T10:03:22.000Z",
			},
			"weekly": map[string]any{
				"status": "ok", "percent": 13.0, "resetsAt": "2026-10-26T00:00:00.000Z",
			},
			"monthly": map[string]any{
				"status": "ok", "percent": 23.0, "resetsAt": "2026-11-16T09:25:50.000Z",
			},
		},
	}
}

func TestParseOpenCodeUsage(t *testing.T) {
	now := time.Now().UnixMilli()
	report := usage.ParseOpenCodePayload(openCodeUsageFixture(), now)
	if report == nil {
		t.Fatal("expected report")
	}
	if report.Provider != "opencode" || report.FetchedAt != now {
		t.Fatalf("report: %+v", report)
	}
	if len(report.Limits) != 3 {
		t.Fatalf("limits: %d %+v", len(report.Limits), report.Limits)
	}

	rolling := report.Limits[0]
	if rolling.ID != "opencode:5h" || rolling.Status != "ok" {
		t.Fatalf("rolling: %+v", rolling)
	}
	if rolling.Window == nil || rolling.Window.ID != "5h" || rolling.Window.Label != "5 hours" {
		t.Fatalf("rolling window: %+v", rolling.Window)
	}
	if rolling.Window.ResetsAt == nil {
		t.Fatalf("rolling reset: %+v", rolling.Window)
	}
	if rolling.Amount.Used == nil || *rolling.Amount.Used != 0 || rolling.Amount.Unit != "percent" {
		t.Fatalf("rolling amount: %+v", rolling.Amount)
	}
	if rolling.Amount.Remaining == nil || *rolling.Amount.Remaining != 100 {
		t.Fatalf("rolling remaining: %+v", rolling.Amount)
	}

	weekly := report.Limits[1]
	if weekly.ID != "opencode:weekly" || weekly.Status != "ok" {
		t.Fatalf("weekly: %+v", weekly)
	}
	if weekly.Amount.Used == nil || *weekly.Amount.Used != 13 {
		t.Fatalf("weekly amount: %+v", weekly.Amount)
	}
	if weekly.Amount.RemainingFraction == nil || *weekly.Amount.RemainingFraction != 0.87 {
		t.Fatalf("weekly remaining fraction: %+v", weekly.Amount)
	}

	monthly := report.Limits[2]
	if monthly.ID != "opencode:monthly" || monthly.Status != "ok" {
		t.Fatalf("monthly: %+v", monthly)
	}
	if monthly.Amount.Used == nil || *monthly.Amount.Used != 23 {
		t.Fatalf("monthly amount: %+v", monthly.Amount)
	}
	if monthly.Scope.Provider != "opencode" || monthly.Scope.WindowID != "monthly" {
		t.Fatalf("monthly scope: %+v", monthly.Scope)
	}
}

func TestParseOpenCodeUsageEmpty(t *testing.T) {
	now := time.Now().UnixMilli()
	if usage.ParseOpenCodePayload(map[string]any{}, now) != nil {
		t.Fatal("empty payload must return nil")
	}
	if usage.ParseOpenCodePayload(nil, now) != nil {
		t.Fatal("nil payload must return nil")
	}
	// A window without a percent is dropped rather than shown as 0% used.
	report := usage.ParseOpenCodePayload(map[string]any{
		"usage": map[string]any{
			"weekly":  map[string]any{"status": "ok", "percent": 10.0},
			"monthly": map[string]any{"status": "ok"},
		},
	}, now)
	if report == nil || len(report.Limits) != 1 || report.Limits[0].ID != "opencode:weekly" {
		t.Fatalf("partial payload: %+v", report)
	}
}

func TestParseOpenCodeUsageStatus(t *testing.T) {
	now := time.Now().UnixMilli()
	report := usage.ParseOpenCodePayload(map[string]any{
		"usage": map[string]any{
			"monthly": map[string]any{"status": "exhausted", "percent": 100.0},
		},
	}, now)
	if report == nil || len(report.Limits) != 1 {
		t.Fatalf("report: %+v", report)
	}
	if report.Limits[0].Status != "exhausted" {
		t.Fatalf("status: %+v", report.Limits[0])
	}

	// An unknown upstream status falls back to the shared thresholds.
	report = usage.ParseOpenCodePayload(map[string]any{
		"usage": map[string]any{
			"monthly": map[string]any{"status": "brand-new", "percent": 60.0},
		},
	}, now)
	if report == nil || report.Limits[0].Status != "warning" {
		t.Fatalf("fallback status: %+v", report)
	}
}

func TestOpenCodeUsageURL(t *testing.T) {
	if got := usage.OpenCodeUsageURL("https://opencode.ai/inference/go/v1"); got != "https://opencode.ai/inference/go/v1/usage" {
		t.Fatalf("url: %s", got)
	}
	if got := usage.OpenCodeUsageURL("https://opencode.ai/inference/go/v1/"); got != "https://opencode.ai/inference/go/v1/usage" {
		t.Fatalf("trailing slash: %s", got)
	}
	if got := usage.OpenCodeUsageURL(""); got != "https://opencode.ai/inference/go/v1/usage" {
		t.Fatalf("default: %s", got)
	}
}

func TestFetchOpenCodeUsage(t *testing.T) {
	var hits atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		if r.URL.Path != "/usage" {
			t.Errorf("path: %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer oc_test" {
			t.Errorf("auth header: %q", r.Header.Get("Authorization"))
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"usage":{"rolling":{"status":"ok","percent":4,"resetsAt":"2026-10-20T10:03:22.000Z"}}}`)
	}))
	defer srv.Close()

	report, err := usage.FetchOpenCodeUsage(context.Background(), srv.Client(), srv.URL, "oc_test")
	if err != nil || report == nil || len(report.Limits) != 1 {
		t.Fatalf("fetch: %+v %v", report, err)
	}
	if report.Limits[0].Amount.Used == nil || *report.Limits[0].Amount.Used != 4 {
		t.Fatalf("amount: %+v", report.Limits[0].Amount)
	}

	// No key configured: no request, no report.
	report, err = usage.FetchOpenCodeUsage(context.Background(), srv.Client(), srv.URL, "")
	if err != nil || report != nil {
		t.Fatalf("no key: %+v %v", report, err)
	}
	if hits.Load() != 1 {
		t.Fatalf("expected 1 upstream hit, got %d", hits.Load())
	}
}

func TestUsageServiceOpenCodeFetchAndCache(t *testing.T) {
	var hits atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		if r.URL.Path != "/usage" {
			t.Errorf("path: %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer oc_test" {
			t.Errorf("auth header: %q", r.Header.Get("Authorization"))
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"usage":{"monthly":{"status":"ok","percent":42,"resetsAt":"2026-11-16T09:25:50.000Z"}}}`)
	}))
	defer srv.Close()

	dir := t.TempDir()
	t.Setenv("CONSOLE_PROVIDER_KEYS_PATH", filepath.Join(dir, "missing.json"))
	t.Setenv("OPENCODE_API_KEY", "oc_test")
	t.Setenv("OPENCODE_BASE_URL", srv.URL)

	svc := usage.NewService()
	first, err := svc.GetUsage(context.Background(), "opencode")
	if err != nil || first == nil || len(first.Limits) != 1 {
		t.Fatalf("fetch: %+v %v", first, err)
	}
	if first.Limits[0].ID != "opencode:monthly" {
		t.Fatalf("limit: %+v", first.Limits[0])
	}
	second, err := svc.GetUsage(context.Background(), "opencode")
	if err != nil || second == nil {
		t.Fatalf("cached: %+v %v", second, err)
	}
	if hits.Load() != 1 {
		t.Fatalf("expected 1 upstream hit, got %d", hits.Load())
	}
	svc.Invalidate("opencode")
	if _, err := svc.GetUsage(context.Background(), "opencode"); err != nil {
		t.Fatal(err)
	}
	if hits.Load() != 2 {
		t.Fatalf("expected refetch after invalidate, got %d", hits.Load())
	}
}

func TestUsageServiceOpenCodeLoggedOut(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CONSOLE_PROVIDER_KEYS_PATH", filepath.Join(dir, "missing.json"))
	t.Setenv("OPENCODE_API_KEY", "")

	svc := usage.NewService()
	report, err := svc.GetUsage(context.Background(), "opencode")
	if err != nil || report != nil {
		t.Fatalf("logged out: %+v %v", report, err)
	}
}
