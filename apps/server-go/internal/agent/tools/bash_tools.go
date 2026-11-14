// bash / bashJob tools: run a shell command synchronously (with timeout,
// output truncation) or as a pollable background job.
package tools

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/providers/github"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/services/integrations"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/services/terminal"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/utils"
)

const (
	bashMaxOutputBytes = 50 * 1024
	bashDefaultTimeout = 30_000
	bashMaxTimeoutMs   = 20 * 60 * 1000
)

type bashInput struct {
	Command      string            `json:"command" jsonschema:"required,description=Shell command to execute"`
	Cwd          string            `json:"cwd,omitempty" jsonschema:"description=Working directory for the command. Defaults to the project directory."`
	TimeoutMs    int               `json:"timeoutMs,omitempty" jsonschema:"description=Foreground: how long to wait for the command (default 30s, max 20min); if it is still running then, it keeps running as a background job and you are notified when it finishes. Background: optional cap on the job lifetime; omit it for dev servers and other open-ended jobs."`
	Env          map[string]string `json:"env,omitempty" jsonschema:"description=Additional environment variables for this command"`
	Background   bool              `json:"background,omitempty" jsonschema:"description=Start as a background job and return immediately with a jobId. You are notified automatically with the exit code and output when it finishes, so do not poll or wait: keep working on other things, or end your turn and tell the user what you are waiting on. The start response is NOT an exit result."`
	ParentRunID  string            `json:"-"`
	ParentCallID string            `json:"-"`
}

func truncateOutput(output string, maxBytes int, label string) string {
	if len(output) <= maxBytes {
		return output
	}
	return output[:maxBytes] + fmt.Sprintf("\n\n[... %s truncated: %d bytes total, showing first %d bytes ...]", label, len(output), maxBytes)
}

// bashSection renders one stdout/stderr section. Output over the cap is
// spilled raw to a per-session file and the tail is shown (errors print at
// the end), with secrets scrubbed before anything is written or returned.
func bashSection(sessionID, label, output string) string {
	redacted := github.RedactSecrets(output)
	if len(redacted) <= bashMaxOutputBytes {
		return cleanCommandOutput(redacted)
	}
	path, err := utils.SpillFile(utils.SpillDir(sessionID), "bash", "."+label+".log", redacted)
	if err != nil {
		return truncateOutput(cleanCommandOutput(redacted), bashMaxOutputBytes, label)
	}
	return fmt.Sprintf("%s: %s, %d lines, full output saved to %s\n--- last %s ---\n%s",
		label, utils.HumanBytes(len(redacted)), countLines(redacted), path,
		utils.HumanBytes(bashMaxOutputBytes), cleanCommandOutput(utils.TailOutput(redacted, bashMaxOutputBytes)))
}

var (
	ansiCSIRe    = regexp.MustCompile("\x1b\\[[0-9;?]*[ -/]*[@-~]")
	ansiOSCRe    = regexp.MustCompile("\x1b\\][^\x07\x1b]*(?:\x07|\x1b\\\\)")
	ansiSimpleRe = regexp.MustCompile("\x1b[@-Z\\-_]")
)

// cleanCommandOutput strips ANSI escapes and collapses carriage-return
// progress redraws (keep the last frame of each line) before the text
// reaches the model. The spill file keeps the raw bytes.
func cleanCommandOutput(output string) string {
	if !strings.ContainsAny(output, "\x1b\r") {
		return output
	}
	out := ansiOSCRe.ReplaceAllString(output, "")
	out = ansiCSIRe.ReplaceAllString(out, "")
	out = ansiSimpleRe.ReplaceAllString(out, "")
	if strings.Contains(out, "\r") {
		lines := strings.Split(out, "\n")
		for i, line := range lines {
			if idx := strings.LastIndex(line, "\r"); idx >= 0 {
				lines[i] = line[idx+1:]
			}
		}
		out = strings.Join(lines, "\n")
	}
	return out
}

func mergedEnv(overrides map[string]string) []string {
	env := os.Environ()
	if len(overrides) == 0 {
		// GH_TOKEN lets the model drive `gh` with the stored PAT instead of
		// an interactive browser login, which a headless box cannot do. The
		// user bin dir lets a server-installed `gh` (~/.local/bin) resolve
		// even when the daemon runs with a thin PATH.
		return github.AppendToEnv(integrations.EnsureUserBinDir(env))
	}
	out := make([]string, 0, len(env)+len(overrides))
	out = append(out, env...)
	for k, v := range overrides {
		out = append(out, k+"="+v)
	}
	return github.AppendToEnv(integrations.EnsureUserBinDir(out))
}

func shellArgv(command string) []string {
	return []string{"/bin/sh", "-c", command}
}

