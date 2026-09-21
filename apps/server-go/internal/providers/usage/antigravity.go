// Antigravity quota fetcher. Port of
// providers/src/usage/google-antigravity.ts: POST
// /v1internal:fetchAvailableModels with {project}, normalizing the many
// quotaInfo/quotaInfos/dailyQuotaInfo*/weeklyQuotaInfo*/quotaInfoByTier/
// quotaInfoByWindow shapes into a deduped UsageReport.
package usage

import (
	"context"
	"encoding/json"
	"io"
	"math"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/providers/antigravity"
)

const antigravityFetchModelsPath = "/v1internal:fetchAvailableModels"
const antigravitySandboxBaseURL = "https://daily-cloudcode-pa.sandbox.googleapis.com"
const dayMs = int64(24 * 60 * 60 * 1000)
const weekMsAG = int64(7 * 24 * 60 * 60 * 1000)

// agQuotaInfo is one normalized quota entry.
type agQuotaInfo struct {
	remainingFraction *float64
	resetTime         string
	tier              string
	windowID          string
	windowLabel       string
	apiProvider       string
	modelProvider     string
}

func agParseISOMs(s string) *int64 {
	if s == "" {
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

type agWindowDescriptor struct {
	id, label  string
	durationMs *int64
}

func agClassifyWindow(id, label string) *agWindowDescriptor {
	source := strings.ToLower(id + " " + label)
	switch {
	case strings.Contains(source, "week") || strings.Contains(source, "7d") || strings.Contains(source, "7 day") || strings.Contains(source, "7-day") || strings.Contains(source, "7_day"):
		w := weekMsAG
		return &agWindowDescriptor{id: "weekly", label: "Weekly", durationMs: &w}
	case strings.Contains(source, "day") || strings.Contains(source, "daily") || strings.Contains(source, "24h"):
		d := dayMs
		return &agWindowDescriptor{id: "daily", label: "Daily", durationMs: &d}
	}
	if id != "" || label != "" {
		outID, outLabel := id, label
		if outID == "" {
			outID = label
		}
		if outLabel == "" {
			outLabel = id
		}
		return &agWindowDescriptor{id: outID, label: outLabel}
	}
	return nil
}

func agInferWindowFromReset(resetAt *int64, nowMs int64) agWindowDescriptor {
	if resetAt != nil && *resetAt-nowMs > dayMs {
		w := weekMsAG
		return agWindowDescriptor{id: "weekly", label: "Weekly", durationMs: &w}
	}
	d := dayMs
	return agWindowDescriptor{id: "daily", label: "Daily", durationMs: &d}
}

func agQuotaInferenceKey(info agQuotaInfo) string {
	return info.modelProvider + "|" + info.apiProvider + "|" + info.tier
}

// agInferWindowDescriptors groups quota infos lacking an explicit window by
// (modelProvider, apiProvider, tier), then classifies each by its reset
// time relative to the group's other reset times, mirroring
// inferWindowDescriptors.
func agInferWindowDescriptors(infos []*agQuotaInfo, nowMs int64) map[*agQuotaInfo]agWindowDescriptor {
	out := map[*agQuotaInfo]agWindowDescriptor{}
	type entry struct {
		info    *agQuotaInfo
		resetAt *int64
	}
	groups := map[string][]entry{}

	for _, info := range infos {
		if d := agClassifyWindow(info.windowID, info.windowLabel); d != nil {
			out[info] = *d
			continue
		}
		key := agQuotaInferenceKey(*info)
		groups[key] = append(groups[key], entry{info: info, resetAt: agParseISOMs(info.resetTime)})
	}

	for _, group := range groups {
		seen := map[int64]bool{}
		var resetTimes []int64
		for _, e := range group {
			if e.resetAt != nil && !seen[*e.resetAt] {
				seen[*e.resetAt] = true
				resetTimes = append(resetTimes, *e.resetAt)
			}
		}
		sort.Slice(resetTimes, func(i, j int) bool { return resetTimes[i] < resetTimes[j] })
		var latestReset *int64
		if len(resetTimes) > 1 {
			latestReset = &resetTimes[len(resetTimes)-1]
		}
		for _, e := range group {
			if latestReset != nil && e.resetAt != nil && *e.resetAt == *latestReset {
				w := weekMsAG
				out[e.info] = agWindowDescriptor{id: "weekly", label: "Weekly", durationMs: &w}
			} else {
				out[e.info] = agInferWindowFromReset(e.resetAt, nowMs)
			}
		}
	}
	return out
}

func agClampFraction(v *float64) *float64 {
	if v == nil {
		return nil
	}
	f := *v
	if math.IsNaN(f) || math.IsInf(f, 0) {
		return nil
	}
	if f < 0 {
		f = 0
	}
	if f > 1 {
		f = 1
	}
	return &f
}

func agUsageStatus(remainingFraction *float64) string {
	if remainingFraction == nil {
		return "unknown"
	}
	if *remainingFraction <= 0 {
		return "exhausted"
	}
	if *remainingFraction <= 0.5 {
		return "warning"
	}
	return "ok"
}

func agParseWindow(info agQuotaInfo, descriptor *agWindowDescriptor) *Window {
	resetAt := agParseISOMs(info.resetTime)
	if descriptor == nil && resetAt == nil {
		return nil
	}
	w := &Window{ResetsAt: resetAt}
	if descriptor != nil {
		w.ID = descriptor.id
		w.Label = descriptor.label
		w.DurationMs = descriptor.durationMs
	} else {
		w.ID = info.windowID
		if w.ID == "" {
			w.ID = "default"
		}
	}
	if w.Label == "" {
		w.Label = info.windowLabel
	}
	if w.Label == "" {
		w.Label = "Default"
	}
	return w
}

func agBuildAmount(info agQuotaInfo) Amount {
	remainingFraction := agClampFraction(info.remainingFraction)
	if remainingFraction == nil && info.resetTime != "" {
		zero := 0.0
		remainingFraction = &zero
	}
	amount := Amount{Unit: "percent"}
	if remainingFraction == nil {
		return amount
	}
	usedFraction := 1 - *remainingFraction
	remaining := *remainingFraction * 100
	used := usedFraction * 100
	limit := 100.0
	amount.RemainingFraction = remainingFraction
	amount.UsedFraction = &usedFraction
	amount.Remaining = &remaining
	amount.Used = &used
	amount.Limit = &limit
	return amount
}

func agFormatCounterName(info agQuotaInfo) string {
	key := info.modelProvider
	if key == "" {
		key = info.apiProvider
	}
	switch key {
	case "MODEL_PROVIDER_ANTHROPIC", "API_PROVIDER_ANTHROPIC_VERTEX":
		return "Anthropic"
	case "MODEL_PROVIDER_GOOGLE", "API_PROVIDER_GOOGLE_GEMINI":
		return "Google"
	case "MODEL_PROVIDER_OPENAI", "API_PROVIDER_OPENAI_VERTEX":
		return "OpenAI"
	default:
		return ""
	}
}

// agSingleOrList reads a field that may be a single object or an array of
// objects, mirroring the TS `AntigravityQuotaInfo | AntigravityQuotaInfo[]`.
func agSingleOrList(v any) []map[string]any {
	switch val := v.(type) {
	case map[string]any:
		return []map[string]any{val}
	case []any:
		out := make([]map[string]any, 0, len(val))
		for _, item := range val {
			if m, ok := item.(map[string]any); ok {
				out = append(out, m)
			}
		}
		return out
	default:
		return nil
	}
}

func agParseQuotaInfo(m map[string]any, base agQuotaInfo, tier string, descriptor *agWindowDescriptor) *agQuotaInfo {
	info := base
	if v, ok := m["remainingFraction"]; ok {
		if f, ok := Number(v); ok {
			info.remainingFraction = &f
		}
	}
	if s, ok := m["resetTime"].(string); ok {
		info.resetTime = s
	}
	if s, ok := m["tier"].(string); ok {
		info.tier = s
	}
	if s, ok := m["windowId"].(string); ok {
		info.windowID = s
	}
	if s, ok := m["windowLabel"].(string); ok {
		info.windowLabel = s
	}
	if s, ok := m["apiProvider"].(string); ok {
		info.apiProvider = s
	}
	if s, ok := m["modelProvider"].(string); ok {
		info.modelProvider = s
	}
	if tier != "" && info.tier == "" {
		info.tier = tier
	}
	if descriptor != nil {
		if info.windowID == "" {
			info.windowID = descriptor.id
		}
		if info.windowLabel == "" {
			info.windowLabel = descriptor.label
		}
	}
	return &info
}

// agNormalizeQuotaInfos flattens every shape a model entry can report quota
// under into a single list, mirroring normalizeQuotaInfos.
func agNormalizeQuotaInfos(modelInfo map[string]any) []*agQuotaInfo {
	base := agQuotaInfo{}
	if s, ok := modelInfo["apiProvider"].(string); ok {
		base.apiProvider = s
	}
	if s, ok := modelInfo["modelProvider"].(string); ok {
		base.modelProvider = s
	}

	var out []*agQuotaInfo
	addValue := func(v any, tier string, descriptor *agWindowDescriptor) {
		for _, m := range agSingleOrList(v) {
			out = append(out, agParseQuotaInfo(m, base, tier, descriptor))
		}
	}

	addValue(modelInfo["quotaInfo"], "", nil)
	addValue(modelInfo["quotaInfos"], "", nil)
	addValue(modelInfo["dailyQuotaInfo"], "", agClassifyWindow("daily", "Daily"))
	addValue(modelInfo["dailyQuotaInfos"], "", agClassifyWindow("daily", "Daily"))
	addValue(modelInfo["weeklyQuotaInfo"], "", agClassifyWindow("weekly", "Weekly"))
	addValue(modelInfo["weeklyQuotaInfos"], "", agClassifyWindow("weekly", "Weekly"))

	if byTier, ok := modelInfo["quotaInfoByTier"].(map[string]any); ok {
		for tier, v := range byTier {
			addValue(v, tier, nil)
		}
	}
	addWindowMap := func(v any) {
		if byWindow, ok := v.(map[string]any); ok {
			for windowID, wv := range byWindow {
				addValue(wv, "", agClassifyWindow(windowID, ""))
			}
		}
	}
	addWindowMap(modelInfo["quotaInfoByWindow"])
	addWindowMap(modelInfo["quotaInfosByWindow"])

	return out
}

func agIsTransientStatus(status int) bool {
	return status == 429 || status >= 500
}

// ParseAntigravityPayload converts a fetchAvailableModels payload (fetched
// with {project}) into a deduped Report of quota limits.
func ParseAntigravityPayload(payload any, provider, accountID, email, endpoint string, nowMs int64) *Report {
	m, ok := Record(payload)
	if !ok {
		return nil
	}
	models, ok := Record(m["models"])
	if !ok {
		return &Report{Provider: provider, FetchedAt: nowMs, Limits: []Limit{}, Metadata: map[string]any{"endpoint": endpoint}, Raw: payload}
	}

	type dedupEntry struct {
		amount      Amount
		window      *Window
		tier        string
		tierKey     string
		windowID    string
		counterName string
		counterKey  string
	}
	deduped := map[string]*dedupEntry{}

	for _, rawModelInfo := range models {
		modelInfo, ok := Record(rawModelInfo)
		if !ok {
			continue
		}
		infos := agNormalizeQuotaInfos(modelInfo)
		descriptors := agInferWindowDescriptors(infos, nowMs)
		for _, info := range infos {
			amount := agBuildAmount(*info)
			var descriptor *agWindowDescriptor
			if d, ok := descriptors[info]; ok {
				descriptor = &d
			}
			window := agParseWindow(*info, descriptor)

			tierKey := strings.ToLower(info.tier)
			if tierKey == "" {
				tierKey = "default"
			}
			counterName := agFormatCounterName(*info)
			counterKey := strings.ToLower(counterName)
			if counterKey == "" {
				counterKey = "default"
			}
			windowID := "default"
			if window != nil {
				windowID = window.ID
			} else if info.windowID != "" {
				windowID = info.windowID
			}
			key := counterKey + "|" + tierKey + "|" + windowID

			existing, has := deduped[key]
			if !has {
				deduped[key] = &dedupEntry{
					amount: amount, window: window, tier: info.tier,
					tierKey: tierKey, windowID: windowID, counterName: counterName, counterKey: counterKey,
				}
				continue
			}

			eFrac, cFrac := existing.amount.RemainingFraction, amount.RemainingFraction
			bestAmount := existing.amount
			bestTier := existing.tier
			if tier := info.tier; tier != "" {
				bestTier = existing.tier
				if bestTier == "" {
					bestTier = tier
				}
			}
			if eFrac == nil && cFrac != nil {
				bestAmount = amount
				if info.tier != "" {
					bestTier = info.tier
				}
			} else if eFrac != nil && cFrac != nil && *cFrac < *eFrac {
				bestAmount = amount
				if info.tier != "" {
					bestTier = info.tier
				}
			}
			bestWindow := existing.window
			if bestWindow == nil || bestWindow.ResetsAt == nil {
				if window != nil {
					bestWindow = window
				}
			}
			existing.amount, existing.window, existing.tier = bestAmount, bestWindow, bestTier
		}
	}

	limits := make([]Limit, 0, len(deduped))
	for _, entry := range deduped {
		label := "Usage"
		if entry.counterName != "" {
			label = "Usage (" + entry.counterName + ")"
		}
		limits = append(limits, Limit{
			ID: provider + ":" + entry.counterKey + ":" + entry.tierKey + ":" + entry.windowID, Label: label,
			Scope:  Scope{Provider: provider, AccountID: accountID, Tier: entry.tier, WindowID: entry.windowID},
			Window: entry.window, Amount: entry.amount,
			Status: agUsageStatus(entry.amount.RemainingFraction),
		})
	}
	sort.SliceStable(limits, func(i, j int) bool {
		ri, rj := 1.0, 1.0
		if limits[i].Amount.RemainingFraction != nil {
			ri = *limits[i].Amount.RemainingFraction
		}
		if limits[j].Amount.RemainingFraction != nil {
			rj = *limits[j].Amount.RemainingFraction
		}
		return ri < rj
	})

	metadata := map[string]any{"endpoint": endpoint}
	if email != "" {
		metadata["email"] = email
	}
	if accountID != "" {
		metadata["accountId"] = accountID
	}
	return &Report{Provider: provider, FetchedAt: nowMs, Limits: limits, Metadata: metadata, Raw: payload}
}

// FetchAntigravityUsage POSTs {project} to /v1internal:fetchAvailableModels
// and parses the quota report. Tries the sandbox endpoint as a fallback on
// transient (429/5xx) failures, mirroring fetchAntigravityUsage.
func FetchAntigravityUsage(ctx context.Context, client *http.Client, baseURL, accessToken, projectID, accountID, email string, expiresAtMs int64) (*Report, error) {
	if accessToken == "" || projectID == "" {
		return nil, nil
	}
	nowMs := time.Now().UnixMilli()
	if expiresAtMs > 0 && expiresAtMs <= nowMs {
		return nil, nil
	}
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}

	var endpoints []string
	if baseURL != "" {
		endpoints = []string{strings.TrimRight(baseURL, "/")}
	} else {
		endpoints = []string{antigravity.BaseURL, antigravitySandboxBaseURL}
	}

	body, _ := json.Marshal(map[string]any{"project": projectID})
	successfulEndpoint := antigravity.BaseURL
	var resp *http.Response
	var err error
	for i, endpoint := range endpoints {
		url := endpoint + antigravityFetchModelsPath
		req, reqErr := http.NewRequestWithContext(ctx, "POST", url, strings.NewReader(string(body)))
		if reqErr != nil {
			return nil, nil
		}
		req.Header.Set("Authorization", "Bearer "+accessToken)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("User-Agent", antigravity.UserAgent())

		resp, err = client.Do(req)
		if err != nil {
			if i == len(endpoints)-1 {
				return nil, nil
			}
			continue
		}
		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			successfulEndpoint = endpoint
			break
		}
		transient := agIsTransientStatus(resp.StatusCode)
		resp.Body.Close()
		resp = nil
		if !transient {
			break
		}
	}
	if resp == nil {
		return nil, nil
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, nil
	}

	raw, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return nil, nil
	}
	var payload any
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, nil
	}
	return ParseAntigravityPayload(payload, "antigravity", accountID, email, successfulEndpoint+antigravityFetchModelsPath, nowMs), nil
}
