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
// The queue holds a single prompt, and staging replaces it. A prompt the user
// staged must not be destroyed by an agent, so an occupied slot is refused.
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

	if s.hasStaged(sessionID) {
		return false, fmt.Errorf("session %q already has a queued prompt; it would be replaced, so wait for it to run or clear it first", sessionID)
	}
	if _, err := s.QueuePrompt(sessionID, dto); err != nil {
		return false, mapPromptErr(sessionID, err)
	}
	// The run may have settled between the check and the staging: a staged
	// prompt is only drained by a run that is still going, so one left behind
	// would sit unseen until the next run. Take it back and start it now. If
	// the run drained it first, takeStaged finds nothing and we are done.
	if !s.IsActive(sessionID) {
		if staged, ok := s.takeStaged(sessionID); ok {
			if _, err := s.StartRun(sessionID, staged); err != nil && !errors.Is(err, ErrActive) {
				return false, mapPromptErr(sessionID, err)
			}
			return false, nil
		}
	}
	return true, nil
}

// hasStaged reports whether a prompt is waiting for the session's next turn.
func (s *Service) hasStaged(sessionID string) bool {
	if _, ok := s.pendingStaged(sessionID); ok {
		return true
	}
	stored, err := s.sessions.GetQueuedPrompt(sessionID)
	return err == nil && stored != nil
}

func mapPromptErr(sessionID string, err error) error {
	if errors.Is(err, ErrNoSession) {
		return fmt.Errorf("session %q not found", sessionID)
	}
	return err
}
