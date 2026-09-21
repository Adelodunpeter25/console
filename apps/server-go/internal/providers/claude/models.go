// Claude model discovery. Port of the claude branch in
// apps/server/agent/src/commands/provider-registry.ts (seed) and
// apps/server/providers/src/claude/discovery.ts (live list).
package claude

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/types"
)

// DiscoveredModel is the provider-registry discovered shape (alias of the
// shared catalog model type).
type DiscoveredModel = types.Model

// DefaultModels mirrors DEFAULT_CLAUDE_MODELS: offline seed refreshed from
// the live list after login. Context windows measured from /v1/models.
func DefaultModels() []DiscoveredModel {
	seed := []struct {
		id string
		cw int
	}{
		{"claude-sonnet-4-5", 1_000_000},
		{"claude-opus-4-6", 1_000_000},
		{"claude-sonnet-4-6", 1_000_000},
		{"claude-haiku-4-5", 200_000},
	}
	models := make([]DiscoveredModel, 0, len(seed))
	for _, s := range seed {
		models = append(models, DiscoveredModel{
			ID: s.id, Provider: "claude", ContextWindow: s.cw, SupportsImages: true,
			ThinkingLevels: ClaudeThinkingLevels, DefaultThinking: "low",
		})
	}
	return models
}

// FetchModels lists Claude models with the subscription OAuth token.
// Returns nil slice on non-OK status so callers fall back to the seed.
func FetchModels(ctx context.Context, client *http.Client, baseURL string, cred ParsedCredential) ([]DiscoveredModel, error) {
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	req, err := http.NewRequestWithContext(ctx, "GET", ModelsURL(baseURL), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+cred.AccessToken)
	req.Header.Set("anthropic-version", "2023-06-01")
	req.Header.Set("anthropic-beta", "oauth-2025-04-20")
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("Claude models request failed (%d %s)", resp.StatusCode, resp.Status)
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return nil, err
	}
	var payload struct {
		Data []struct {
			ID               string `json:"id"`
			MaxInputTokens   *int   `json:"max_input_tokens"`
			Capabilities     *struct {
				ImageInput *struct {
					Supported *bool `json:"supported"`
				} `json:"image_input"`
			} `json:"capabilities"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, fmt.Errorf("decode Claude models: %w", err)
	}
	var out []DiscoveredModel
	for _, entry := range payload.Data {
		if entry.ID == "" {
			continue
		}
		model := DiscoveredModel{
			ID: entry.ID, Provider: "claude",
			ThinkingLevels: ClaudeThinkingLevels, DefaultThinking: "low",
		}
		if entry.MaxInputTokens != nil && *entry.MaxInputTokens > 0 {
			model.ContextWindow = *entry.MaxInputTokens
		}
		if entry.Capabilities != nil && entry.Capabilities.ImageInput != nil &&
			entry.Capabilities.ImageInput.Supported != nil {
			model.SupportsImages = *entry.Capabilities.ImageInput.Supported
		}
		out = append(out, model)
	}
	return out, nil
}
