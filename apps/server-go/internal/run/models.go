// Run helpers: model resolution for roles, background title generation,
// and storage record builders.
package run

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"strings"
	"time"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/compaction"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/loop"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/roles"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/titles"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/tools"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/vision"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/providers"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/services"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/types"
)

// generateTitle resolves the session title off the critical path: LLM
// title first, truncation fallback on failure/empty. Applied + broadcast
// only while the stored title is still generic. Uses a fresh provider
// instance so generation never interferes with the running turn.
func (s *Service) generateTitle(sessionID, prompt, providerID, modelID string, hub *Hub, pins *accountPins) {
	// Titles use the configured smol role model, falling back to the run
	// model.
	titleModel := roles.ResolveRoleModel(roles.Smol, s.resolveModel(context.Background(), providerID, modelID), s.roleRef(roles.Smol))
	title := ""
	if provider, err := s.Lookup(titleModel.Provider); err == nil {
		title = titles.Generate(context.Background(), provider, titleModel.ID, prompt, pins.forProvider(titleModel.Provider))
	}
	if title == "" {
		title = titles.FallbackTitle(prompt)
	}
	loaded, err := s.sessions.Load(sessionID, 0, 0)
	if err != nil || loaded == nil || !titles.IsGenericTitle(loaded.Header.Title) {
		return
	}
	if err := s.sessions.UpdateTitle(sessionID, title); err != nil {
		return
	}
	hub.Broadcast(loop.Event{Kind: loop.EventSessionTitleUpdated, Title: title})
}

// resolveModel returns the catalog entry for a run model, consulting live
// discovery (fetch-once-per-hour cache) before synthesizing a fallback
// with inferred thinking levels for unknown ids.
func (s *Service) resolveModel(ctx context.Context, providerID, modelID string) types.Model {
	if found, ok := providers.FindModel(providerID, modelID); ok {
		return found
	}
	if found, ok := providers.ResolveLiveModel(ctx, providerID, modelID); ok {
		return found
	}
	levels, def := roles.InferThinkingLevels(providerID, modelID)
	return types.Model{
		ID: modelID, Provider: providerID, ContextWindow: 128_000,
		ThinkingLevels: levels, DefaultThinking: def,
	}
}

// DeliversToolResultImages reports whether a provider renders inline images
// carried by a tool result. Claude sends them natively; Codex and Antigravity
// forward them in their own wire shapes. Other adapters (e.g. OpenCode) redact
// them to a placeholder before the request is built, regardless of the model's
// declared capability.
func DeliversToolResultImages(providerID string) bool {
	switch providerID {
	case "claude", "codex", "antigravity":
		return true
	}
	return false
}

// resolveVision maps a model without image support to the configured vision
// role model when it does support images.
func (s *Service) resolveVision(model types.Model) (types.Model, bool) {
	vm, ok := s.visionRole(model)
	if !ok || !vm.SupportsImages {
		return model, false
	}
	return vm, true
}

// visionRole resolves the configured vision role, reporting whether one is
// set at all. An id the catalog does not know is trusted on the user's word,
// with a warning so the decision is visible in logs.
func (s *Service) visionRole(model types.Model) (types.Model, bool) {
	ref := s.roleRef(roles.Vision)
	if strings.TrimSpace(ref) == "" {
		return types.Model{}, false
	}
	vm := roles.ResolveRoleModel(roles.Vision, model, ref)
	if _, ok := providers.FindModel(vm.Provider, vm.ID); !ok {
		slog.Warn("vision role resolves to an uncatalogued model; trusting its declared image support",
			"model", vm.Provider+"/"+vm.ID)
	}
	return vm, true
}

// visionLabel is the "provider/model" of the configured vision role, or "".
func (s *Service) visionLabel(model types.Model) string {
	vm, ok := s.visionRole(model)
	if !ok {
		return ""
	}
	return vm.Provider + "/" + vm.ID
}

// ReadFileToolOptions builds read_file's per-run options: direct image parts
// when the model and provider can carry them, otherwise a vision-role
// describer, otherwise the placeholder. pins pins the describer's calls to
// the run's account (decision 7); nil keeps the ordinary loader.
func (s *Service) ReadFileToolOptions(model types.Model, providerID, modelID string, pins *accountPins) tools.ReadFileOptions {
	return tools.ReadFileOptions{
		CanSeeImages: model.SupportsImages && DeliversToolResultImages(providerID),
		Describer:    s.imageDescriber(model, providerID, pins),
		RunModel:     modelID,
		VisionModel:  s.visionLabel(model),
	}
}

