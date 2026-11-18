// Automation scanner: wakes sessions whose scheduled prompt is due. A fire is
// a tagged prompt on the owning session (start now when idle, queue when
// busy), so it appears in the thread with full context.
package run

import (
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/services/automations"
)

// AutomationScanInterval is how often due schedules are swept. A fire is at
// most one tick late, which is fine for minute-scale schedules.
const AutomationScanInterval = 15 * time.Second

const maxTransientAutomationFailures = 3

// SetAutomations attaches the automation store backing the console.automation
// tools and the scanner (nil-safe: without it the tools report unavailable).
func (s *Service) SetAutomations(store *automations.Service) {
	s.mu.Lock()
	s.automations = store
	manager := s.consoleManager
	s.mu.Unlock()
	if manager != nil {
		manager.SetAutomations(store)
	}
}

func (s *Service) automationStore() *automations.Service {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.automations
}

// StartAutomationScanner runs an immediate sweep and then sweeps due
// automations every AutomationScanInterval until the returned stop func is
// called.
func (s *Service) StartAutomationScanner() func() {
	stop := make(chan struct{})
	done := make(chan struct{})
	var once sync.Once
	failures := map[string]int{}

	go func() {
		defer close(done)
		s.fireDueAutomations(time.Now(), failures)
		ticker := time.NewTicker(AutomationScanInterval)
		defer ticker.Stop()
		for {
			select {
			case <-stop:
				return
			case <-ticker.C:
				s.fireDueAutomations(time.Now(), failures)
			}
		}
	}()

	return func() {
		once.Do(func() {
			close(stop)
			<-done
		})
	}
}

func (s *Service) fireDueAutomations(now time.Time, failures map[string]int) {
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
		if _, err := s.promptSessionWithSource(current.SessionID, current.Prompt, "automation", "", "", "", ""); err != nil {
			if errors.Is(err, ErrNoSession) {
				fireErr = err
			} else {
				failures[current.ID]++
				slog.Warn("automation fire failed", "automation", current.ID, "attempt", failures[current.ID], "error", err)
				if failures[current.ID] < maxTransientAutomationFailures {
					continue
				}
				fireErr = err
			}
		}
		delete(failures, current.ID)
		if err := store.AfterFire(current.ID, fireErr, now); err != nil {
			slog.Warn("automation post-fire update failed", "automation", current.ID, "error", err)
		} else if fireErr == nil {
			slog.Info("automation fired", "automation", current.ID, "session", current.SessionID, "kind", current.Kind)
		}
	}
}
