// Usage migration coverage: the trimmed proto reports must match the shared
// golden fixtures, logged-out providers stay JSON null, and validation is
// preserved.
package tests

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"
	"google.golang.org/protobuf/encoding/protojson"

	consolev1 "github.com/Adelodunpeter25/console/apps/server-go/internal/gen/console/v1"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/providers/usage"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/routes"
)

func usageFixture(t *testing.T, name string) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "proto", "testdata", "usage", name))
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(string(raw))
}

func strptr(s string) *string { return &s }
func f64ptr(f float64) *float64 { return &f }
func i64ptr(i int64) *int64 { return &i }

// canonicalReport mirrors usage_report.json: one fully populated limit.
func canonicalReport() *consolev1.UsageReport {
	return &consolev1.UsageReport{
		Provider: "claude", FetchedAt: 1700000000000,
		Limits: []*consolev1.UsageLimit{{
			Id: "session", Label: "Session",
			Scope: &consolev1.UsageScope{
				Provider: "claude", Tier: strptr("pro"), ModelId: strptr("claude-opus-4"),
			},
			Window: &consolev1.UsageWindow{
				Id: "session", Label: "Session",
				ResetsAt: i64ptr(1700003600000), ResetLabel: strptr("resets in 1h"),
			},
			Amount: &consolev1.UsageAmount{
				Used: f64ptr(12.5), Limit: f64ptr(100), Unit: "percent",
			},
			Status: strptr("ok"),
		}},
	}
}

func TestUsageProtoMatchesFixture(t *testing.T) {
	// NOTE: raw protojson emits ", " separators; the wire goes through
	// Fiber's encoding/json envelope which compacts it. Compact here so
	// the fixture captures true wire bytes.
	raw, err := protojson.Marshal(canonicalReport())
	if err != nil {
		t.Fatal(err)
	}
	if compactJSON(t, raw) != usageFixture(t, "report.json") {
		t.Fatalf("report bytes drifted:\n got %s\nwant %s", compactJSON(t, raw), usageFixture(t, "report.json"))
	}

	// Map encoding preserves nulls and sorts keys, matching the handler.
	entries := map[string]json.RawMessage{
		"codex":       json.RawMessage("null"),
		"claude":      mustMarshal(t, canonicalReport()),
		"antigravity": json.RawMessage("null"),
	}
	encoded, err := json.Marshal(entries)
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(encoded)) != usageFixture(t, "map.json") {
		t.Fatalf("map bytes drifted:\n got %s\nwant %s", encoded, usageFixture(t, "map.json"))
	}

	// Fixtures stay parseable with unknown-field tolerance.
	var decoded consolev1.UsageReport
	unmarshal := protojson.UnmarshalOptions{DiscardUnknown: true}
	if err := unmarshal.Unmarshal([]byte(usageFixture(t, "report.json")), &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.GetFetchedAt() != 1700000000000 || len(decoded.GetLimits()) != 1 {
		t.Fatalf("decoded: %+v", &decoded)
	}
}

func mustMarshal(t *testing.T, msg *consolev1.UsageReport) json.RawMessage {
	t.Helper()
	raw, err := protojson.Marshal(msg)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func compactJSON(t *testing.T, raw []byte) string {
	t.Helper()
	var buf bytes.Buffer
	if err := json.Compact(&buf, raw); err != nil {
		t.Fatal(err)
	}
	return buf.String()
}

// loggedOutEnv points every provider credential lookup at missing files so
// fetches fail closed with nil reports and no network traffic.
func loggedOutEnv(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("CODEX_CREDENTIALS_PATH", filepath.Join(dir, "missing.json"))
	t.Setenv("OPENAI_CODEX_OAUTH_TOKEN", "")
	t.Setenv("CLAUDE_CREDENTIALS_PATH", filepath.Join(dir, "missing.json"))
	t.Setenv("CLAUDE_OAUTH_TOKEN", "")
	t.Setenv("ANTHROPIC_OAUTH_TOKEN", "")
	t.Setenv("ANTIGRAVITY_CREDENTIALS_PATH", filepath.Join(dir, "missing.json"))
}

func TestUsageRoutesLoggedOut(t *testing.T) {
	loggedOutEnv(t)
	app := fiber.New()
	routes.RegisterUsageRoutes(app, usage.NewService())

	req := httptest.NewRequest("GET", "/api/usage", nil)
	resp, err := app.Test(req, 10000)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := io.ReadAll(resp.Body)
	var envelope struct {
		Success bool            `json:"success"`
		Data    map[string]any  `json:"data"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil || !envelope.Success {
		t.Fatalf("envelope: %s", raw)
	}
	for _, p := range []string{"antigravity", "codex", "claude"} {
		v, ok := envelope.Data[p]
		if !ok {
			t.Fatalf("missing provider %s: %s", p, raw)
		}
		if v != nil {
			t.Fatalf("logged-out %s must be null: %v", p, v)
		}
	}

	// Unknown provider is still a 400.
	bad := httptest.NewRequest("GET", "/api/providers/nope/usage", nil)
	if resp, err := app.Test(bad, 10000); err != nil || resp.StatusCode != 400 {
		t.Fatalf("unknown provider must 400: %v", resp)
	}

	// Logged-out provider returns success with null data.
	single := httptest.NewRequest("GET", "/api/providers/codex/usage", nil)
	resp, err = app.Test(single, 10000)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ = io.ReadAll(resp.Body)
	var singleEnvelope struct {
		Success bool            `json:"success"`
		Data    json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(raw, &singleEnvelope); err != nil || !singleEnvelope.Success {
		t.Fatalf("envelope: %s", raw)
	}
	if strings.TrimSpace(string(singleEnvelope.Data)) != "null" {
		t.Fatalf("logged-out report must be null: %s", singleEnvelope.Data)
	}
}
