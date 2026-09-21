// bash / bashJob tools: run a shell command synchronously (with timeout,
// output truncation) or as a pollable background job. Ports of
// apps/server/agent/src/tools/bash/{bash.ts,job-tool.ts}.
package tools

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/services"
)

const (
	bashMaxOutputBytes = 50 * 1024
	bashDefaultTimeout = 30_000
	bashMaxTimeoutMs   = 20 * 60 * 1000
)

type bashInput struct {
	Command    string            `json:"command" jsonschema:"required,description=Shell command to execute"`
	Cwd        string            `json:"cwd,omitempty" jsonschema:"description=Working directory for the command. Defaults to the current directory."`
	TimeoutMs  int               `json:"timeoutMs,omitempty" jsonschema:"description=Timeout in milliseconds. Sync mode: time the tool call waits (default 30s). Background mode: max job lifetime (default 10min). Max: 20min."`
	Env        map[string]string `json:"env,omitempty" jsonschema:"description=Additional environment variables for this command"`
	Background bool              `json:"background,omitempty" jsonschema:"description=Start as a background job and return immediately with a jobId. Poll with bashJob (status/output/wait/kill). The start response is NOT an exit result."`
}

func truncateOutput(output string, maxBytes int, label string) string {
	if len(output) <= maxBytes {
		return output
	}
	return output[:maxBytes] + fmt.Sprintf("\n\n[... %s truncated: %d bytes total, showing first %d bytes ...]", label, len(output), maxBytes)
}

func mergedEnv(overrides map[string]string) []string {
	env := os.Environ()
	if len(overrides) == 0 {
		return env
	}
	out := make([]string, 0, len(env)+len(overrides))
	out = append(out, env...)
	for k, v := range overrides {
		out = append(out, k+"="+v)
	}
	return out
}

func shellArgv(command string) []string {
	return []string{"/bin/sh", "-c", command}
}

// NewBashTool builds the "bash" tool bound to jobs (for background=true
// runs). A nil manager disables background mode (headless singleton).
func NewBashTool(jobs *services.BashJobManager, ownerSessionID string) Tool {
	return NewTool("bash", "Run a shell command and return output. Use for builds, tests, or git — not for reading or editing files. Long-running commands: background=true starts a managed job (poll with bashJob); the start response only confirms the process started.", TierExec,
		func(ctx context.Context, in bashInput) (any, error) {
			if in.Command == "" {
				return nil, NewToolError("command is required")
			}
			cwd := in.Cwd
			if cwd != "" {
				if abs, err := filepath.Abs(cwd); err == nil {
					cwd = abs
				}
			} else if wd, err := os.Getwd(); err == nil {
				cwd = wd
			}

			if in.Background {
				if jobs == nil {
					return nil, NewToolError("Background jobs are not available in this context.")
				}
				timeoutMs := in.TimeoutMs
				if timeoutMs <= 0 {
					timeoutMs = services.BashJobDefaultTimeoutMs
				}
				snap, err := jobs.Start(services.StartBashJobOptions{
					Command: in.Command, Cwd: cwd, Env: mergedEnv(in.Env),
					TimeoutMs: timeoutMs, OwnerSessionID: ownerSessionID,
				})
				if err != nil {
					return nil, NewToolError("%v", err)
				}
				return textResult(fmt.Sprintf(
					"Started: %s\nMax lifetime: %dms\nDefault wait: %dms\n\nUse bashJob action=\"output|wait|kill\" with jobId=%q.\nThis response is not an exit result — poll for status before assuming success.",
					snap.JobID, timeoutMs, services.BashJobMaxWaitMs, snap.JobID,
				)), nil
			}

			timeoutMs := in.TimeoutMs
			if timeoutMs <= 0 {
				timeoutMs = bashDefaultTimeout
			}
			res := services.SpawnCapture(ctx, shellArgv(in.Command), services.SpawnCaptureOptions{
				Cwd: cwd, Env: mergedEnv(in.Env), TimeoutMs: timeoutMs,
			})

			if res.Aborted {
				return nil, NewToolError(
					"Command cancelled by user abort.\nCommand: %s\nWorking directory: %s\n\nPartial stdout:\n%s\n\nPartial stderr:\n%s",
					in.Command, cwd, truncateOutput(res.Stdout, bashMaxOutputBytes, "stdout"), truncateOutput(res.Stderr, bashMaxOutputBytes, "stderr"))
			}
			if res.Killed {
				return nil, NewToolError(
					"Command timed out after %dms.\nCommand: %s\nWorking directory: %s\n\nPartial stdout:\n%s\n\nPartial stderr:\n%s",
					timeoutMs, in.Command, cwd, truncateOutput(res.Stdout, bashMaxOutputBytes, "stdout"), truncateOutput(res.Stderr, bashMaxOutputBytes, "stderr"))
			}

			stdout := truncateOutput(res.Stdout, bashMaxOutputBytes, "stdout")
			stderr := truncateOutput(res.Stderr, bashMaxOutputBytes, "stderr")
			sections := []string{fmt.Sprintf("Exit code: %d", res.ExitCode), fmt.Sprintf("Working directory: %s", cwd)}
			if strings.TrimSpace(stdout) != "" {
				sections = append(sections, "", "stdout:", stdout)
			} else {
				sections = append(sections, "stdout: (empty)")
			}
			if strings.TrimSpace(stderr) != "" {
				sections = append(sections, "", "stderr:", stderr)
			}
			result := strings.Join(sections, "\n")
			return Envelope{Content: textResult(result), IsError: res.ExitCode != 0}, nil
		})
}

// Bash is the unbound default instance used by DefaultTools() (background
// mode disabled — a real run binds its own via NewBashTool with a shared
// BashJobManager and session id).
var Bash = NewBashTool(nil, "")
