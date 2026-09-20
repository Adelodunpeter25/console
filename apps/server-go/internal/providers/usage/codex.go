// Codex quota fetcher. Port of providers/src/usage/openai-codex.ts:
// GET {base}/wham/usage with the subscription token, parsed into a
// UsageReport with primary/secondary/additional limits.
package usage

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/providers/shared"
)

const codexUsagePath = "wham/usage"
const codexUserAgent = "console/1.0"

// NormalizeCodexUsageBaseURL mirrors normalizeCodexBaseUrl: only
// chatgpt.com/chat.openai.com origins are honored, else the default.
func NormalizeCodexUsageBaseURL(baseURL, def string) string {
	trimmed := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if trimmed == "" {
		return def
	}
	parsed, err := url.Parse(trimmed)
	if err != nil {
		return def
	}
	host := strings.ToLower(parsed.Host)
	if host != "chatgpt.com" && host != "chat.openai.com" {
		return def
	}
	return parsed.Scheme + "://" + parsed.Host + "/backend-api"
}

// CodexUsageURL appends the wham/usage path to a base URL.
func CodexUsageURL(baseURL string) string {
	return strings.TrimRight(baseURL, "/") + "/" + codexUsagePath
}

type codexWindow struct {
	usedPercent        *float64
	limitWindowSeconds *float64
	resetAfterSeconds  *float64
	resetAt            *float64
}

type codexAdditional struct {
	limitName      string
	meteredFeature string
	allowed        *bool
	limitReached   *bool
	primary        *codexWindow
	secondary      *codexWindow
}

