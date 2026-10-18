// OpenCode Go model discovery.
//
// Go's model-list endpoint is authoritative on *which* models exist but
// publishes nothing about them — every entry is exactly {id, object, created,
// owned_by}. Capabilities therefore come from models.dev (see metadata.go),
// and a model the catalogue does not cover still appears, carrying conservative
// defaults, rather than being hidden.
package opencode

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/types"
)

// modelsResponseLimit caps the discovery read. Go returns ~40 entries today, so
// this is generous while still bounding a runaway response.
const modelsResponseLimit = 4 << 20

// decodeModelIDs pulls the model ids out of a discovery response.
//
// Only the id is read. The endpoint publishes no capability fields, so decoding
// anything else would be inventing structure that is not there.
func decodeModelIDs(resp *http.Response) ([]string, error) {
	var payload struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, modelsResponseLimit)).Decode(&payload); err != nil {
		return nil, fmt.Errorf("decode OpenCode models: %w", err)
	}

	ids := make([]string, 0, len(payload.Data))
	seen := make(map[string]bool, len(payload.Data))
	for _, model := range payload.Data {
		// Blank and duplicate ids would otherwise become duplicate picker
		// entries; neither is worth failing the whole list over.
		if model.ID == "" || seen[model.ID] {
			continue
		}
		seen[model.ID] = true
		ids = append(ids, model.ID)
	}
	return ids, nil
}

// FetchModels lists the models the Go subscription currently exposes.
//
// Warms the models.dev index first so the returned entries carry real context
// windows, image support and effort levels. That fetch is best-effort: if it
// fails, the models still list with defaults rather than the call failing.
func FetchModels(ctx context.Context, client *http.Client, baseURL string) ([]types.Model, error) {
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, ModelsURL(baseURL), nil)
	if err != nil {
		return nil, err
	}
	// Fail before the network call if no key is configured, so an unconfigured
	// server reports the real problem rather than a 401 from upstream.
	if err := SetCommonHeaders(req, "application/json"); err != nil {
		return nil, err
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("OpenCode models request failed (%d %s)", resp.StatusCode, resp.Status)
	}

	ids, err := decodeModelIDs(resp)
	if err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		return nil, fmt.Errorf("OpenCode models response contained no models")
	}

	// Warm capabilities only once the request has succeeded, so an upstream
	// outage does not also trigger a 6 MB catalogue fetch.
	EnsureMetadata(ctx, client)

	out := make([]types.Model, 0, len(ids))
	for _, id := range ids {
		out = append(out, NewModel(id))
	}
	return out, nil
}

// ResolveModel returns the catalog entry for a single model id, using live
// metadata when available.
//
// Exists so model resolution does not have to synthesize a guess for a Go model
// it has never seen: this is what makes an unfamiliar id resolve to its true
// context window and effort vocabulary instead of a blanket 128k.
func ResolveModel(ctx context.Context, modelID string) (types.Model, bool) {
	if modelID == "" {
		return types.Model{}, false
	}
	EnsureMetadata(ctx, nil)
	return NewModel(modelID), true
}

// NewModel builds a catalog entry for one model id, taking every capability it
// can from the models.dev index.
//
// Exported because roles.InferThinkingLevels needs to build a full entry for an
// id it only has a name for, and duplicating this mapping in two places is how
// the capability table and the catalog drifted apart in the first place.
func NewModel(id string) types.Model {
	m := types.Model{
		ID:            id,
		Provider:      ProviderID,
		ContextWindow: DefaultContextWindow,
	}

	md, ok := MetadataFor(id)
	if !ok {
		// Not in the catalogue yet — one of the handful of models OpenCode
		// ships ahead of models.dev. Listed with the conservative default and
		// no level picker rather than dropped: the user can still pick it.
		return m
	}

	if md.ContextWindow > 0 {
		m.ContextWindow = md.ContextWindow
	}
	m.SupportsImages = md.SupportsImages
	if len(md.ThinkingLevels) > 0 {
		m.ThinkingLevels = md.ThinkingLevels
		m.DefaultThinking = DefaultThinkingFor(md.ThinkingLevels)
	}
	return m
}

// DefaultThinkingFor picks the effort a level-capable model should start at.
//
// The midpoint of the model's own vocabulary, which is always a member of it.
// A hardcoded "medium" is not: DeepSeek V4.1 Flash's levels are
// low/high/max, so defaulting to medium would ship a value the provider rejects
// on the very first run.
func DefaultThinkingFor(levels []string) string {
	if len(levels) == 0 {
		return ""
	}
	return levels[len(levels)/2]
}
