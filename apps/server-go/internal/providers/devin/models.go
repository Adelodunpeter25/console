// Devin model discovery via GetCliModelConfigs, with the same cache
// shape as codex/models.go. The discovery cache also retains per-uid wire
// metadata (router flag, parallel-tool support, fusion lead) that RunTurn
// needs but types.Model cannot carry.
package devin

import (
	"context"
	"net/http"
	"strings"
	"sync"
	"time"

	exav1 "github.com/Adelodunpeter25/console/apps/server-go/internal/gen/exa/v1"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/providers/shared"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/types"
)

// DiscoveredModel is the provider-registry discovered shape.
type DiscoveredModel = types.Model

// modelCacheTTL mirrors the other providers: an hour keeps the picker and
// run resolution fresh without per-request fetches. There is no static
// seed — the live endpoint is the only source of truth.
const modelCacheTTL = time.Hour

var (
	modelCacheMu sync.Mutex
	modelCacheAt time.Time
	modelCache   []DiscoveredModel
	staleModels  []DiscoveredModel
	refreshing   bool

	// liveMeta is the last successful discovery's per-uid wire metadata,
	// keyed by the catalog id (the wire uid, or the fusion pairing uid).
	liveMeta map[string]modelMeta
)

// modelMeta is what RunTurn needs beyond types.Model.
type modelMeta struct {
	// requestUID overrides the wire uid sent as chatModelUid when no effort
	// route applies (fusion pairings run their lead model; the composite uid
	// is unservable).
	requestUID string
	// effortUIDs maps a thinking level ("minimal".."max", "none" for the
	// family's off route) to the chat uid the server declared for it.
	effortUIDs map[string]string
	// defaultLevel is the effort routing to the family's default member;
	// empty when the default member is reached through the off route.
	defaultLevel string
	// defaultUID is the family's default member chat uid.
	defaultUID string
	// isAssignRouter marks harness-less routing slots ("adaptive") that
	// must go through AssignModel before GetChatMessage.
	isAssignRouter bool
	// parallelTools mirrors modelFeatures.supportsParallelToolCalls;
	// absent features mean parallel calls stay disabled (upstream default).
	parallelTools bool
	// maxOutputTokens is the per-model output cap for CompletionConfiguration.
	maxOutputTokens int
}

// DiscoveryBaseURL overrides the models endpoint (tests).
var DiscoveryBaseURL = ""

func discoveryBaseURL() string {
	if DiscoveryBaseURL != "" {
		return DiscoveryBaseURL
	}
	return BaseURL
}

// InvalidateModelCache drops the discovery cache (tests; called on logout
// paths that rotate credentials).
func InvalidateModelCache() {
	modelCacheMu.Lock()
	defer modelCacheMu.Unlock()
	modelCacheAt = time.Time{}
	modelCache = nil
	staleModels = nil
	liveMeta = nil
	refreshing = false
}

// MarkModelCacheStale keeps the current list available while revalidating:
// an account switch may change model availability, but serving the old list
// for a moment beats blocking every catalog read on a discovery round-trip.
func MarkModelCacheStale() {
	modelCacheMu.Lock()
	defer modelCacheMu.Unlock()
	modelCacheAt = time.Time{}
	if len(modelCache) > 0 {
		staleModels = modelCache
	}
	modelCache = nil
}

func copyModels(in []DiscoveredModel) []DiscoveredModel {
	out := make([]DiscoveredModel, 0, len(in))
	return append(out, in...)
}

// Snapshot returns the last discovered list without fetching. Empty (never
// nil) until the first successful discovery.
func Snapshot() []DiscoveredModel {
	modelCacheMu.Lock()
	defer modelCacheMu.Unlock()
	return copyModels(modelCache)
}

// CachedModels returns the cached discovery, refreshing when stale or
// missing. With a stale list on hand it returns immediately and
// re-discovers in the background; only a cold cache blocks. Empty (never
// nil) when logged out or when the fetch fails with nothing cached.
func CachedModels(ctx context.Context) []DiscoveredModel {
	modelCacheMu.Lock()
	fresh := len(modelCache) > 0 && time.Since(modelCacheAt) < modelCacheTTL
	stale := copyModels(staleModels)
	busy := refreshing
	modelCacheMu.Unlock()
	if fresh {
		return Snapshot()
	}
	if len(stale) > 0 {
		if !busy {
			go discover()
		}
		return stale
	}
	return discover()
}

