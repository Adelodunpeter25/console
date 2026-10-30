package run

import (
	"errors"
	"fmt"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/services/background"
	"log/slog"
	"strings"
)

// backgroundContext claims completed detached work immediately before a
// provider request. It is deliberately not persisted as a chat message and is
// never injected into a provider stream that is already in flight.
const backgroundContextOutputLimit = 12_000

func (s *Service) backgroundContext(sessionID string) string {
	work := s.BackgroundWorkManager()
	if work == nil {
		return ""
	}
	completed := work.TakeCompletions(sessionID)
	if len(completed) == 0 {
		return ""
	}
	return s.renderBackgroundWork(sessionID, completed)
}

func (s *Service) renderBackgroundWork(sessionID string, completed []background.Work) string {
	lines := make([]string, 0, len(completed)*6)
	for _, item := range completed {
		lines = append(lines, fmt.Sprintf("Background work completed: %s (%s)", item.ID, item.Kind))
		lines = append(lines, "Status: "+string(item.Status))
		if command := item.Metadata["command"]; command != "" {
			lines = append(lines, "Command: "+command)
		}
		if exitCode := item.Metadata["exit_code"]; exitCode != "" {
			lines = append(lines, "Exit code: "+exitCode)
		}
		if item.Summary != "" {
			lines = append(lines, "Summary: "+item.Summary)
		}
		if item.Error != "" {
			lines = append(lines, "Error: "+item.Error)
		}
		if name := item.Metadata["name"]; name != "" {
			lines = append(lines, "Name: "+name)
		}
		if jobID := item.Metadata["job_id"]; jobID != "" {
			if jobs := s.bashJobManager(); jobs != nil {
				if output, err := jobs.Output(jobID, sessionID, 0, backgroundContextOutputLimit); err == nil {
					if output.Stdout != "" {
						lines = append(lines, "stdout:\n"+output.Stdout)
					}
					if output.Stderr != "" {
						lines = append(lines, "stderr:\n"+output.Stderr)
					}
					if output.Truncated {
						lines = append(lines, "Output was truncated; use bashJob for the complete output.")
					}
				}
			}
			lines = append(lines, "Use bashJob with jobId=\""+jobID+"\" for complete shell output.")
		}
	}
	return strings.Join(lines, "\n")
}

// wakeIfIdle continues an idle session when finished background work is
// waiting to be delivered. While a run is active nothing is needed: the next
// provider request claims the completion. A lost race (a user turn started
// first) is fine for the same reason.
func (s *Service) wakeIfIdle(sessionID string) {
	work := s.BackgroundWorkManager()
	if work == nil || s.sessions == nil || s.IsActive(sessionID) || !work.HasUndelivered(sessionID) {
		return
	}
	if _, err := s.StartRun(sessionID, Prompt{Synthetic: true}); err != nil && !errors.Is(err, ErrActive) {
		slog.Warn("background wake failed", "session", sessionID, "error", err)
	}
}

// wakeContext claims the finished work and renders the synthetic user turn.
// It returns false when nothing is left to deliver.
func (s *Service) wakeContext(sessionID string) (string, []background.Work, bool) {
	work := s.BackgroundWorkManager()
	if work == nil {
		return "", nil, false
	}
	items := work.TakeCompletions(sessionID)
	if len(items) == 0 {
		return "", nil, false
	}
	return "Background work finished while you were idle. Review the results and continue the task.\n\n" + s.renderBackgroundWork(sessionID, items), items, true
}

// wakeMessage is the short human label carried on the wake event so clients
// can say what resumed the agent.
func wakeMessage(items []background.Work) string {
	shell, sub := false, false
	for _, item := range items {
		switch item.Kind {
		case background.KindShell:
			shell = true
		case background.KindSubagent:
			sub = true
		}
	}
	switch {
	case shell && sub:
		return "Background work completed, continuing agent"
	case sub:
		return "Subagent completed, continuing agent"
	default:
		return "Bash tool completed, continuing agent"
	}
}
