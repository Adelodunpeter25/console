// Queued prompts and steering: queue prompts that run, oldest first, as each
// turn settles; edit, fetch or discard them; and steer (halt the run, the
// steering prompt runs next).
package run

import (
	"errors"
	"log/slog"
	"time"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/loop"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/types"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/utils"
)

// QueuePrompt adds the prompt to the back of the session's queue; queued
// prompts run oldest first as each turn settles. Returns ErrNoSession when
// the session does not exist.
func (s *Service) QueuePrompt(sessionID string, dto Prompt) (*types.QueuedPrompt, error) {
	return s.stage(sessionID, dto, false)
}

// stage queues dto at the back, or at the front when it must run next.
// Steer stages at the front.
func (s *Service) stage(sessionID string, dto Prompt, front bool) (*types.QueuedPrompt, error) {
	loaded, err := s.sessions.Load(sessionID, 0, 0)
	if err != nil {
		return nil, err
	}
	if loaded == nil {
		return nil, ErrNoSession
	}
	// A queued prompt is a send too: it unsettles (and Steer inherits this).
	s.unsettleOnSend(sessionID)
	qp := &types.QueuedPrompt{
		ID: utils.RandomID(), SessionID: sessionID, Prompt: dto.Text, ContextFiles: dto.ContextFiles,
		Annotations: dto.Annotations,
		ModelID:     dto.ModelID, Provider: dto.Provider, ApprovalMode: dto.ApprovalMode,
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
	}
	for _, a := range dto.Attachments {
		qp.Attachments = append(qp.Attachments, types.QueuedAttachment{Data: a.Data, MimeType: a.MimeType})
	}
	if front {
		err = s.sessions.SaveQueuedPromptFirst(sessionID, *qp)
	} else {
		err = s.sessions.SaveQueuedPrompt(sessionID, *qp)
	}
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	s.pending[qp.ID] = dto
	s.mu.Unlock()
	s.broadcastQueue(sessionID, false)
	return qp, nil
}

// broadcastQueue tells everyone watching the run what the queue holds now.
// drained marks that the front prompt just started running rather than the
// queue being edited or cleared.
func (s *Service) broadcastQueue(sessionID string, drained bool) {
	s.mu.Lock()
	hub := activeHubLocked(s.active, sessionID)
	s.mu.Unlock()
	if hub == nil {
		return
	}
	queue, err := s.sessions.ListQueuedPrompts(sessionID)
	if err != nil {
		slog.Warn("queue broadcast: list failed", "session", sessionID, "err", err)
		return
	}
	event := loop.Event{Kind: loop.EventQueueUpdated, QueuedAll: queue, Drained: drained}
	if !drained && len(queue) > 0 {
		event.Queued = &queue[0]
	}
	hub.Broadcast(event)
}

// startStagedIfIdle runs the next queued prompt now when no run is active and
// reports whether it did. If the run that would drain it started meanwhile,
// the prompt goes back in the queue.
func (s *Service) startStagedIfIdle(sessionID string) (bool, error) {
	if s.IsActive(sessionID) {
		return false, nil
	}
	staged, ok := s.takeStaged(sessionID)
	if !ok {
		return false, nil
	}
	if _, err := s.StartRun(sessionID, staged); err != nil {
		if errors.Is(err, ErrActive) {
			_, err = s.QueuePrompt(sessionID, staged)
		}
		return false, err
	}
	return true, nil
}

// EditQueuedPrompt edits the prompt that runs next in place (keeps its id).
// Returns (nil, nil) when nothing is queued.
func (s *Service) EditQueuedPrompt(sessionID string, dto Prompt) (*types.QueuedPrompt, error) {
	existing, err := s.sessions.GetQueuedPrompt(sessionID)
	if err != nil || existing == nil {
		return nil, err
	}
	existing.Prompt = dto.Text
	existing.ContextFiles = dto.ContextFiles
	existing.Annotations = dto.Annotations
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
	s.pending[existing.ID] = dto
	s.mu.Unlock()
	s.broadcastQueue(sessionID, false)
	return existing, nil
}