func toNumber(v any) (float64, bool) {
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

func toBool(v any) (*bool, bool) {
	b, ok := v.(bool)
	if !ok {
		return nil, false
	}
	return &b, true
}

func asRecord(v any) (map[string]any, bool) {
	m, ok := v.(map[string]any)
	return m, ok
}

func parseCodexWindow(payload any) *codexWindow {
	m, ok := asRecord(payload)
	if !ok {
		return nil
	}
	w := &codexWindow{}
	if v, ok := toNumber(m["used_percent"]); ok {
		w.usedPercent = &v
	}
	if v, ok := toNumber(m["limit_window_seconds"]); ok {
		w.limitWindowSeconds = &v
	}
	if v, ok := toNumber(m["reset_after_seconds"]); ok {
		w.resetAfterSeconds = &v
	}
	if v, ok := toNumber(m["reset_at"]); ok {
		w.resetAt = &v
	}
	if w.usedPercent == nil && w.limitWindowSeconds == nil && w.resetAfterSeconds == nil && w.resetAt == nil {
		return nil
	}
	return w
}

func parseCodexAdditional(payload any) *codexAdditional {
	m, ok := asRecord(payload)
	if !ok {
		return nil
	}
	rl, ok := asRecord(m["rate_limit"])
	if !ok {
		return nil
	}
	out := &codexAdditional{}
	if s, ok := m["limit_name"].(string); ok {
		out.limitName = s
	}
	if s, ok := m["metered_feature"].(string); ok {
		out.meteredFeature = s
	}
	if b, ok := toBool(rl["allowed"]); ok {
		out.allowed = b
	}
	if b, ok := toBool(rl["limit_reached"]); ok {
		out.limitReached = b
	}
	out.primary = parseCodexWindow(rl["primary_window"])
	out.secondary = parseCodexWindow(rl["secondary_window"])
	if out.primary == nil && out.secondary == nil && out.allowed == nil && out.limitReached == nil {
		return nil
	}
	return out
}

func windowLabel(seconds float64) (id, label string) {
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

func resolveResetTime(w *codexWindow, nowMs int64) *int64 {
	if w.resetAt != nil {
		ms := *w.resetAt
		if ms <= 1_000_000_000_000 {
			ms *= 1000
		}
		if !math.IsNaN(ms) && !math.IsInf(ms, 0) {
			out := int64(ms)
			return &out
		}
	}
	if w.resetAfterSeconds != nil {
		out := nowMs + int64(*w.resetAfterSeconds*1000)
		return &out
	}
	return nil
}

func buildWindow(w *codexWindow, key string, nowMs int64) Window {
	out := Window{ID: key}
	if w.limitWindowSeconds != nil {
		id, label := windowLabel(*w.limitWindowSeconds)
		out.ID, out.Label = id, label
		dur := int64(*w.limitWindowSeconds * 1000)
		out.DurationMs = &dur
	} else if key == "primary" {
		out.Label = "Primary window"
	} else {
		out.Label = "Secondary window"
	}
	out.ResetsAt = resolveResetTime(w, nowMs)
	return out
}

func buildAmount(w *codexWindow) Amount {
	out := Amount{Unit: "percent"}
	if w.usedPercent == nil {
		return out
	}
	clamped := math.Min(math.Max(*w.usedPercent, 0), 100)
	frac := clamped / 100
	rem := math.Max(0, 100-clamped)
	remFrac := math.Max(0, 1-frac)
	out.Used, out.Limit, out.Remaining = &clamped, ptrFloat(100), &rem
	out.UsedFraction, out.RemainingFraction = &frac, &remFrac
	return out
}

func ptrFloat(v float64) *float64 { return &v }

func usageStatus(amount Amount, allowed, limitReached *bool) string {
	if amount.UsedFraction == nil {
		return "unknown"
	}
	f := *amount.UsedFraction
	// Mirrors buildUsageStatus: explicitlyAllowed is allowed===true &&
	// limitReached===false (undefined is not false).
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

func additionalSlug(limitName, meteredFeature string) string {
	probe := strings.ToLower(limitName + " " + meteredFeature)
	if strings.Contains(probe, "spark") || strings.Contains(probe, "bengalfox") {
		return "spark"
	}
	source := meteredFeature
	if source == "" {
		source = limitName
	}
	if source == "" {
		source = "extra"
	}
	slug := strings.ToLower(source)
	slug = strings.TrimPrefix(slug, "codex-")
	slug = strings.TrimPrefix(slug, "codex_")
	var b strings.Builder
	prevDash := true
	for _, r := range slug {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			prevDash = false
		} else if !prevDash {
			b.WriteRune('-')
			prevDash = true
		}
	}
	slug = strings.Trim(b.String(), "-")
	if slug == "" {
		return "extra"
	}
	return slug
}

func additionalDisplayName(slug, limitName string) string {
	if slug == "spark" {
		return "Spark"
	}
	if limitName != "" {
		return limitName
	}
	parts := strings.Split(slug, "-")
	for i, p := range parts {
		if len(p) > 0 {
			parts[i] = strings.ToUpper(p[:1]) + p[1:]
		}
	}
	return strings.Join(parts, " ")
}

func primaryLimit(key string, w *codexWindow, accountID, planType string, allowed, limitReached *bool, nowMs int64) Limit {
	window := buildWindow(w, key, nowMs)
	amount := buildAmount(w)
	return Limit{
		ID: "codex:" + key, Label: window.Label,
		Scope: Scope{Provider: "codex", WindowID: window.ID, Shared: true},
		Window: &window, Amount: amount,
		Status: usageStatus(amount, allowed, limitReached),
	}
}

func additionalLimit(key, slug, displayName string, w *codexWindow, accountID, limitName, meteredFeature string, allowed, limitReached *bool, nowMs int64) Limit {
	window := buildWindow(w, key, nowMs)
	amount := buildAmount(w)
	scope := Scope{Provider: "codex", WindowID: window.ID, Shared: true, Tier: slug}
	if accountID != "" {
		scope.AccountID = accountID
	}
	if limitName != "" {
		scope.ModelID = limitName
	}
	return Limit{
		ID: "codex:" + slug + ":" + key, Label: window.Label + " (" + displayName + ")",
		Scope: scope, Window: &window, Amount: amount,
		Status: usageStatus(amount, allowed, limitReached),
	}
}

// ParseCodexPayload converts a wham/usage payload to a Report. Returns nil
// when the payload carries no usable limit data.
func ParseCodexPayload(payload any, accountID, email string, nowMs int64) *Report {
	m, ok := asRecord(payload)
	if !ok {
		return nil
	}
	var planType string
	if s, ok := m["plan_type"].(string); ok {
		planType = s
	}
	rl, _ := asRecord(m["rate_limit"])
	var allowed, limitReached *bool
	if rl != nil {
		allowed, _ = toBool(rl["allowed"])
		limitReached, _ = toBool(rl["limit_reached"])
	}
	var primary, secondary *codexWindow
	if rl != nil {
		primary = parseCodexWindow(rl["primary_window"])
		secondary = parseCodexWindow(rl["secondary_window"])
	}
	var additional []*codexAdditional
	if raw, ok := m["additional_rate_limits"].([]any); ok {
		for _, item := range raw {
			if a := parseCodexAdditional(item); a != nil {
				additional = append(additional, a)
			}
		}
	}
	if rl == nil && len(additional) == 0 {
		return nil
	}
	if primary == nil && secondary == nil && allowed == nil && limitReached == nil && len(additional) == 0 {
		return nil
	}

	var limits []Limit
	meterStates := map[string]any{
		"chat": map[string]any{"allowed": allowed, "limitReached": limitReached},
	}
	if primary != nil {
		limits = append(limits, primaryLimit("primary", primary, accountID, planType, allowed, limitReached, nowMs))
	}
	if secondary != nil {
		limits = append(limits, primaryLimit("secondary", secondary, accountID, planType, allowed, limitReached, nowMs))
	}
	for _, extra := range additional {
		slug := additionalSlug(extra.limitName, extra.meteredFeature)
		display := additionalDisplayName(slug, extra.limitName)
		meterStates[slug] = map[string]any{"allowed": extra.allowed, "limitReached": extra.limitReached}
		if extra.primary != nil {
			limits = append(limits, additionalLimit("primary", slug, display, extra.primary, accountID, extra.limitName, extra.meteredFeature, extra.allowed, extra.limitReached, nowMs))
		}
		if extra.secondary != nil {
			limits = append(limits, additionalLimit("secondary", slug, display, extra.secondary, accountID, extra.limitName, extra.meteredFeature, extra.allowed, extra.limitReached, nowMs))
		}
	}

	var resetCredits *ResetCredits
	if block, ok := asRecord(m["rate_limit_reset_credits"]); ok {
		if n, ok := toNumber(block["available_count"]); ok {
			count := int(math.Max(0, math.Trunc(n)))
			resetCredits = &ResetCredits{AvailableCount: count}
		}
	}

	metadata := map[string]any{
		"planType": planType, "allowed": allowed, "limitReached": limitReached,
		"email": email, "accountId": accountID, "meterStates": meterStates,
	}
	return &Report{
		Provider: "codex", FetchedAt: nowMs, Limits: limits,
		ResetCredits: resetCredits, Metadata: metadata, Raw: payload,
	}
}

// FetchCodexUsage GETs the wham/usage endpoint and parses the report.
// Returns (nil, nil) when not logged in, the token is expired, or the
// upstream call fails — matching the TS provider's null contract.
func FetchCodexUsage(ctx context.Context, client *http.Client, baseURL, accessToken, accountID, email string, expiresAtMs int64) (*Report, error) {
	if accessToken == "" {
		return nil, nil
	}
	nowMs := time.Now().UnixMilli()
	if expiresAtMs > 0 && expiresAtMs <= nowMs {
		return nil, nil
	}
	if client == nil {
		client = &http.Client{Timeout: 5 * time.Second}
	}
	req, err := http.NewRequestWithContext(ctx, "GET", CodexUsageURL(baseURL), nil)
	if err != nil {
		return nil, nil
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("User-Agent", codexUserAgent)
	if accountID != "" {
		req.Header.Set("ChatGPT-Account-Id", accountID)
	}
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
	if accountID == "" {
		if profile := shared.DecodeJWTPayload(accessToken); profile != nil {
			if auth, ok := asRecord(profile["https://api.openai.com/auth"]); ok {
				if id, ok := auth["chatgpt_account_id"].(string); ok {
					accountID = id
				}
			}
			if email == "" {
				if prof, ok := asRecord(profile["https://api.openai.com/profile"]); ok {
					if e, ok := prof["email"].(string); ok {
						email = strings.TrimSpace(strings.ToLower(e))
					}
				}
			}
		}
	}
	return ParseCodexPayload(payload, accountID, email, nowMs), nil
}
