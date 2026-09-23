// OpenCode Zen model discovery.
package opencode

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/types"
)

// DefaultModels is the offline seed for catalog startup and discovery failure.
func DefaultModels() []types.Model {
	ids := []string{
		"big-pickle",
		"space-bunny-free",
		"mimo-v2.6-flash-free",
		"mimo-v2.5-free",
		"ling-3.0-flash-fin-free",
		"nemotron-3-ultra-free",
		"nemotron-3.5-lightning-free",
		"muse-spark-1.3-contributor-free",
		"muse-spark-1.2-contributor-free",
	}
	return modelsForIDs(ids)
}

// IsFreeModelID accepts the regular -free suffix and Zen's big-pickle special
// case. Unknown future free models are therefore discovered without a code
// change.
func IsFreeModelID(id string) bool {
	return strings.HasSuffix(id, "-free") || id == "big-pickle"
}

// FetchModels lists the currently available free models.
func FetchModels(ctx context.Context, client *http.Client, baseURL string) ([]types.Model, error) {
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, ModelsURL(baseURL), nil)
	if err != nil {
		return nil, err
	}
	SetCommonHeaders(req, "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("OpenCode models request failed (%d %s)", resp.StatusCode, resp.Status)
	}

	var payload struct {
		Data []struct {
			ID            string `json:"id"`
			ContextWindow any    `json:"context_window"`
			ContextLength any    `json:"context_length"`
		} `json:"data"`
	}
	decoder := json.NewDecoder(io.LimitReader(resp.Body, 4<<20))
	if err := decoder.Decode(&payload); err != nil {
		return nil, fmt.Errorf("decode OpenCode models: %w", err)
	}

	ids := make([]string, 0, len(payload.Data))
	contexts := make(map[string]int, len(payload.Data))
	for _, model := range payload.Data {
		if !IsFreeModelID(model.ID) {
			continue
		}
		ids = append(ids, model.ID)
		contexts[model.ID] = modelContextWindow(model.ContextWindow, model.ContextLength)
	}
	if len(ids) == 0 {
		return nil, fmt.Errorf("OpenCode models response contained no free models")
	}

	out := make([]types.Model, 0, len(ids))
	for _, id := range ids {
		contextWindow := contexts[id]
		if contextWindow <= 0 {
			contextWindow = DefaultContextWindow
		}
		out = append(out, types.Model{
			ID:            id,
			Provider:      "opencode",
			ContextWindow: contextWindow,
		})
	}
	return out, nil
}

func modelContextWindow(values ...any) int {
	for _, value := range values {
		switch n := value.(type) {
		case float64:
			if n > 0 {
				return int(n)
			}
		case json.Number:
			if parsed, err := n.Int64(); err == nil && parsed > 0 {
				return int(parsed)
			}
		}
	}
	return 0
}

func modelsForIDs(ids []string) []types.Model {
	out := make([]types.Model, 0, len(ids))
	for _, id := range ids {
		out = append(out, types.Model{
			ID:            id,
			Provider:      "opencode",
			ContextWindow: DefaultContextWindow,
		})
	}
	return out
}