// NewBashTool builds the "bash" tool bound to jobs (for background=true
// runs). A nil manager disables background mode (headless singleton).
func NewBashTool(jobs *terminal.BashJobManager, ownerSessionID string, parentIDs ...string) Tool {
	return NewSandboxedBashTool(SandboxOff, jobs, ownerSessionID, parentIDs...)
}

// NewBashToolWithCwd builds the "bash" tool with a session default working directory.
func NewBashToolWithCwd(jobs *terminal.BashJobManager, defaultCwd, ownerSessionID string, parentIDs ...string) Tool {
	return NewSandboxedBashToolWithCwd(SandboxOff, jobs, defaultCwd, ownerSessionID, parentIDs...)
}

// NewSandboxedBashTool is NewBashTool for a given sandbox mode. Plan mode passes
// SandboxReadOnly. Background jobs are refused in that mode: a job outlives the
// call, which is hard to justify in a read-only plan, and the job manager is
// not sandbox-aware.
func NewSandboxedBashTool(mode SandboxMode, jobs *terminal.BashJobManager, ownerSessionID string, parentIDs ...string) Tool {
	return NewSandboxedBashToolWithCwd(mode, jobs, "", ownerSessionID, parentIDs...)
}

// NewSandboxedBashToolWithCwd is NewSandboxedBashTool with a default working directory fallback.
func NewSandboxedBashToolWithCwd(mode SandboxMode, jobs *terminal.BashJobManager, defaultCwd, ownerSessionID string, parentIDs ...string) Tool {
	parentRunID, parentCallID := "", ""
	if len(parentIDs) > 0 {
		parentRunID = parentIDs[0]
	}
	if len(parentIDs) > 1 {
		parentCallID = parentIDs[1]
	}
	return NewTool("bash", "Run a shell command and return output. For file access prefer read_file/grep/editFile. Long-running commands (builds, test suites, dev servers): background=true returns immediately and the result is delivered when it finishes. A foreground command still running at its timeout keeps running as a background job — you're notified when it finishes.", TierExec,
		func(ctx context.Context, in bashInput) (any, error) {
			if in.Command == "" {
				return nil, NewToolError("command is required")
			}
			cwd := in.Cwd
			if cwd != "" {
				if !filepath.IsAbs(cwd) && defaultCwd != "" {
					cwd = filepath.Join(defaultCwd, cwd)
				} else if abs, err := filepath.Abs(cwd); err == nil {
					cwd = abs
				}
			} else if defaultCwd != "" {
				cwd = defaultCwd
			} else if wd, err := os.Getwd(); err == nil {
				cwd = wd
			}

			if in.Background {
				if mode == SandboxReadOnly {
					return nil, NewToolError("Background jobs are not available in plan mode. Run the command in the foreground, or describe it in your plan.")
				}
				if jobs == nil {
					return nil, NewToolError("Background jobs are not available in this context.")
				}
				snap, err := jobs.Start(terminal.StartBashJobOptions{
					Command: in.Command, Cwd: cwd, Env: mergedEnv(in.Env),
					TimeoutMs: in.TimeoutMs, OwnerSessionID: ownerSessionID,
					ParentRunID: parentRunID, ParentCallID: parentCallID,
				})
				if err != nil {
					return nil, NewToolError("%v", err)
				}
				workLine := ""
				if snap.WorkID != "" {
					workLine = fmt.Sprintf("\nBackground work: %s", snap.WorkID)
				}
				return textResult(backgroundStartedText(snap.JobID, workLine, in.TimeoutMs, "")), nil
			}

			timeoutMs := in.TimeoutMs
			if timeoutMs <= 0 {
				timeoutMs = bashDefaultTimeout
			}
			if timeoutMs > bashMaxTimeoutMs {
				timeoutMs = bashMaxTimeoutMs
			}
			var res terminal.SpawnCaptureResult
			if jobs != nil && mode != SandboxReadOnly {
				snap, promoted, runRes, err := runForeground(ctx, jobs, in, cwd, ownerSessionID, parentRunID, parentCallID, timeoutMs)
				if err != nil {
					return nil, NewToolError("%v", err)
				}
				if promoted {
					workLine := ""
					if snap.WorkID != "" {
						workLine = fmt.Sprintf("\nBackground work: %s", snap.WorkID)
					}
					return textResult(backgroundStartedText(snap.JobID, workLine, 0,
						fmt.Sprintf("The command was still running after %dms, so it was moved to the background instead of being killed.\n", timeoutMs))), nil
				}
				res = runRes
			} else {
				res = terminal.SpawnCapture(ctx, sandboxedShellArgv(in.Command, mode), terminal.SpawnCaptureOptions{
					Cwd: cwd, Env: mergedEnv(in.Env), TimeoutMs: timeoutMs,
				})
			}

			if res.Aborted {
				return nil, NewToolError(
					"Command cancelled by user abort.\nCommand: %s\nWorking directory: %s\n\nPartial stdout:\n%s\n\nPartial stderr:\n%s",
					in.Command, cwd, bashSection(ownerSessionID, "stdout", res.Stdout), bashSection(ownerSessionID, "stderr", res.Stderr))
			}
			if res.Killed {
				hint := ""
				if mode == SandboxReadOnly {
					hint = "\n\n" + planModeTimeoutHint
				}
				return nil, NewToolError(
					"Command timed out after %dms.\nCommand: %s\nWorking directory: %s\n\nPartial stdout:\n%s\n\nPartial stderr:\n%s%s",
					timeoutMs, in.Command, cwd, bashSection(ownerSessionID, "stdout", res.Stdout), bashSection(ownerSessionID, "stderr", res.Stderr), hint)
			}

			// bashSection redacts before spilling: the child inherits
			// GH_TOKEN, so its output may echo the secret (`env`,
			// `printenv GH_TOKEN`) — scrub the exact token before it can
			// reach the transcript or the spill file.
			stdout := bashSection(ownerSessionID, "stdout", res.Stdout)
			stderr := bashSection(ownerSessionID, "stderr", res.Stderr)
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
			if mode == SandboxReadOnly && res.ExitCode != 0 && looksLikeBlockedWrite(res.Stderr) && SandboxAvailable() {
				result += sandboxHint
			}
			return Envelope{Content: textResult(result), IsError: res.ExitCode != 0}, nil
		})
}

