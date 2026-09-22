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
	s.cancelActive(sessionID)
	s.mu.Lock()
	ar, ok := s.active[sessionID]
	if !ok || ar.hub != hub {
		s.mu.Unlock()
		return
	}
	delete(s.active, sessionID)
	s.mu.Unlock()
	slog.Error("run watchdog: no hub frames, force-settling run", "session", sessionID)
	hub.Close(OutcomeAborted)
	s.decisions.RejectAllForSession(sessionID, "Run watchdog timeout")
}

// removeActiveIf drops the session's active entry only when it still belongs
// to hub, so a force-settled run (or any replaced entry) can never delete a
// newer run's entry on its way out.
func (s *Service) removeActiveIf(sessionID string, hub *Hub) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if ar, ok := s.active[sessionID]; ok && ar.hub == hub {
		delete(s.active, sessionID)
	}
}

// cancelActive cancels the active run's current context while holding the
// service lock, so a concurrent turn-boundary swap can't strand the cancel
// on a stale context. Returns false when no run is active.
func (s *Service) cancelActive(sessionID string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	ar, ok := s.active[sessionID]
	if !ok {
		return false
	}
	ar.cancel()
	return true
}