// discover fetches models with the currently active credential and stores
// them on success. Single-flight like the other providers.
func discover() []DiscoveredModel {
	modelCacheMu.Lock()
	if refreshing {
		modelCacheMu.Unlock()
		return Snapshot()
	}
	refreshing = true
	modelCacheMu.Unlock()
	defer func() {
		modelCacheMu.Lock()
		refreshing = false
		modelCacheMu.Unlock()
	}()

	cred, err := LoadCredential()
	if err != nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	discovered, meta, err := FetchModels(ctx, nil, discoveryBaseURL(), cred.Token)
	if err != nil || len(discovered) == 0 {
		return nil
	}
	modelCacheMu.Lock()
	modelCache, modelCacheAt = discovered, time.Now()
	staleModels = discovered
	liveMeta = meta
	out := copyModels(modelCache)
	modelCacheMu.Unlock()
	return out
}

// ResolveModel finds id in discovery, fetching once on a cold cache.
// Case-insensitive like providers.FindModel.
func ResolveModel(ctx context.Context, id string) (DiscoveredModel, bool) {
	for _, m := range CachedModels(ctx) {
		if strings.EqualFold(m.ID, id) {
			return m, true
		}
	}
	return DiscoveredModel{}, false
}

// modelMetaFor returns the run-time wire metadata for a catalog id, or the
// zero value when discovery has not run (tests, cold cache).
func modelMetaFor(id string) modelMeta {
	modelCacheMu.Lock()
	defer modelCacheMu.Unlock()
	if liveMeta == nil {
		return modelMeta{}
	}
	return liveMeta[id]
}

// FetchModels lists Devin models through GetCliModelConfigs. It tries the
// native chisel discovery identity first; seats that answer with only the
// seed rows fall back to the legacy Windsurf-editor identity, which exposes
// the full credential-scoped roster on enterprise plans.
func FetchModels(ctx context.Context, client *http.Client, baseURL, sessionToken string) ([]DiscoveredModel, map[string]modelMeta, error) {
	if client == nil {
		client = shared.PooledClient(30 * time.Second)
	}
	fetchCatalog := func(md *exav1.Metadata) ([]*exav1.ClientModelConfig, error) {
		req := &exav1.GetCliModelConfigsRequest{Metadata: md}
		resp := &exav1.GetCliModelConfigsResponse{}
		if err := postUnary(ctx, client, baseURL, cliModelConfigsPath, req, resp); err != nil {
			return nil, err
		}
		return resp.GetClientModelConfigs(), nil
	}

	nativeConfigs, err := fetchCatalog(discoverySessionMetadata(sessionToken))
	if err != nil {
		return nil, nil, err
	}
	nativeModels, nativeMeta, err := normalizeDevinModels(nativeConfigs)
	if err != nil {
		return nil, nil, err
	}
	// The seed-only check runs on the normalized list, like upstream: a
	// disabled or internal non-seed config does not count.
	nativeIsSeedOnly := len(nativeModels) > 0
	for _, m := range nativeModels {
		if m.ID != "swe-1-6" && m.ID != "swe-1-6-fast" {
			nativeIsSeedOnly = false
			break
		}
	}
	if !nativeIsSeedOnly && len(nativeModels) > 0 {
		return nativeModels, nativeMeta, nil
	}

	// Legacy Windsurf Enterprise seats expose their full credential-scoped
	// roster only to the editor identity and raw session token. Native
	// chisel discovery returns the two-row seed for those seats.
	legacyConfigs, lerr := fetchCatalog(legacyDiscoveryMetadata(sessionToken))
	if lerr != nil {
		if len(nativeModels) > 0 {
			return nativeModels, nativeMeta, nil
		}
		return nil, nil, lerr
	}
	legacyModels, legacyMeta, err := normalizeDevinModels(legacyConfigs)
	if err != nil {
		return nil, nil, err
	}
	if len(legacyModels) > len(nativeModels) {
		return legacyModels, legacyMeta, nil
	}
	if len(nativeModels) > 0 {
		return nativeModels, nativeMeta, nil
	}
	return legacyModels, legacyMeta, nil
}