// imageDescriber returns a describer for an image the run would otherwise not
// see. Capability is not delivery: a model that sees user-message images still
// gets tool-result images redacted by a non-delivering adapter, so it needs a
// describer too.
func (s *Service) imageDescriber(model types.Model, providerID string, pins *accountPins) tools.ImageDescriber {
	if model.SupportsImages && DeliversToolResultImages(providerID) {
		return nil
	}
	if !services.NewSettingsService().Load().Vision.DescribeImagesEnabled() {
		return nil
	}
	if _, ok := s.resolveVision(model); !ok {
		return nil
	}
	return &visionDescriber{s: s, fallback: model, pins: pins}
}

// visionDescriber runs the configured vision model once per image.
type visionDescriber struct {
	s        *Service
	fallback types.Model
	// pins, when set, pins the describer's calls to the run's account.
	pins *accountPins
}

func (d *visionDescriber) Describe(ctx context.Context, mime string, data []byte, hint string) (string, bool) {
	vm, ok := d.s.resolveVision(d.fallback)
	if !ok {
		return "", false
	}
	provider, err := d.s.Lookup(vm.Provider)
	if err != nil {
		return "", false
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	desc := vision.Describe(ctx, provider, vm.ID, mime, data, hint, d.pins.forProvider(vm.Provider))
	if strings.TrimSpace(desc) == "" {
		return "", false
	}
	return desc, true
}

// roleRef reads one configured model-role reference from settings.
func (s *Service) roleRef(role string) string {
	return services.NewSettingsService().Load().ModelRoles[role]
}

// compactionHooks builds the per-turn context management hooks: payload
// threshold checks with structural summaries, smol-model narratives when
// configured, and emergency recovery on context overflow.
func (s *Service) compactionHooks(sessionID string, model types.Model, systemPrompt string, defs []tools.Definition, pins *accountPins) *loop.CompactionHooks {
	options := compaction.Options{}
	emergencyChars := compaction.EmergencyChars
	return &loop.CompactionHooks{
		PreTurn: func(ctx context.Context, history []any) []any {
			if !compaction.ShouldCompact(history, model.ContextWindow, options, systemPrompt, defs) {
				return history
			}
			// Compaction rewrites the prefix anyway; rebuild setup next run.
			s.prompts.forget(sessionID)
			result := compaction.CompactHistory(history, options)
			if summary, ok := s.summarizeCompaction(ctx, model, history, pins); ok {
				result = compaction.CompactHistoryWithSummary(history, options, summary)
			}
			return result.CompactedMessages
		},
		IsOverflow: compaction.IsContextOverflowError,
		Emergency: func(history []any) []any {
			shaken := compaction.ShakeConversation(history, emergencyChars, len(history), true)
			options.KeepRecentTokens = 20000
			return compaction.CompactHistory(shaken, options).CompactedMessages
		},
	}
}

// summarizeCompaction produces an smol-model narrative summary when a smol
// role is configured; false keeps the structural summary. The summarizer
// runs under the run's pinned account.
func (s *Service) summarizeCompaction(ctx context.Context, model types.Model, history []any, pins *accountPins) (string, bool) {
	ref := s.roleRef(roles.Smol)
	if strings.TrimSpace(ref) == "" {
		return "", false
	}
	smol := roles.ResolveRoleModel(roles.Smol, model, ref)
	provider, err := s.Lookup(smol.Provider)
	if err != nil {
		return "", false
	}
	summary, err := compaction.SummarizeWithProvider(ctx, provider, smol.ID, history, "", pins.forProvider(smol.Provider))
	if err != nil || strings.TrimSpace(summary) == "" {
		return "", false
	}
	return strings.TrimSpace(summary), true
}

// userMessageRecord wraps a user message for storage (same canonical shape
// the loop persists it in).
func userMessageRecord(user loop.UserMessage) types.AgentMessage {
	raw, err := loop.ToProtoBytes(user)
	if err != nil {
		raw = []byte(`{"user":{"content":"unserializable message"}}`)
	}
	return types.AgentMessage{
		ID:   "msg_" + randomHex(16),
		Role: string(loop.RoleUser),
		Data: raw,
	}
}

func randomHex(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}
