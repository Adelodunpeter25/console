// Run helpers: model resolution for roles, background title generation,
// and storage record builders.
package run

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/loop"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/roles"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/titles"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/providers"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/services"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/types"
)

// generateTitle resolves the session title off the critical path: LLM
// title first, truncation fallback on failure/empty. Applied + broadcast
// only while the stored title is still generic. Uses a fresh provider
// instance so generation never interferes with the running turn.
func (s *Service) generateTitle(sessionID, prompt, providerID, modelID string, hub *Hub) {
	// Titles use the configured smol role model (TS generateSessionTitle
	// parity), falling back to the run model.
	titleModel := roles.ResolveRoleModel(roles.Smol, s.resolveModel(providerID, modelID), s.roleRef(roles.Smol))
	title := ""
	if provider, err := s.Lookup(titleModel.Provider); err == nil {
		title = titles.Generate(context.Background(), provider, titleModel.ID, prompt)
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

// resolveModel returns the catalog entry for a run model, synthesizing a
// fallback with inferred thinking levels for unknown ids (TS
// resolveRoleModel parity).
func (s *Service) resolveModel(providerID, modelID string) types.Model {
	if found, ok := providers.FindModel(providerID, modelID); ok {
		return found
	}
	levels, def := roles.InferThinkingLevels(providerID, modelID)
	return types.Model{
		ID: modelID, Provider: providerID, ContextWindow: 128_000,
		ThinkingLevels: levels, DefaultThinking: def,
	}
}

// resolveVision maps a model without image support to the configured
// vision role model when it does support images.
func (s *Service) resolveVision(model types.Model) (types.Model, bool) {
	vision := roles.ResolveRoleModel(roles.Vision, model, s.roleRef(roles.Vision))
	if !vision.SupportsImages {
		return model, false
	}
	return vision, true
}

// roleRef reads one configured model-role reference from settings.
func (s *Service) roleRef(role string) string {
	return services.NewSettingsService().Load().ModelRoles[role]
}

// userMessageRecord wraps a user message for storage (same shape the
// loop persists it in).
func userMessageRecord(user loop.UserMessage) types.AgentMessage {
	raw, err := json.Marshal(user)
	if err != nil {
		raw = []byte(`{"role":"user","content":"unserializable message"}`)
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
