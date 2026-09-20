// Codex model discovery. Port of the codex branch in
// apps/server/agent/src/commands/provider-registry.ts.
package codex

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// DiscoveredModel mirrors the provider-registry discovered shape.
type DiscoveredModel struct {
	ID              string   `json:"id"`
	Provider        string   `json:"provider"`
	ContextWindow   int      `json:"contextWindow"`
	SupportsImages  bool     `json:"supportsImages,omitempty"`
	ThinkingLevels  []string `json:"supportedThinkingLevels,omitempty"`
	DefaultThinking string   `json:"defaultThinkingLevel,omitempty"`
}

// CodexThinkingLevels mirrors CODEX_THINKING_LEVELS in TS.
var CodexThinkingLevels = []string{"none", "low", "medium", "high", "xhigh", "max"}

const defaultContextWindow = 272_000

// FetchModels lists Codex models with auth headers. Returns nil slice on
// non-OK status so callers can fall back to static models.
func FetchModels(ctx context.Context, client *http.Client, baseURL string, cred ParsedCredential) ([]DiscoveredModel, error) {
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	req, err := http.NewRequestWithContext(ctx, "GET", ModelsURL(baseURL), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+cred.AccessToken)
	req.Header.Set("chatgpt-account-id", cred.AccountID)
	req.Header.Set("OpenAI-Beta", "responses=experimental")
	req.Header.Set("originator", "pi")
	req.Header.Set("version", ClientVersion)
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("Codex models request failed (%d %s)", resp.StatusCode, resp.Status)
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return nil, err
	}
	var payload struct {
		Models []struct {
			Slug            string   `json:"slug"`
			ID              string   `json:"id"`
			ContextWindow   *int     `json:"context_window"`
			InputModalities []string `json:"input_modalities"`
		} `json:"models"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, fmt.Errorf("decode Codex models: %w", err)
	}
	var out []DiscoveredModel
	for _, entry := range payload.Models {
		id := entry.Slug
		if id == "" {
			id = entry.ID
		}
		if id == "" {
			continue
		}
		cw := defaultContextWindow
		if entry.ContextWindow != nil && *entry.ContextWindow > 0 {
			cw = *entry.ContextWindow
		}
		supportsImages := false
		for _, m := range entry.InputModalities {
			if m == "image" {
				supportsImages = true
				break
			}
		}
		out = append(out, DiscoveredModel{
			ID:              id,
			Provider:        "codex",
			ContextWindow:   cw,
			SupportsImages:  supportsImages,
			ThinkingLevels:  CodexThinkingLevels,
			DefaultThinking: "low",
		})
	}
	return out, nil
}
