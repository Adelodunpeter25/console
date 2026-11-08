package run

import (
	"errors"
	"fmt"
)

// PromptSession is promptSession for callers outside the package (tests and
// any other service that needs the same start-or-queue behaviour).
func (s *Service) PromptSession(sessionID, text, modelID, provider, approval, thinking string) (bool, error) {
	return s.promptSession(sessionID, text, modelID, provider, approval, thinking)
}

// promptSession delivers a follow-up prompt to an existing session on behalf
// of the console kernel. An idle session starts a run now; a busy one gets
// the prompt staged so it runs when the current run settles.
//
// A busy session queues the prompt behind any the user already staged.
func (s *Service) promptSession(sessionID, text, modelID, provider, approval, thinking string) (queued bool, err error) {
	dto := Prompt{Text: text, ModelID: modelID, Provider: provider, ApprovalMode: approval, Thinking: thinking}

	if !s.IsActive(sessionID) {
		_, err := s.StartRun(sessionID, dto)
		if err == nil {
			return false, nil
		}
		if !errors.Is(err, ErrActive) {
			return false, mapPromptErr(sessionID, err)
		}
		// A run started between the check and StartRun; fall through to queue.
	}

	if _, err := s.QueuePrompt(sessionID, dto); err != nil {
		return false, mapPromptErr(sessionID, err)
	}
	// The run may have settled between the check and the staging; start the
	// prompt now rather than leave it unseen until the next run.
	started, err := s.startStagedIfIdle(sessionID)
	if err != nil {
		return false, mapPromptErr(sessionID, err)
	}
	return !started, nil
}

func mapPromptErr(sessionID string, err error) error {
	if errors.Is(err, ErrNoSession) {
		// %w keeps the sentinel so automation fires can tell a deleted
		// session (permanent) from a transient failure.
		return fmt.Errorf("session %q not found: %w", sessionID, ErrNoSession)
	}
	return err
}

// consoleQueued reports the prompt staged for a session, for the console kernel.
func (s *Service) consoleQueued(sessionID string) (prompt, createdAt string, ok bool, err error) {
	stored, err := s.sessions.GetQueuedPrompt(sessionID)
	if err != nil || stored == nil {
		return "", "", false, err
	}
	return stored.Prompt, stored.CreatedAt, true, nil
}

// abortAndClear stops a session's run and discards any prompt staged behind it,
// matching POST /api/sessions/:id/abort. Cancelling alone is not enough: a
// cancelled run drains a staged prompt as its next turn, so the session would
// start working again right after being stopped.
func (s *Service) abortAndClear(sessionID string) bool {
	if !s.Abort(sessionID) {
		return false
	}
	_, _ = s.ClearQueuedPrompt(sessionID)
	return true
}

// AbortAndClear is abortAndClear for callers outside the package.
func (s *Service) AbortAndClear(sessionID string) bool { return s.abortAndClear(sessionID) }
