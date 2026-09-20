// bashJob tool: manage background bash jobs started by the bash tool's
// background=true mode. Port of
// apps/server/agent/src/tools/bash/job-tool.ts.
package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/services"
)

type bashJobInput struct {
	Action string `json:"action" jsonschema:"required,description=status|output|wait|kill|list"`
	JobID  string `json:"jobId,omitempty" jsonschema:"description=background job id (job_...) — required except for list"`
	Cursor int    `json:"cursor,omitempty" jsonschema:"description=stdout offset from previous output call"`
	Limit  int    `json:"limit,omitempty" jsonschema:"description=max chars per stream (default 20000, max 50000)"`
	WaitMs int    `json:"waitMs,omitempty" jsonschema:"description=max wait for wait action (default 10000, max 120000)"`
}

func formatJobSnapshot(s services.BashJobSnapshot) string {
	lines := []string{
		fmt.Sprintf("Job: %s", s.JobID),
		fmt.Sprintf("Status: %s", s.Status),
		fmt.Sprintf("Command: %s", s.Command),
		fmt.Sprintf("Working directory: %s", s.Cwd),
		fmt.Sprintf("Started: %s", s.StartedAt),
	}
	if s.FinishedAt != "" {
		lines = append(lines, fmt.Sprintf("Finished: %s", s.FinishedAt))
	}
	if s.ExitCode != nil {
		lines = append(lines, fmt.Sprintf("Exit code: %d", *s.ExitCode))
	}
	if s.TimedOut {
		lines = append(lines, fmt.Sprintf("Timed out: true (max %dms)", services.BashJobMaxTimeoutMs))
	}
	if s.Aborted {
		lines = append(lines, "Killed: true")
	}
	return strings.Join(lines, "\n")
}

func renderJobOutputBody(jobs *services.BashJobManager, jobID, ownerSessionID string, cursor, limit int) (string, error) {
	r, err := jobs.Output(jobID, ownerSessionID, cursor, limit)
	if err != nil {
		return "", err
	}
	sections := []string{
		fmt.Sprintf("cursor: %d -> nextCursor: %d", cursor, r.NextCursor),
		fmt.Sprintf("truncated: %v", r.Truncated),
		"", "stdout:",
	}
	if r.Stdout != "" {
		sections = append(sections, r.Stdout)
	} else {
		sections = append(sections, "(no new output)")
	}
	if strings.TrimSpace(r.Stderr) != "" {
		sections = append(sections, "", "stderr:", r.Stderr)
	}
	sections = append(sections, "", fmt.Sprintf("Poll again with cursor=%d for new output.", r.NextCursor))
	return strings.Join(sections, "\n"), nil
}

// NewBashJobTool builds the "bashJob" tool bound to jobs, scoped to
// ownerSessionID (must match the session that started the job via bash's
// background=true, matching the TS ownership check).
func NewBashJobTool(jobs *services.BashJobManager, ownerSessionID string) Tool {
	return NewTool("bashJob", "Manage background bash jobs. action: status|output|wait|kill|list. output is cursor-paginated (nextCursor, truncated).", TierExec,
		func(ctx context.Context, in bashJobInput) (any, error) {
			if jobs == nil {
				return nil, NewToolError("Background jobs are not available in this context.")
			}
			switch in.Action {
			case "list":
				snapshots := jobs.List(ownerSessionID)
				if len(snapshots) == 0 {
					return "No background bash jobs.", nil
				}
				lines := make([]string, len(snapshots))
				for i, s := range snapshots {
					lines[i] = formatJobSnapshot(s)
				}
				return strings.Join(lines, "\n\n---\n\n"), nil

			case "status":
				if in.JobID == "" {
					return nil, NewToolError(`Missing jobId for action="status".`)
				}
				s, err := jobs.Status(in.JobID, ownerSessionID)
				if err != nil {
					return nil, NewToolError("%v", err)
				}
				if s.Status == services.BashJobFailed || s.Status == services.BashJobExpired {
					return nil, NewToolError("%s", formatJobSnapshot(s))
				}
				return formatJobSnapshot(s), nil

			case "output":
				if in.JobID == "" {
					return nil, NewToolError(`Missing jobId for action="output".`)
				}
				s, err := jobs.Status(in.JobID, ownerSessionID)
				if err != nil {
					return nil, NewToolError("%v", err)
				}
				body, err := renderJobOutputBody(jobs, in.JobID, ownerSessionID, in.Cursor, in.Limit)
				if err != nil {
					return nil, NewToolError("%v", err)
				}
				return formatJobSnapshot(s) + "\n" + body, nil

			case "wait":
				if in.JobID == "" {
					return nil, NewToolError(`Missing jobId for action="wait".`)
				}
				waitMs := in.WaitMs
				if waitMs <= 0 {
					waitMs = 10_000
				}
				s, err := jobs.Wait(in.JobID, ownerSessionID, waitMs)
				if err != nil {
					return nil, NewToolError("%v", err)
				}
				body, err := renderJobOutputBody(jobs, in.JobID, ownerSessionID, in.Cursor, in.Limit)
				if err != nil {
					return nil, NewToolError("%v", err)
				}
				suffix := ""
				if s.Status == services.BashJobRunning {
					suffix = fmt.Sprintf("\n\nStill running after wait. Poll again or wait longer (max %dms).", services.BashJobMaxWaitMs)
				}
				result := formatJobSnapshot(s) + "\n" + body + suffix
				if s.Status == services.BashJobFailed || s.Status == services.BashJobExpired {
					return nil, NewToolError("%s", result)
				}
				return result, nil

			case "kill":
				if in.JobID == "" {
					return nil, NewToolError(`Missing jobId for action="kill".`)
				}
				s, err := jobs.Kill(in.JobID, ownerSessionID)
				if err != nil {
					return nil, NewToolError("%v", err)
				}
				return formatJobSnapshot(s) + "\n\nJob killed; process tree terminated.", nil

			default:
				return nil, NewToolError("Unknown action: %s", in.Action)
			}
		})
}

// BashJob is the unbound default instance used by DefaultTools() (nil
// manager — errors on every call; a real run binds its own via
// NewBashJobTool sharing the same manager passed to NewBashTool).
var BashJob = NewBashJobTool(nil, "")
