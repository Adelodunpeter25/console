// Automation scanner: wakes sessions whose scheduled prompt is due. A fire is
// an ordinary prompt on the owning session (start now when idle, queue when
// busy), so it appears in the thread like any other turn.
package run

import (
	"errors"
	"log/slog"
	"time"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/services/automations"
)

// AutomationScanInterval is how often due schedules are swept. A fire is at
// most one tick late, which is fine for minute-scale schedules.
const AutomationScanInterval = 15 * time.Second

// SetAutomations attaches the automation store backing the console.automation
// tools and the scanner (nil-safe: without it the tools report unavailable).
func (s *Service) SetAutomations(store *automations.Service) {
	s.mu.Lock()
	s.automations = store
	s.mu.Unlock()
}

func (s *Service) automationStore() *automations.Service {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.automations
}

// RunAutomationScanner sweeps due automations until ctx is canceled. One
// goroutine per server; started after wiring in routes.go.
func (s *Service) RunAutomationScanner(ctx interface{ Done() <-chan struct{} }) {
	ticker := time.NewTicker(AutomationScanInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.fireDueAutomations(time.Now())
		}
	}
}

func (s *Service) fireDueAutomations(now time.Time) {
	store := s.automationStore()
	if store == nil {
		return
	}
	due, err := store.Due(now)
	if err != nil {
		slog.Warn("automation scan failed", "error", err)
		return
	}
	for _, a := range due {
		// Re-check inside the loop: a fire may take a while and the row can
		// be paused or deleted meanwhile.
		current, err := store.Get(a.ID)
		if err != nil || current == nil || current.Status != automations.StatusActive || current.NextRunAt > now.Unix() {
			continue
		}
		var fireErr error
		if _, err := s.promptSession(current.SessionID, current.Prompt, "", "", "", ""); err != nil {
			// An unknown session (deleted chat) is permanent; anything else
			// is worth one retry next tick.
			if errors.Is(err, ErrNoSession) {
				fireErr = err
			} else {
				slog.Warn("automation fire failed", "automation", current.ID, "error", err)
				continue
			}
		}
		if err := store.AfterFire(current.ID, fireErr, now); err != nil {
			slog.Warn("automation post-fire update failed", "automation", current.ID, "error", err)
		} else if fireErr == nil {
			slog.Info("automation fired", "automation", current.ID, "session", current.SessionID, "kind", current.Kind)
		}
	}
}
