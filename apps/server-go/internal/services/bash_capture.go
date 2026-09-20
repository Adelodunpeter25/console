// Synchronous subprocess runner (bash tool's non-background path). Port of
// apps/server/api/src/utils/exec.ts spawnCapture: captures stdout/stderr
// with a hard byte cap, keeps draining past the cap so the child still
// exits normally, and tree-kills on timeout or context cancellation.
package services

import (
	"context"
	"io"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"time"
)

const bashCaptureDefaultMaxBytes = 10 * 1024 * 1024

// SpawnCaptureOptions configures one synchronous command run.
type SpawnCaptureOptions struct {
	Cwd       string
	Env       []string
	TimeoutMs int
	MaxBytes  int
}

// SpawnCaptureResult is the outcome of one synchronous command run.
type SpawnCaptureResult struct {
	Stdout   string
	Stderr   string
	ExitCode int
	Killed   bool // terminated by timeout or ctx cancellation
	Aborted  bool // specifically terminated by ctx cancellation
}

// readCapped drains r fully, retaining up to maxBytes. Early termination
// (timeout/ctx cancel) happens via killing the process, which closes the
// pipe and unblocks Read with EOF — not via a cancellation channel here.
func readCapped(r io.Reader, maxBytes int) string {
	decoder := newUTF8Decoder()
	buf := make([]byte, 32*1024)
	var out strings.Builder
	total := 0
	for {
		n, err := r.Read(buf)
		if n > 0 {
			chunk := buf[:n]
			prevTotal := total
			total += n
			if prevTotal < maxBytes {
				keep := maxBytes - prevTotal
				if keep > len(chunk) {
					keep = len(chunk)
				}
				out.WriteString(decoder.decode(chunk[:keep]))
			}
			// Over budget: keep draining without retaining, so the pipe
			// never blocks and the child can still exit normally.
		}
		if err != nil {
			return out.String()
		}
	}
}

// SpawnCapture runs argv synchronously, returning captured output and the
// exit code. ctx cancellation tree-kills the process and marks Aborted.
func SpawnCapture(ctx context.Context, argv []string, opts SpawnCaptureOptions) SpawnCaptureResult {
	maxBytes := opts.MaxBytes
	if maxBytes <= 0 {
		maxBytes = bashCaptureDefaultMaxBytes
	}
	cmd := exec.Command(argv[0], argv[1:]...)
	cmd.Dir = opts.Cwd
	cmd.Env = opts.Env
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return SpawnCaptureResult{Stderr: "[Process error: " + err.Error() + "]", ExitCode: 1}
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return SpawnCaptureResult{Stderr: "[Process error: " + err.Error() + "]", ExitCode: 1}
	}
	if err := cmd.Start(); err != nil {
		return SpawnCaptureResult{Stderr: "[Process error: " + err.Error() + "]", ExitCode: 1}
	}

	done := make(chan struct{})
	var timedOut, aborted bool
	var mu sync.Mutex

	terminate := func() {
		if cmd.Process == nil {
			return
		}
		pid := cmd.Process.Pid
		safeKillGroup(pid)
	}

	var timer *time.Timer
	if opts.TimeoutMs > 0 {
		timer = time.AfterFunc(time.Duration(opts.TimeoutMs)*time.Millisecond, func() {
			mu.Lock()
			timedOut = true
			mu.Unlock()
			terminate()
		})
	}

	ctxDone := ctx.Done()
	go func() {
		select {
		case <-ctxDone:
			mu.Lock()
			aborted = true
			mu.Unlock()
			terminate()
		case <-done:
		}
	}()

	var stdoutText, stderrText string
	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); stdoutText = readCapped(stdout, maxBytes) }()
	go func() { defer wg.Done(); stderrText = readCapped(stderr, maxBytes) }()
	wg.Wait()
	close(done)
	if timer != nil {
		timer.Stop()
	}

	waitErr := cmd.Wait()
	exitCode := 0
	if waitErr != nil {
		if exitErr, ok := waitErr.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			exitCode = 1
		}
	}

	mu.Lock()
	defer mu.Unlock()
	return SpawnCaptureResult{
		Stdout: stdoutText, Stderr: stderrText, ExitCode: exitCode,
		Killed: timedOut || aborted, Aborted: aborted,
	}
}