const planModeTimeoutHint = "This command is too slow for plan mode, which can't run background jobs. Narrow it (for example, limit the paths or output), or describe it in your plan and run it after the plan is approved."

func backgroundStartedText(jobID, workLine string, timeoutMs int, prefix string) string {
	limit := "none (runs until it exits or is killed)"
	if timeoutMs > 0 {
		limit = fmt.Sprintf("%dms", timeoutMs)
	}
	return fmt.Sprintf(
		"%sStarted: %s%s\nTime limit: %s\n\nThe job runs in the background. You will be notified automatically with its exit code and output when it finishes — do not poll or wait for it. Continue with other work, or end your turn and tell the user what you are waiting on.\nThis response is not an exit result. Use bashJob (jobId=%q) only to peek at partial output or to kill it.",
		prefix, jobID, workLine, limit, jobID)
}

// runForeground runs the command as a silent job and waits up to timeoutMs.
// A command still running then is promoted to a background job (promoted=true)
// rather than killed; one that settles is collected and removed.
func runForeground(ctx context.Context, jobs *terminal.BashJobManager, in bashInput, cwd, owner, runID, callID string, timeoutMs int) (terminal.BashJobSnapshot, bool, terminal.SpawnCaptureResult, error) {
	var none terminal.SpawnCaptureResult
	snap, err := jobs.Start(terminal.StartBashJobOptions{
		Command: in.Command, Cwd: cwd, Env: mergedEnv(in.Env), OwnerSessionID: owner, Foreground: true,
	})
	if err != nil {
		return snap, false, none, err
	}
	snap, err = jobs.WaitSettled(ctx, snap.JobID, owner, time.Duration(timeoutMs)*time.Millisecond)
	aborted := err != nil
	if aborted {
		_, _ = jobs.Kill(snap.JobID, owner)
		snap, _ = jobs.WaitSettled(context.Background(), snap.JobID, owner, 2*time.Second)
	} else if snap.Status == terminal.BashJobRunning {
		promotedSnap, ok, perr := jobs.Promote(snap.JobID, owner, runID, callID)
		if perr != nil {
			_, _ = jobs.Kill(snap.JobID, owner)
			return snap, false, none, perr
		}
		if ok {
			return promotedSnap, true, none, nil
		}
		snap = promotedSnap
	}
	out, cerr := jobs.Collect(snap.JobID, owner)
	if cerr != nil {
		return snap, false, none, cerr
	}
	code := 0
	if out.Snapshot.ExitCode != nil {
		code = *out.Snapshot.ExitCode
	}
	return out.Snapshot, false, terminal.SpawnCaptureResult{
		Stdout: out.Stdout, Stderr: out.Stderr, ExitCode: code, Killed: aborted, Aborted: aborted,
	}, nil
}

// Bash is the unbound default instance used by DefaultTools() (background
// mode disabled — a real run binds its own via NewBashTool with a shared
// BashJobManager and session id).
var Bash = NewBashTool(nil, "")
