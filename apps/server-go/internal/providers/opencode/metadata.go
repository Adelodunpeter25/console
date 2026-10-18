// Model capability metadata sourced from models.dev.
//
// Go's own model-list endpoint is the authority on *which* models exist, but it
// publishes nothing about them: every entry is exactly {id, object, created,
// owned_by}. Context window, image support and effort levels all have to come
// from somewhere else.
//
// This used to be a hand-written table keyed by three model-family prefixes,
// which was wrong twice over. It covered 3 of Go's 37 models, and for the one
// family it did cover it guessed "low/medium/high/xhigh" for DeepSeek V4 while
// models.dev records that same model as "low/high/max" — a level the UI could
// offer but the provider would reject.
//
// models.dev is the same catalogue OpenCode's own client resolves capabilities
// against, so reading it keeps us aligned with upstream instead of encoding a
// stale guess. It is a single ~6 MB document covering 150+ providers; there is
// no per-provider endpoint (the ?provider= query parameter is ignored), so this
// decodes the top level as raw JSON and materializes only our own entry.
package opencode

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"slices"
	"sync"
	"time"
)

// ModelsDevEndpoint is the catalogue index. No credentials required.
//
// A var rather than a const so tests can point it at a stub, and so a
// self-hosted mirror is possible without a code change.
var ModelsDevEndpoint = "https://models.dev/api.json"

// modelsDevProvider is the key for OpenCode Go. It is a distinct entry from the
// plain "opencode" provider (the free-tier catalogue), which is exactly why Go
// models must be looked up here rather than under the old provider id.
const modelsDevProvider = "opencode-go"

const (
	// metadataCacheTTL keeps this to one fetch per day. Go reshuffles its
	// lineup regularly, but not fast enough to justify re-pulling 6 MB on every
	// settings page load, and the payload is also only read at startup in
	// practice.
	metadataCacheTTL = 24 * time.Hour

	metadataFetchTimeout = 30 * time.Second

	// metadataMaxBytes caps the read. The document is ~6 MB today; this leaves
	// room to grow without letting a bad response exhaust memory.
	metadataMaxBytes = 64 << 20
)

// ModelMetadata is what models.dev can tell us about one model, reduced to the
// fields the catalog actually carries.
type ModelMetadata struct {
	// ContextWindow is the advertised input limit in tokens.
	ContextWindow int
	// SupportsImages reports whether image input is accepted.
	SupportsImages bool
	// ThinkingLevels is the effort vocabulary this model accepts, in upstream
	// order. Empty means the model does no tunable reasoning — which is
	// different from a model whose levels we failed to look up, and callers
	// treat both as "no level offered".
	ThinkingLevels []string
	// DisplayName is the human-readable name, when published.
	DisplayName string
}

var (
	// metadataMu guards the index. Reads happen on every catalog render, so
	// this is an RWMutex rather than a plain one.
	metadataMu sync.RWMutex
	// metadataIndex maps model id -> capabilities.
	metadataIndex map[string]ModelMetadata
	// metadataFetchedAt is when the index was last replaced.
	metadataFetchedAt time.Time

	// metadataRefreshMu serializes refreshes. The model list is fetched on
	// every settings page open and several of those overlap, so without this
	// a cold start would fire one 6 MB download per concurrent request.
	metadataRefreshMu sync.Mutex
)

// EnsureMetadata refreshes the models.dev index if it is missing or stale.
//
// Failure is deliberately silent: this is an enrichment, not a requirement. Go's
// model list still works without it, models simply carry conservative defaults,
// and a provider that cannot reach models.dev must not fail because of it. A
// server on an offline network degrades exactly as it did before this existed.
func EnsureMetadata(ctx context.Context, client *http.Client) {
	if metadataFresh() {
		return
	}

	metadataRefreshMu.Lock()
	defer metadataRefreshMu.Unlock()

	// Re-check under the lock: a concurrent caller may have finished the
	// refresh while we were waiting for it.
	if metadataFresh() {
		return
	}

	index, err := fetchMetadata(ctx, client)
	if err != nil {
		// Keep whatever we had. A stale index is better than none — dropping
		// it would silently strip context windows and effort levels from
		// models the user can already see.
		return
	}

	metadataMu.Lock()
	metadataIndex = index
	metadataFetchedAt = time.Now()
	metadataMu.Unlock()
}

// metadataFresh reports whether the index can be served without a refetch.
func metadataFresh() bool {
	metadataMu.RLock()
	defer metadataMu.RUnlock()
	return len(metadataIndex) > 0 && time.Since(metadataFetchedAt) < metadataCacheTTL
}