// internalDisplayOptions are requested but never surfaced.
var internalDisplayOptions = map[exav1.DisplayOption]bool{
	exav1.DisplayOption_DISPLAY_OPTION_QUICK_REVIEW:     true,
	exav1.DisplayOption_DISPLAY_OPTION_INTERNAL_DEFAULT: true,
}

// devinFamilyLane is one server-declared family lane: fast service and 1M
// context are separate logical models; reasoning effort is the lane's only
// selectable axis (port of oh-my-pi's collectDevinFamilyLane).
type devinFamilyLane struct {
	id            string
	members       []string
	defaultMember string
	routing       map[string]string // effort or "none" -> member wire uid; first claim wins
}

// devinEffortByName maps family-metadata effort display names onto Console's
// thinking-level vocabulary ("none" covers the family's off route).
var devinEffortByName = map[string]string{
	"none": "none", "nothinking": "none",
	"minimal": "minimal", "low": "low", "medium": "medium",
	"high": "high", "xhigh": "xhigh", "max": "max",
}

// devinEffortOrder is the picker order for collapsed family levels.
var devinEffortOrder = []string{"minimal", "low", "medium", "high", "xhigh", "max"}

// normalizeKey lowercases and collapses non-alphanumerics to single spaces
// ("Reasoning Effort" -> "reasoning effort").
func normalizeKey(s string) string {
	var b strings.Builder
	space := false
	for _, r := range strings.ToLower(s) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			if space && b.Len() > 0 {
				b.WriteByte(' ')
			}
			space = false
			b.WriteRune(r)
		} else {
			space = true
		}
	}
	return b.String()
}

