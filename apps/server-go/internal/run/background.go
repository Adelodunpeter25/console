package run

import (
	"fmt"
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
