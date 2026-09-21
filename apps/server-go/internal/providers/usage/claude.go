// Claude quota fetcher. Port of providers/src/usage/claude.ts: GET
// {base}/usage with the subscription OAuth token, parsed into a
// UsageReport with 5h/7d/scoped/extra limits.
package usage

import (
	"context"
	"encoding/json"
	"io"
	"math"
	"net/http"
	"strings"
	"time"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/providers/claude"
)

const claudeDefaultEndpoint = "https://api.anthropic.com/api/oauth"
const hourMs = int64(60 * 60 * 1000)
const weekMs = int64(7 * 24 * 60 * 60 * 1000)

func claudeHeaders(accessToken string) map[string]string {
	return map[string]string{
		"Accept":         "application/json, text/plain, */*",
		"Content-Type":   "application/json",
		"User-Agent":     "claude-cli/" + claude.CodeVersion + " (external, cli)",
		"anthropic-beta": "claude-code-20250219,oauth-2025-04-20,interleaved-thinking-2025-05-14,context-management-2025-06-27",
		"Authorization":  "Bearer " + accessToken,
	}
}

// normalizeClaudeUsageBaseURL mirrors normalizeBaseUrl in the TS provider.
func normalizeClaudeUsageBaseURL(baseURL string) string {
	trimmed := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if trimmed == "" {
		return claudeDefaultEndpoint
	}
	lower := strings.ToLower(trimmed)
	if strings.HasSuffix(lower, "/api/oauth") {
		return trimmed
	}
	origin, path, ok := splitOriginPath(trimmed)
	if !ok {
		return claudeDefaultEndpoint
	}
	path = strings.TrimRight(path, "/")
	if strings.HasSuffix(strings.ToLower(path), "/v1") {
		path = path[:len(path)-3]
	}
	if path == "" {
		return origin + "/api/oauth"
	}
	return origin + path + "/api/oauth"
}

func splitOriginPath(raw string) (origin, path string, ok bool) {
	idx := strings.Index(raw, "://")
	if idx < 0 {
		return "", "", false
	}
	rest := raw[idx+3:]
	slash := strings.Index(rest, "/")
	if slash < 0 {
		return raw, "", true
	}
	return raw[:idx+3+slash], rest[slash:], true
}

func parseIsoTimestampMs(v any) *int64 {
	s, ok := v.(string)
	if !ok || s == "" {
		return nil
	}
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339} {
		if t, err := time.Parse(layout, s); err == nil {
			ms := t.UnixMilli()
			return &ms
		}
	}
	return nil
}

// claudeBucket is a normalized utilization/reset pair.
type claudeBucket struct {
	utilization *float64
	resetsAtMs  *int64
}

func parseClaudeBucket(payload any) *claudeBucket {
	m, ok := Record(payload)
	if !ok {
		return nil
	}
	b := &claudeBucket{}
	if v, ok := Number(m["utilization"]); ok {
		b.utilization = &v
	}
	b.resetsAtMs = parseIsoTimestampMs(m["resets_at"])
	if b.utilization == nil && b.resetsAtMs == nil {
		return nil
	}
	return b
}

type claudeLimitEntry struct {
	kind        string
	bucket      *claudeBucket
	displayName string
}

func parseClaudeLimitEntries(raw any) []claudeLimitEntry {
	list, ok := raw.([]any)
	if !ok {
		return nil
	}
	var out []claudeLimitEntry
	for _, item := range list {
		m, ok := Record(item)
		if !ok {
			continue
		}
		kind, ok := m["kind"].(string)
		if !ok {
			continue
		}
		utilization, hasUtil := Number(m["percent"])
		resetsAt := parseIsoTimestampMs(m["resets_at"])
		if !hasUtil && resetsAt == nil {
			continue
		}
		bucket := &claudeBucket{resetsAtMs: resetsAt}
		if hasUtil {
			bucket.utilization = &utilization
		}
		entry := claudeLimitEntry{kind: kind, bucket: bucket}
		if scope, ok := Record(m["scope"]); ok {
			if model, ok := Record(scope["model"]); ok {
				if name, ok := model["display_name"].(string); ok && strings.TrimSpace(name) != "" {
					entry.displayName = strings.TrimSpace(name)
				}
			}
		}
		out = append(out, entry)
	}
	return out
}