// QueuedPrompt returns the prompt that runs next, or nil when none is queued.
func (s *Service) QueuedPrompt(sessionID string) (*types.QueuedPrompt, error) {
	return s.sessions.GetQueuedPrompt(sessionID)
}

// QueuedPrompts returns the whole queue, next-to-run first.
func (s *Service) QueuedPrompts(sessionID string) ([]types.QueuedPrompt, error) {
	return s.sessions.ListQueuedPrompts(sessionID)
}

// RemoveQueuedPrompt discards one queued prompt and reports whether it was
// queued.
func (s *Service) RemoveQueuedPrompt(sessionID, queueID string) (bool, error) {
	deleted, err := s.sessions.DeleteQueuedPrompt(sessionID, queueID)
	if err != nil {
		return false, err
	}
	s.mu.Lock()
	delete(s.pending, queueID)
	s.mu.Unlock()
	if deleted {
		s.broadcastQueue(sessionID, false)
	}
	return deleted, nil
}

// ClearQueuedPrompt discards every queued prompt. Returns true when any was
// queued.
func (s *Service) ClearQueuedPrompt(sessionID string) (bool, error) {
	queue, err := s.sessions.ListQueuedPrompts(sessionID)
	if err != nil {
		return false, err
	}
	if err := s.sessions.ClearQueuedPrompt(sessionID); err != nil {
		return false, err
	}
	s.mu.Lock()
	for _, qp := range queue {
		delete(s.pending, qp.ID)
	}
	s.mu.Unlock()
	if len(queue) > 0 {
		s.broadcastQueue(sessionID, false)
	}
	return len(queue) > 0, nil
}

// Steer halts the active run and queues dto at the front, so it starts as the
// next turn as soon as the aborted run settles. Returns false when no run is
// active.
func (s *Service) Steer(sessionID string, dto Prompt) (bool, error) {
	if !s.IsActive(sessionID) {
		return false, nil
	}
	qp, err := s.stage(sessionID, dto, true)
	if err != nil {
		return false, err
	}
	// Abort without clearing: the staged prompt must survive to drain.
	// cancelActive cancels under the service lock — reading ar.cancel after
	// unlocking would race execute()'s turn-boundary swap (turns.go) and
	// could cancel the wrong (not-yet-started) turn's context instead.
	if !s.cancelActive(sessionID) {
		// The run ended first: drop the prompt just staged, since the caller
		// is told no run was steered and would otherwise send it again.
		_, err := s.RemoveQueuedPrompt(sessionID, qp.ID)
		return false, err
	}
	s.decisions.RejectAllForSession(sessionID, "Run steered")
	slog.Info("run steered", "session", sessionID)
	return true, nil
}

// takeStaged takes the prompt that runs next, if any. The stored row is the
// source of truth, so a prompt queued before a restart still drains; the
// in-memory copy only restores fields the row does not keep.
func (s *Service) takeStaged(sessionID string) (Prompt, bool) {
	stored, err := s.sessions.PopQueuedPrompt(sessionID)
	if err != nil || stored == nil {
		return Prompt{}, false
	}
	s.mu.Lock()
	dto, ok := s.pending[stored.ID]
	delete(s.pending, stored.ID)
	s.mu.Unlock()
	if ok {
		return dto, true
	}
	out := Prompt{
		Text: stored.Prompt, ContextFiles: stored.ContextFiles, Annotations: stored.Annotations,
		ModelID: stored.ModelID, Provider: stored.Provider,
		ApprovalMode: stored.ApprovalMode,
	}
	for _, a := range stored.Attachments {
		out.Attachments = append(out.Attachments, Attachment{Data: a.Data, MimeType: a.MimeType})
	}
	return out, true
}

func activeHubLocked(active map[string]*activeRun, sessionID string) *Hub {
	if ar, ok := active[sessionID]; ok {
		return ar.hub
	}
	return nil
}
