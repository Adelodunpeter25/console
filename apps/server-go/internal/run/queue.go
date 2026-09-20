// Queued prompts and steering. Port of the queue slice of
// apps/server/api/src/services/run.service.ts: stage (or replace) the
// prompt that runs once the active turn settles, edit/fetch/discard it,
// and steer (halt the run, staged prompt drains next).
package run

import (
	"time"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/loop"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/types"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/utils"
)

// QueuePrompt stages (or replaces) the next-turn prompt. Shared by queue
// (auto-run on settle) and steer (abort now, drain sooner). Returns
// ErrNoSession when the session does not exist.
func (s *Service) QueuePrompt(sessionID string, dto Prompt) (*types.QueuedPrompt, error) {
	loaded, err := s.sessions.Load(sessionID, 0, 0)
	if err != nil {
		return nil, err
	}
	if loaded == nil {
		return nil, ErrNoSession
	}
	qp := &types.QueuedPrompt{
		ID: utils.RandomID(), SessionID: sessionID, Prompt: dto.Text,
		ModelID: dto.ModelID, Provider: dto.Provider, ApprovalMode: dto.ApprovalMode,
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
	}
	for _, a := range dto.Attachments {
		qp.Attachments = append(qp.Attachments, types.QueuedAttachment{Data: a.Data, MimeType: a.MimeType})
	}
	if err := s.sessions.SaveQueuedPrompt(sessionID, *qp); err != nil {
		return nil, err
	}
	s.mu.Lock()
	s.pending[sessionID] = dto
	hub := activeHubLocked(s.active, sessionID)
	s.mu.Unlock()
	if hub != nil {
		hub.Broadcast(loop.Event{Kind: loop.EventQueueUpdated, Queued: qp})
	}
	return qp, nil
}

// EditQueuedPrompt edits the staged prompt in place (keeps its id).
// Returns (nil, nil) when no prompt is staged.
func (s *Service) EditQueuedPrompt(sessionID string, dto Prompt) (*types.QueuedPrompt, error) {
	existing, err := s.sessions.GetQueuedPrompt(sessionID)
	if err != nil {
		return nil, err
	}
	if existing == nil {
		if _, ok := s.pendingStaged(sessionID); !ok {
			return nil, nil
		}
		existing = &types.QueuedPrompt{ID: utils.RandomID(), SessionID: sessionID, CreatedAt: time.Now().UTC().Format(time.RFC3339)}
	}
	existing.Prompt = dto.Text
	existing.ModelID = dto.ModelID
	existing.Provider = dto.Provider
	existing.ApprovalMode = dto.ApprovalMode
	existing.Attachments = nil
	for _, a := range dto.Attachments {
		existing.Attachments = append(existing.Attachments, types.QueuedAttachment{Data: a.Data, MimeType: a.MimeType})
	}
	if err := s.sessions.SaveQueuedPrompt(sessionID, *existing); err != nil {
		return nil, err
	}
	s.mu.Lock()
	s.pending[sessionID] = dto
	hub := activeHubLocked(s.active, sessionID)
	s.mu.Unlock()
	if hub != nil {
		hub.Broadcast(loop.Event{Kind: loop.EventQueueUpdated, Queued: existing})
	}
	return existing, nil
}

// QueuedPrompt returns the staged prompt, or nil when none is staged.
func (s *Service) QueuedPrompt(sessionID string) (*types.QueuedPrompt, error) {
	return s.sessions.GetQueuedPrompt(sessionID)
}

// ClearQueuedPrompt discards the staged prompt. Returns true when one was
// staged.
func (s *Service) ClearQueuedPrompt(sessionID string) (bool, error) {
	s.mu.Lock()
	_, hadPending := s.pending[sessionID]
	delete(s.pending, sessionID)
	hub := activeHubLocked(s.active, sessionID)
	s.mu.Unlock()
	stored, err := s.sessions.GetQueuedPrompt(sessionID)
	if err != nil {
		return false, err
	}
	if err := s.sessions.ClearQueuedPrompt(sessionID); err != nil {
		return false, err
	}
	had := hadPending || stored != nil
	if had && hub != nil {
		hub.Broadcast(loop.Event{Kind: loop.EventQueueUpdated})
	}
	return had, nil
}

// Steer halts the active run and stages dto to start as the next turn as
// soon as the aborted run settles. Returns false when no run is active.
func (s *Service) Steer(sessionID string, dto Prompt) (bool, error) {
	if !s.IsActive(sessionID) {
		return false, nil
	}
	if _, err := s.QueuePrompt(sessionID, dto); err != nil {
		return false, err
	}
	// Abort without clearing: the staged prompt must survive to drain.
	s.mu.Lock()
	ar, ok := s.active[sessionID]
	s.mu.Unlock()
	if !ok {
		return false, nil
	}
	ar.cancel()
	s.decisions.RejectAllForSession(sessionID, "Run steered")
	return true, nil
}

// takeStaged takes the staged next-turn prompt, if any. Prefers the
// in-memory entry; falls back to the persisted row so a prompt staged
// before a restart still drains. Clears both stores.
func (s *Service) takeStaged(sessionID string) (Prompt, bool) {
	s.mu.Lock()
	dto, ok := s.pending[sessionID]
	if ok {
		delete(s.pending, sessionID)
	}
	s.mu.Unlock()
	if ok {
		_ = s.sessions.ClearQueuedPrompt(sessionID)
		return dto, true
	}
	stored, err := s.sessions.GetQueuedPrompt(sessionID)
	if err != nil || stored == nil {
		return Prompt{}, false
	}
	_ = s.sessions.ClearQueuedPrompt(sessionID)
	out := Prompt{
		Text: stored.Prompt, ModelID: stored.ModelID, Provider: stored.Provider,
		ApprovalMode: stored.ApprovalMode,
	}
	for _, a := range stored.Attachments {
		out.Attachments = append(out.Attachments, Attachment{Data: a.Data, MimeType: a.MimeType})
	}
	return out, true
}

func (s *Service) pendingStaged(sessionID string) (Prompt, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	dto, ok := s.pending[sessionID]
	return dto, ok
}

func activeHubLocked(active map[string]*activeRun, sessionID string) *Hub {
	if ar, ok := active[sessionID]; ok {
		return ar.hub
	}
	return nil
}