// claudeBuildLimit mirrors the TS buildLimit: fixed window duration/label
// (not derived from payload seconds, unlike Codex), percent amount.
func claudeBuildLimit(id, label, windowID, windowLabel string, durationMs int64, bucket *claudeBucket, provider, tier string, shared bool) *Limit {
	if bucket == nil {
		return nil
	}
	amount := BuildPercentAmount(bucket.utilization)
	if amount.Used == nil {
		return nil
	}
	window := Window{ID: windowID, Label: windowLabel, DurationMs: &durationMs, ResetsAt: bucket.resetsAtMs}
	return &Limit{
		ID: id, Label: label,
		Scope:  Scope{Provider: provider, WindowID: windowID, Tier: tier, Shared: shared},
		Window: &window, Amount: amount,
		Status: StatusForFraction(amount.UsedFraction, nil, nil),
	}
}

func claudeSlugify(name string) string {
	var b strings.Builder
	prevDash := true
	for _, r := range strings.ToLower(name) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			prevDash = false
		} else if !prevDash {
			b.WriteRune('-')
			prevDash = true
		}
	}
	return strings.Trim(b.String(), "-")
}

func parseDollarAmount(amountMinor, exponent any, currency any) *float64 {
	amt, ok := Number(amountMinor)
	if !ok || amt < 0 || math.Trunc(amt) != amt {
		return nil
	}
	exp, ok := Number(exponent)
	if !ok || exp < 0 || math.Trunc(exp) != exp {
		return nil
	}
	if currency != nil {
		s, ok := currency.(string)
		if !ok || strings.ToUpper(s) != "USD" {
			return nil
		}
	}
	divisor := math.Pow(10, exp)
	if math.IsInf(divisor, 0) || divisor == 0 {
		return nil
	}
	dollars := amt / divisor
	if !isFinite(dollars) {
		return nil
	}
	return &dollars
}

func isFinite(f float64) bool {
	return !math.IsNaN(f) && !math.IsInf(f, 0)
}

type dollarBudget struct {
	used  float64
	limit *float64
}

func parseSpendExtra(v any) *dollarBudget {
	m, ok := Record(v)
	if !ok {
		return nil
	}
	if enabled, ok := m["enabled"].(bool); !ok || !enabled {
		return nil
	}
	if _, hasLimit := m["limit"]; !hasLimit {
		return nil
	}
	usedRec, ok := Record(m["used"])
	if !ok {
		return nil
	}
	used := parseDollarAmount(usedRec["amount_minor"], usedRec["exponent"], usedRec["currency"])
	if used == nil {
		return nil
	}
	if m["limit"] == nil {
		return &dollarBudget{used: *used}
	}
	limitRec, ok := Record(m["limit"])
	if !ok {
		return nil
	}
	limit := parseDollarAmount(limitRec["amount_minor"], limitRec["exponent"], limitRec["currency"])
	if limit == nil || *limit <= 0 {
		return nil
	}
	return &dollarBudget{used: *used, limit: limit}
}

func parseLegacyExtra(v any) *dollarBudget {
	m, ok := Record(v)
	if !ok {
		return nil
	}
	if enabled, ok := m["is_enabled"].(bool); !ok || !enabled {
		return nil
	}
	if _, hasLimit := m["monthly_limit"]; !hasLimit {
		return nil
	}
	decimalPlaces := any(2.0)
	if dp, ok := m["decimal_places"]; ok {
		decimalPlaces = dp
	}
	used := parseDollarAmount(m["used_credits"], decimalPlaces, m["currency"])
	if used == nil {
		return nil
	}
	if m["monthly_limit"] == nil {
		return &dollarBudget{used: *used}
	}
	limit := parseDollarAmount(m["monthly_limit"], decimalPlaces, m["currency"])
	if limit == nil || *limit <= 0 {
		return nil
	}
	return &dollarBudget{used: *used, limit: limit}
}

