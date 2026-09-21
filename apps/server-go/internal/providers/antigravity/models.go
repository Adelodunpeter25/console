// Antigravity model discovery. Port of the antigravity branch in
// apps/server/agent/src/commands/provider-registry.ts (seed) and
// apps/server/providers/src/discovery/fetch-models.ts (live list).
package antigravity

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/types"
)

// availableModelIDs mirrors AVAILABLE_MODELS.
var availableModelIDs = []string{
	"claude-opus-4-6-thinking",
	"claude-sonnet-4-6",
	"gemini-3.1-pro-high",
	"gemini-3.1-pro-low",
	"gemini-3-flash",
	"gemini-3-flash-agent",
	"gemini-3.5-flash-low",
	"gpt-oss-120b-medium",
}

// DefaultModels mirrors DEFAULT_ANTIGRAVITY_MODELS: offline seed refreshed
// from the live list after login.
func DefaultModels() []types.Model {
	models := make([]types.Model, 0, len(availableModelIDs))
	for _, id := range availableModelIDs {
		contextWindow := 1_048_576
		switch {
		case strings.HasPrefix(id, "claude-"):
			contextWindow = 250_000
		case strings.HasPrefix(id, "gpt-oss-"):
			contextWindow = 131_072
		}
		model := types.Model{ID: id, Provider: "antigravity", ContextWindow: contextWindow}
		if strings.HasPrefix(id, "gemini-") || strings.HasPrefix(id, "claude-") {
			model.ThinkingLevels = GeminiThinkingLevels
			model.DefaultThinking = "low"
		}
		models = append(models, model)
	}
	return models
}

var discoveryDenylist = map[string]bool{"chat_20706": true, "chat_23310": true}

type discoveredAPIModel struct {
	SupportsImages   *bool `json:"supportsImages"`
	SupportsThinking *bool `json:"supportsThinking"`
	MaxTokens        *int  `json:"maxTokens"`
	IsInternal       *bool `json:"isInternal"`
}

// FetchModels lists Antigravity models via /v1internal:fetchAvailableModels.
// Returns nil slice on failure so callers fall back to the seed.
func FetchModels(ctx context.Context, client *http.Client, baseURL, accessToken string) ([]types.Model, error) {
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	if baseURL == "" {
		baseURL = BaseURL
	}
	url := strings.TrimRight(baseURL, "/") + "/v1internal:fetchAvailableModels"
	req, err := http.NewRequestWithContext(ctx, "POST", url, strings.NewReader("{}"))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", UserAgent())

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, nil
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return nil, err
	}
	var payload struct {
		Models map[string]discoveredAPIModel `json:"models"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil || payload.Models == nil {
		return nil, nil
	}

	var out []types.Model
	for id, meta := range payload.Models {
		if discoveryDenylist[id] {
			continue
		}
		if meta.IsInternal != nil && *meta.IsInternal {
			continue
		}
		contextWindow := 200_000
		if meta.MaxTokens != nil && *meta.MaxTokens > 0 {
			contextWindow = *meta.MaxTokens
		}
		supportsThinking := strings.HasPrefix(id, "gemini-")
		if meta.SupportsThinking != nil {
			supportsThinking = *meta.SupportsThinking
		}
		model := types.Model{ID: id, Provider: "antigravity", ContextWindow: contextWindow}
		if meta.SupportsImages != nil {
			model.SupportsImages = *meta.SupportsImages
		}
		if supportsThinking {
			model.ThinkingLevels = GeminiThinkingLevels
			model.DefaultThinking = "low"
		}
		out = append(out, model)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}
