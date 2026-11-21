// Run watchdog: backstop against turns that stop emitting hub frames and
// ignore cancellation (wedged provider streams, unkillable children). Without
// it a stuck turn holds the session's active entry forever and every later
// run is rejected with ErrActive until the daemon restarts.
package run

import (
	"log/slog"
	"time"
)

// defaultWatchdogTimeout bounds hub silence well above the longest legitimate
// quiet stretch (decision waits resolve or time out at 10 minutes).
const defaultWatchdogTimeout = 15 * time.Minute

func (s *Service) watchdogTimeout() time.Duration {
	if s.WatchdogTimeout > 0 {
		return s.WatchdogTimeout
	}
	return defaultWatchdogTimeout
}

// watchRun force-settles a run whose hub goes silent past WatchdogTimeout.
// It returns when the run settles normally (done closes) or right after
// forcing — it never outlives the need.
func (s *Service) watchRun(sessionID string, hub *Hub, done <-chan struct{}) {
	timeout := s.watchdogTimeout()
	if timeout <= 0 {
		return
	}
	t := time.NewTimer(timeout)
	defer t.Stop()
	for {
		select {
		case <-done:
			return
		case <-t.C:
			remaining := timeout - hub.IdleSince()
			if remaining > 0 {
				// Frames arrived while the timer was pending; re-arm for
				// the rest instead of firing on stale silence.
				t.Reset(remaining)
				continue
			}
			s.forceSettle(sessionID, hub)
			return
		}
	}
}

// forceSettle best-effort cancels a silent run and, when it still holds the
// slot, closes its hub and drops the entry so later runs are not rejected
// forever. The delete is pointer-guarded and the hub close is idempotent, so
// a newer run on the same session — or the stuck turn finally returning —
// can never be disturbed: its broadcasts become safe no-ops.
func (s *Service) forceSettle(sessionID string, hub *Hub) {
	s.cancelActive(sessionID, errRunAborted)
	if !s.releaseSlot(sessionID, hub) {
		return
	}
	slog.Error("run watchdog: no hub frames, force-settling run", "session", sessionID)
	hub.Close(OutcomeAborted)
	s.decisions.RejectAllForSession(sessionID, "Run watchdog timeout")
}

// releaseSlot drops the session's active entry only when it still belongs
// to this run's hub, and reports whether this run held the slot. A newer run
// that already took the slot is left alone, so a run settling late can never
// disturb it.
func (s *Service) releaseSlot(sessionID string, hub *Hub) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	ar, ok := s.active[sessionID]
	if !ok || ar.hub != hub {
		return false
	}
	delete(s.active, sessionID)
	return true
}

// cancelActive cancels the active run's current context while holding the
// service lock, so a concurrent turn-boundary swap can't strand the cancel
// on a stale context. Returns the cancelled run's entry, or nil when no run
// is active.
func (s *Service) cancelActive(sessionID string, cause error) *activeRun {
	s.mu.Lock()
	defer s.mu.Unlock()
	ar, ok := s.active[sessionID]
	if !ok {
		return nil
	}
	ar.cancel(cause)
	return ar
}

// defaultAbortSettleTimeout bounds how long an aborted run may take to
// unwind before its session slot is force-released. A healthy abort settles
// in milliseconds; this only fires for runs wedged somewhere that ignores
// ctx (a provider stream or tool call that never returns).
const defaultAbortSettleTimeout = 20 * time.Second

func (s *Service) abortSettleTimeout() time.Duration {
	if s.AbortSettleTimeout != 0 {
		return s.AbortSettleTimeout
	}
	return defaultAbortSettleTimeout
}

// armAbortSettle force-releases the session slot when an aborted run does
// not unwind within AbortSettleTimeout. Abort only cancels; the execute
// goroutine owns releaseSlot, so without this a wedged run holds the session
// — and rejects every later StartRun with ErrActive — until the 15-minute
// hub watchdog. releaseSlot is pointer-guarded, so a run that settled (or a
// newer run that took the slot) is never disturbed.
func (s *Service) armAbortSettle(sessionID string, ar *activeRun) {
	timeout := s.abortSettleTimeout()
	if timeout <= 0 {
		return
	}
	go func() {
		timer := time.NewTimer(timeout)
		defer timer.Stop()
		select {
		case <-ar.done:
			return
		case <-timer.C:
		}
		if s.releaseSlot(sessionID, ar.hub) {
			slog.Warn("run abort: run did not settle, force-releasing session", "session", sessionID)
			ar.hub.Close(OutcomeAborted)
			s.decisions.RejectAllForSession(sessionID, "Run aborted")
		}
	}()
}