// UseMetadata replaces the capability index outright and marks it fresh.
//
// Exists so tests get deterministic capabilities without reaching the network,
// and so a future offline snapshot can be installed the same way a fetched one
// is. Passing nil or an empty map clears the index, after which the next
// MetadataFor miss falls back to conservative defaults.
func UseMetadata(index map[string]ModelMetadata) {
	metadataMu.Lock()
	defer metadataMu.Unlock()
	if len(index) == 0 {
		metadataIndex = nil
		metadataFetchedAt = time.Time{}
		return
	}
	metadataIndex = index
	metadataFetchedAt = time.Now()
}

// MetadataFor returns the capabilities of one model, if known.
//
// Returns ok=false when the index is cold or the model is missing from it —
// which is the normal state for the handful of models OpenCode ships before
// models.dev catches up, and for any server that has not warmed the cache.
func MetadataFor(modelID string) (ModelMetadata, bool) {
	metadataMu.RLock()
	defer metadataMu.RUnlock()
	md, ok := metadataIndex[modelID]
	return md, ok
}

// modelsDevEntry is the slice of a models.dev model record we read. Fields we
// do not use (cost, release_date, provider, ...) are omitted on purpose: adding
// one should not silently change catalog behaviour.
type modelsDevEntry struct {
	Name       string `json:"name"`
	Attachment bool   `json:"attachment"`
	Reasoning  bool   `json:"reasoning"`
	// ReasoningOptions is a tagged list; we only consume the "effort" variant.
	//
	// Values is []any rather than []string on purpose. The field is a free-form
	// control list — a boolean toggle publishes [true], an effort scale
	// publishes ["low","high"] — so a string-typed slice makes the whole
	// document fail to parse over one unexpected entry, taking capabilities
	// away from every model at once.
	ReasoningOptions []struct {
		Type   string `json:"type"`
		Values []any  `json:"values"`
	} `json:"reasoning_options"`
	Modalities struct {
		Input []string `json:"input"`
	} `json:"modalities"`
	Limit struct {
		Context int `json:"context"`
	} `json:"limit"`
}

// fetchMetadata downloads the catalogue and returns the index for our provider.
func fetchMetadata(ctx context.Context, client *http.Client) (map[string]ModelMetadata, error) {
	if client == nil {
		client = &http.Client{Timeout: metadataFetchTimeout}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, ModelsDevEndpoint, nil)
	if err != nil {
		return nil, err
	}
	// Deliberately not SetCommonHeaders: that attaches the provider's API key,
	// and models.dev is a third party that must never see it.
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", UserAgent())

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("models.dev request failed (%d %s)", resp.StatusCode, resp.Status)
	}

	// Decode the top level as raw messages and expand only our provider.
	// Materializing all 150+ provider entries to read one would allocate every
	// model record in the catalogue for nothing.
	var raw map[string]json.RawMessage
	if err := json.NewDecoder(io.LimitReader(resp.Body, metadataMaxBytes)).Decode(&raw); err != nil {
		return nil, fmt.Errorf("decode models.dev: %w", err)
	}

	providerRaw, ok := raw[modelsDevProvider]
	if !ok {
		return nil, fmt.Errorf("models.dev has no %q entry", modelsDevProvider)
	}

	var provider struct {
		Models map[string]modelsDevEntry `json:"models"`
	}
	if err := json.Unmarshal(providerRaw, &provider); err != nil {
		return nil, fmt.Errorf("decode models.dev %s: %w", modelsDevProvider, err)
	}

	index := make(map[string]ModelMetadata, len(provider.Models))
	for id, entry := range provider.Models {
		index[id] = ModelMetadata{
			ContextWindow:  entry.Limit.Context,
			SupportsImages: entry.Attachment || slices.Contains(entry.Modalities.Input, "image"),
			ThinkingLevels: effortValues(entry),
			DisplayName:    entry.Name,
		}
	}
	return index, nil
}

// effortValues extracts the effort vocabulary from a model's reasoning options.
//
// Returns nil when the model has none, or when its only reasoning control is a
// toggle rather than a set of named levels — offering a level picker for those
// would be inventing a control that does not exist.
func effortValues(entry modelsDevEntry) []string {
	if !entry.Reasoning {
		return nil
	}
	for _, option := range entry.ReasoningOptions {
		if option.Type != "effort" {
			continue
		}
		// Keep only the string values: a control that publishes numbers or
		// booleans is not an effort vocabulary, and offering it as one would
		// send values the provider cannot parse.
		levels := make([]string, 0, len(option.Values))
		for _, value := range option.Values {
			if s, ok := value.(string); ok && s != "" {
				levels = append(levels, s)
			}
		}
		if len(levels) > 0 {
			return levels
		}
	}
	return nil
}