// normalizeName lowercases and drops every non-alphanumeric ("X High" and
// "XHigh" both -> "xhigh").
func normalizeName(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// collectDevinFamilyLane files config under its server-declared family lane.
// Configs with no family metadata, or whose family carries no effort axis,
// are left alone and stay standalone.
func collectDevinFamilyLane(lanes map[string]*devinFamilyLane, c *exav1.ClientModelConfig, uid string) {
	fm := c.GetModelFamilyMetadata()
	if fm == nil {
		return
	}
	label := strings.TrimSpace(fm.GetModelFamilyLabel())
	if label == "" {
		return
	}

	var effort string
	thinking := -1 // -1 unset, 0 false, 1 true
	var fast, oneMillion bool
	for _, e := range fm.GetEntries() {
		v := e.GetValue()
		if v == nil {
			continue
		}
		key := normalizeKey(e.GetKey())
		switch {
		case key == "fast mode":
			fast = v.GetOrder() == 1
		case key == "thinking":
			if v.GetOrder() == 1 {
				thinking = 1
			} else {
				thinking = 0
			}
		case key == "1m context":
			oneMillion = v.GetOrder() == 1
		case key == "effort" || key == "reasoning effort":
			effort = devinEffortByName[normalizeName(v.GetName())]
		}
	}
	// Claude's paired non-thinking/thinking configs share effort labels; the
	// explicit Thinking axis decides whether the route is off.
	if thinking == 0 {
		effort = "none"
	}

	baseID := strings.ReplaceAll(normalizeKey(label), " ", "-")
	if baseID == "" {
		return
	}
	laneID := baseID
	if oneMillion {
		laneID += "-1m"
	}
	if fast {
		laneID += "-fast"
	}
	lane := lanes[laneID]
	if lane == nil {
		lane = &devinFamilyLane{id: laneID, routing: map[string]string{}}
		lanes[laneID] = lane
	}
	lane.members = append(lane.members, uid)
	if lane.defaultMember == "" && (c.GetIsDefaultModelInFamily() || fm.GetIsDefaultModelInFamily()) {
		lane.defaultMember = uid
	}
	if effort != "" {
		if _, claimed := lane.routing[effort]; !claimed {
			lane.routing[effort] = uid
		}
	}
}

// normalizeDevinModels filters disabled/internal configs, resolves fusion
// pairings to their lead, collapses server-declared effort families into
// one logical model per lane, and returns catalog models plus per-id run
// metadata. Port of oh-my-pi's normalizeDevinModels.
func normalizeDevinModels(configs []*exav1.ClientModelConfig) ([]DiscoveredModel, map[string]modelMeta, error) {
	liveUIDs := map[string]bool{}
	liveConfigs := map[string]*exav1.ClientModelConfig{}
	for _, c := range configs {
		uid := strings.TrimSpace(c.GetModelUid())
		if !c.GetDisabled() && uid != "" {
			liveUIDs[uid] = true
			if _, ok := liveConfigs[uid]; !ok {
				liveConfigs[uid] = c
			}
		}
	}

	type entry struct {
		config         *exav1.ClientModelConfig
		uid            string
		isAssignRouter bool
		requestUID     string
	}
	var entries []entry
	lanes := map[string]*devinFamilyLane{}
	seen := map[string]bool{}
	for _, c := range configs {
		if c.GetDisabled() {
			continue
		}
		mi := c.GetModelInfo()
		display := exav1.DisplayOption_DISPLAY_OPTION_UNSPECIFIED
		if mi != nil {
			display = mi.GetDisplayOption()
		}
		if internalDisplayOptions[display] {
			continue
		}
		uid := strings.TrimSpace(c.GetModelUid())
		if uid == "" || seen[uid] {
			continue
		}
		seen[uid] = true

		isRouter := display == exav1.DisplayOption_DISPLAY_OPTION_MODEL_ROUTER || (mi != nil && mi.GetIsModelRouter())
		// isModelRouter marks two things: harness-less routing slots
		// ("adaptive") that AssignModel resolves, and harness-backed
		// composites ("fusion-*") that are themselves valid chat uids. Only
		// the former take the AssignModel path.
		isAssignRouter := isRouter && (mi == nil || len(mi.GetHarnessUids()) == 0)

		// Fusion pairings are orchestrated by the native client: it runs
		// the lead model as an ordinary chat uid. The composite uid itself
		// is unservable, so the request carries the lead uid; a pairing
		// without a live lead is not listed.
		requestUID := ""
		if leadUID, isFusion, leadLive := fusionLead(uid, liveUIDs); isFusion {
			if !leadLive {
				continue
			}
			requestUID = leadUID
		}

		entries = append(entries, entry{config: c, uid: uid, isAssignRouter: isAssignRouter, requestUID: requestUID})
		// A router is a server-side dispatcher, not an effort tier: it stays
		// a standalone model even when upstream files it under a family.
		if !isRouter {
			collectDevinFamilyLane(lanes, c, uid)
		}
	}

	// A lane collapses only when it declares at least one non-off effort
	// route; member wire uids then leave the standalone list (upstream
	// collapseWithTable replaces them with the family spec).
	familyOf := map[string]string{}
	collapsed := map[string]bool{}
	for id, lane := range lanes {
		hasEffort := false
		for e := range lane.routing {
			if e != "none" {
				hasEffort = true
				break
			}
		}
		if !hasEffort {
			continue
		}
		collapsed[id] = true
		for _, m := range lane.members {
			familyOf[m] = id
		}
	}

	var models []DiscoveredModel
	meta := map[string]modelMeta{}
	for _, e := range entries {
		if _, ok := familyOf[e.uid]; ok {
			continue // emitted with its family below
		}
		spec, m := devinModelMeta(e.config, e.uid, liveConfigs, e.requestUID, e.isAssignRouter)
		models = append(models, spec)
		meta[e.uid] = m
	}

	for _, lane := range lanes {
		if !collapsed[lane.id] {
			continue
		}
		defUID := lane.defaultMember
		if defUID == "" && len(lane.members) > 0 {
			defUID = lane.members[0]
		}
		defConfig := liveConfigs[defUID]
		if defConfig == nil {
			continue
		}
		// Chat-resolve every routed uid through the fusion-lead table.
		effortUIDs := map[string]string{}
		var levels []string
		for _, effort := range devinEffortOrder {
			if uid, ok := lane.routing[effort]; ok {
				levels = append(levels, effort)
				effortUIDs[effort] = chatUIDForWire(uid, liveUIDs)
			}
		}
		if uid, ok := lane.routing["none"]; ok {
			levels = append([]string{"none"}, levels...)
			effortUIDs["none"] = chatUIDForWire(uid, liveUIDs)
		}
		defaultUID := chatUIDForWire(defUID, liveUIDs)
		defaultLevel := ""
		for _, effort := range devinEffortOrder {
			if lane.routing[effort] == defUID {
				defaultLevel = effort
				break
			}
		}

		spec, m := devinModelMeta(defConfig, lane.id, liveConfigs, defaultUID, false)
		spec.ID = lane.id
		spec.ThinkingLevels = levels
		spec.DefaultThinking = defaultLevel
		m.requestUID = defaultUID
		m.effortUIDs = effortUIDs
		m.defaultLevel = defaultLevel
		m.defaultUID = defaultUID
		models = append(models, spec)
		meta[lane.id] = m
	}
	return models, meta, nil
}

// chatUIDForWire resolves a member wire uid to the uid actually sent on the
// wire: fusion pairings route to their live lead.
func chatUIDForWire(uid string, liveUIDs map[string]bool) string {
	if lead, isFusion, live := fusionLead(uid, liveUIDs); isFusion && live {
		return lead
	}
	return uid
}

// devinModelMeta builds the catalog spec + run metadata for one config,
// mirroring devinModelSpec/routeDevinFusionLead upstream. requestUID is the
// chat uid override (fusion lead or family default), "" for the raw uid.
func devinModelMeta(c *exav1.ClientModelConfig, id string, liveConfigs map[string]*exav1.ClientModelConfig, requestUID string, isAssignRouter bool) (DiscoveredModel, modelMeta) {
	uid := strings.TrimSpace(c.GetModelUid())
	mi := c.GetModelInfo()
	var features *exav1.ModelFeatures
	if mi != nil {
		features = mi.GetModelFeatures()
	}
	supportsImages := c.GetSupportsImages()
	if features != nil {
		supportsImages = features.GetSupportsImages()
	}
	supportsImages = supportsImages && !imageBlindUIDs[uid]

	contextWindow := int(c.GetMaxTokens())
	if contextWindow <= 0 {
		contextWindow = defaultContextWindow
	}
	// Fusion specs budget against the lead's limits, not the composite
	// card's (routeDevinFusionLead upstream).
	if requestUID != "" {
		if lead, ok := liveConfigs[requestUID]; ok {
			if cw := int(lead.GetMaxTokens()); cw > 0 {
				contextWindow = cw
			}
		}
	}

	return DiscoveredModel{
		ID:             id,
		Provider:       "devin",
		ContextWindow:  contextWindow,
		SupportsImages: supportsImages,
	}, modelMeta{
		requestUID:      requestUID,
		isAssignRouter:  isAssignRouter,
		parallelTools:   features != nil && features.GetSupportsParallelToolCalls(),
		maxOutputTokens: int(mi.GetMaxOutputTokens()),
	}
}

// fusionLead resolves the lead chat uid of a Fusion pairing
// `fusion-<lead>[-fast]-sidekick-<sidekick>`. Returns (leadUID, isFusion,
// leadLive); isFusion is false for non-pairing uids.
func fusionLead(uid string, liveUIDs map[string]bool) (string, bool, bool) {
	if !strings.HasPrefix(uid, "fusion-") {
		return "", false, false
	}
	cut := strings.Index(uid, "-sidekick-")
	if cut <= len("fusion-") {
		return "", false, false
	}
	lead := uid[len("fusion-"):cut]
	if liveUIDs[lead] {
		return lead, true, true
	}
	if strings.HasSuffix(lead, "-fast") {
		base := strings.TrimSuffix(lead, "-fast")
		if liveUIDs[base+"-priority"] {
			return base + "-priority", true, true
		}
		if liveUIDs[base] {
			return base, true, true
		}
	}
	return "", true, false
}

// chatModelUID resolves the catalog id plus optional thinking level to the
// wire chatModelUid and whether it needs AssignModel resolution. Falls back
// to the raw id when discovery has no metadata for it.
func chatModelUID(modelID, thinkingLevel string) (uid string, meta modelMeta) {
	meta = modelMetaFor(modelID)
	uid = meta.requestUID
	if len(meta.effortUIDs) > 0 {
		level := thinkingLevel
		if level == "" {
			level = meta.defaultLevel
		}
		if routed, ok := meta.effortUIDs[level]; ok {
			uid = routed
		} else if meta.defaultUID != "" {
			uid = meta.defaultUID
		}
	}
	if uid == "" {
		uid = modelID
	}
	return uid, meta
}