func claudeBuildExtraLimit(payload map[string]any, provider string) *Limit {
	spend, hasSpend := payload["spend"]
	var parsed *dollarBudget
	if !hasSpend || spend == nil {
		parsed = parseLegacyExtra(payload["extra_usage"])
	} else {
		parsed = parseSpendExtra(spend)
	}
	if parsed == nil {
		return nil
	}
	if parsed.limit == nil {
		return &Limit{
			ID: "claude:extra", Label: "Claude Extra Usage",
			Scope:  Scope{Provider: provider, WindowID: "extra"},
			Amount: Amount{Used: &parsed.used, Unit: "usd"},
		}
	}
	remaining := math.Max(0, *parsed.limit-parsed.used)
	usedFraction := parsed.used / *parsed.limit
	remainingFraction := remaining / *parsed.limit
	if !isFinite(usedFraction) || !isFinite(remainingFraction) {
		return nil
	}
	status := "exhausted"
	if parsed.used < *parsed.limit {
		status = StatusForFraction(&usedFraction, nil, nil)
	}
	return &Limit{
		ID: "claude:extra", Label: "Claude Extra Usage",
		Scope: Scope{Provider: provider, WindowID: "extra"},
		Amount: Amount{
			Used: &parsed.used, Limit: parsed.limit, Remaining: &remaining,
			UsedFraction: &usedFraction, RemainingFraction: &remainingFraction, Unit: "usd",
		},
		Status: status,
	}
}

// ParseClaudePayload converts a /usage payload to a Report.
func ParseClaudePayload(payload any, provider, accountID, email, endpoint string, nowMs int64) *Report {
	m, ok := Record(payload)
	if !ok {
		return nil
	}
	entries := parseClaudeLimitEntries(m["limits"])

	fiveHour := parseClaudeBucket(m["five_hour"])
	if fiveHour == nil {
		for _, e := range entries {
			if e.kind == "session" {
				fiveHour = e.bucket
				break
			}
		}
	}
	sevenDay := parseClaudeBucket(m["seven_day"])
	if sevenDay == nil {
		for _, e := range entries {
			if e.kind == "weekly_all" {
				sevenDay = e.bucket
				break
			}
		}
	}
	sevenDayOpus := parseClaudeBucket(m["seven_day_opus"])
	sevenDaySonnet := parseClaudeBucket(m["seven_day_sonnet"])

	seenSlugs := map[string]bool{}
	var scoped []Limit
	for _, e := range entries {
		if e.kind != "weekly_scoped" || e.displayName == "" {
			continue
		}
		slug := claudeSlugify(e.displayName)
		if slug == "" || seenSlugs[slug] {
			continue
		}
		seenSlugs[slug] = true
		if limit := claudeBuildLimit("claude:7d:"+slug, "Claude 7 Day ("+e.displayName+")", "7d", "7 Day", weekMs, e.bucket, provider, slug, false); limit != nil {
			scoped = append(scoped, *limit)
		}
	}

	var limits []Limit
	if l := claudeBuildLimit("claude:5h", "Claude 5 Hour", "5h", "5 Hour", 5*hourMs, fiveHour, provider, "", true); l != nil {
		limits = append(limits, *l)
	}
	if l := claudeBuildLimit("claude:7d", "Claude 7 Day", "7d", "7 Day", weekMs, sevenDay, provider, "", true); l != nil {
		limits = append(limits, *l)
	}
	if l := claudeBuildLimit("claude:7d:opus", "Claude 7 Day (Opus)", "7d", "7 Day", weekMs, sevenDayOpus, provider, "opus", false); l != nil {
		limits = append(limits, *l)
	}
	if l := claudeBuildLimit("claude:7d:sonnet", "Claude 7 Day (Sonnet)", "7d", "7 Day", weekMs, sevenDaySonnet, provider, "sonnet", false); l != nil {
		limits = append(limits, *l)
	}
	limits = append(limits, scoped...)
	if l := claudeBuildExtraLimit(m, provider); l != nil {
		limits = append(limits, *l)
	}

	if len(limits) == 0 {
		return nil
	}
	metadata := map[string]any{"endpoint": endpoint}
	if accountID != "" {
		metadata["accountId"] = accountID
	}
	if email != "" {
		metadata["email"] = email
	}
	return &Report{Provider: provider, FetchedAt: nowMs, Limits: limits, Metadata: metadata, Raw: payload}
}

// FetchClaudeUsage GETs the Claude /usage endpoint and parses the report.
// Returns (nil, nil) when not logged in, the token is expired, or the
// upstream call fails — matching the TS provider's null contract.
func FetchClaudeUsage(ctx context.Context, client *http.Client, baseURL, accessToken, email string, expiresAtMs int64) (*Report, error) {
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
	endpoint := normalizeClaudeUsageBaseURL(baseURL) + "/usage"
	req, err := http.NewRequestWithContext(ctx, "GET", endpoint, nil)
	if err != nil {
		return nil, nil
	}
	for k, v := range claudeHeaders(accessToken) {
		req.Header.Set(k, v)
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
	return ParseClaudePayload(payload, "claude", "", email, endpoint, nowMs), nil
}
