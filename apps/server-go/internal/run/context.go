// Per-session context-window occupancy: estimated payload tokens against
// the run model's window, cached by message count so the footer ring and
// the on-demand endpoint never pay for a recompute they don't need.
package run

import (
	"context"
	"sync"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/compaction"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/loop"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/systemprompt"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/tools"
)

// contextThresholdRatio mirrors the compaction default: the ring turns red
// at the same occupancy that triggers autocompact.
const contextThresholdRatio = 0.85

type contextCacheEntry struct {
	messageCount int
	snapshot     loop.ContextSnapshot
}

type contextCache struct {
	mu      sync.Mutex
	entries map[string]contextCacheEntry
}

// ContextUsage returns the session's cached context snapshot, recomputing
// when the message count moved since the last snapshot. No session (or no
// model window) yields a zero snapshot, never an error for missing rows —
// callers map unknown sessions to 404 themselves.
func (s *Service) ContextUsage(ctx context.Context, sessionID string) (loop.ContextSnapshot, error) {
	loaded, err := s.sessions.Load(sessionID, 0, 0)
	if err != nil {
		return loop.ContextSnapshot{}, err
	}
	if loaded == nil {
		return loop.ContextSnapshot{}, ErrNoSession
	}
	header := loaded.Header

	s.ctxCache.mu.Lock()
	if entry, ok := s.ctxCache.entries[sessionID]; ok && entry.messageCount == header.MessageCount {
		snap := entry.snapshot
		s.ctxCache.mu.Unlock()
		return snap, nil
	}
	s.ctxCache.mu.Unlock()

	model := s.resolveModel(ctx, header.Provider, header.ModelID)
	prompt := s.prompts.get(sessionID, systemprompt.BuildOptions{
		Cwd:          header.Cwd,
		Model:        header.ModelID,
		ApprovalMode: systemprompt.ApprovalMode(header.ApprovalMode),
	})
	history := decodeHistory(loaded.Messages)
	defs := tools.NewRegistry(tools.DefaultTools()...).Definitions()
	est := compaction.EstimatePayloadTokens(history, prompt.SystemPrompt, defs)

	snap := loop.ContextSnapshot{
		UsedTokens:     est.Tokens,
		ContextWindow:  model.ContextWindow,
		ThresholdRatio: contextThresholdRatio,
		ModelID:        model.ID,
		Provider:       model.Provider,
		Source:         est.Source,
	}
	if model.ContextWindow > 0 {
		snap.PercentUsed = float64(est.Tokens) / float64(model.ContextWindow) * 100
	}

	s.ctxCache.mu.Lock()
	if s.ctxCache.entries == nil {
		s.ctxCache.entries = map[string]contextCacheEntry{}
	}
	s.ctxCache.entries[sessionID] = contextCacheEntry{messageCount: header.MessageCount, snapshot: snap}
	s.ctxCache.mu.Unlock()
	return snap, nil
}

// broadcastContextUsage recomputes (or reuses) the snapshot and pushes it to
// subscribers. Best-effort: a context frame must never fail a turn.
func (s *Service) broadcastContextUsage(ctx context.Context, sessionID string, hub *Hub) {
	snap, err := s.ContextUsage(ctx, sessionID)
	if err != nil {
		return
	}
	hub.Broadcast(loop.Event{Kind: loop.EventContextUpdate, Context: &snap})
}
