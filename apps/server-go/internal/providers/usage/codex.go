// Codex quota fetcher. Port of providers/src/usage/openai-codex.ts:
// GET {base}/wham/usage with the subscription token, parsed into a
// UsageReport with primary/secondary/additional limits.
package usage

import (
	"context"
	"encoding/json"
	"io"
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

// times converts parsed fields to the shared WindowTimes shape.
func (w *codexWindow) times() WindowTimes {
	return WindowTimes{
		LimitWindowSeconds: w.limitWindowSeconds,
		ResetAt:            w.resetAt,
		ResetAfterSeconds:  w.resetAfterSeconds,
	}
}

func parseCodexWindow(payload any) *codexWindow {
	m, ok := Record(payload)
	if !ok {
		return nil
	}
	w := &codexWindow{}
	if v, ok := Number(m["used_percent"]); ok {
		w.usedPercent = &v
	}
	if v, ok := Number(m["limit_window_seconds"]); ok {
		w.limitWindowSeconds = &v
	}
	if v, ok := Number(m["reset_after_seconds"]); ok {
		w.resetAfterSeconds = &v
	}
	if v, ok := Number(m["reset_at"]); ok {
		w.resetAt = &v
	}
	if w.usedPercent == nil && w.limitWindowSeconds == nil && w.resetAfterSeconds == nil && w.resetAt == nil {
		return nil
	}
	return w
}

func parseCodexAdditional(payload any) *codexAdditional {
	m, ok := Record(payload)
	if !ok {
		return nil
	}
	rl, ok := Record(m["rate_limit"])
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
	if b, ok := Bool(rl["allowed"]); ok {
		out.allowed = b
	}
	if b, ok := Bool(rl["limit_reached"]); ok {
		out.limitReached = b
	}
	out.primary = parseCodexWindow(rl["primary_window"])
	out.secondary = parseCodexWindow(rl["secondary_window"])
	if out.primary == nil && out.secondary == nil && out.allowed == nil && out.limitReached == nil {
		return nil
	}
	return out
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
	window := BuildWindow(key, w.times(), nowMs)
	amount := BuildPercentAmount(w.usedPercent)
	return Limit{
		ID: "codex:" + key, Label: window.Label,
		Scope: Scope{Provider: "codex", WindowID: window.ID, Shared: true},
		Window: &window, Amount: amount,
		Status: StatusForFraction(amount.UsedFraction, allowed, limitReached),
	}
}

func additionalLimit(key, slug, displayName string, w *codexWindow, accountID, limitName, meteredFeature string, allowed, limitReached *bool, nowMs int64) Limit {
	window := BuildWindow(key, w.times(), nowMs)
	amount := BuildPercentAmount(w.usedPercent)
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
		Status: StatusForFraction(amount.UsedFraction, allowed, limitReached),
	}
}

// ParseCodexPayload converts a wham/usage payload to a Report. Returns nil
// when the payload carries no usable limit data.
func ParseCodexPayload(payload any, accountID, email string, nowMs int64) *Report {
	m, ok := Record(payload)
	if !ok {
		return nil
	}
	var planType string
	if s, ok := m["plan_type"].(string); ok {
		planType = s
	}
	rl, _ := Record(m["rate_limit"])
	var allowed, limitReached *bool
	if rl != nil {
		allowed, _ = Bool(rl["allowed"])
		limitReached, _ = Bool(rl["limit_reached"])
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
	if block, ok := Record(m["rate_limit_reset_credits"]); ok {
		if n, ok := Number(block["available_count"]); ok {
			count := int(n)
			if count < 0 {
				count = 0
			}
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
			if auth, ok := Record(profile["https://api.openai.com/auth"]); ok {
				if id, ok := auth["chatgpt_account_id"].(string); ok {
					accountID = id
				}
			}
			if email == "" {
				if prof, ok := Record(profile["https://api.openai.com/profile"]); ok {
					if e, ok := prof["email"].(string); ok {
						email = strings.TrimSpace(strings.ToLower(e))
					}
				}
			}
		}
	}
	return ParseCodexPayload(payload, accountID, email, nowMs), nil
}
