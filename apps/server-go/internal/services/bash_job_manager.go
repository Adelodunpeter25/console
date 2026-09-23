// Background bash job manager. Port of
// apps/server/agent/src/tools/bash/manager.ts: one child shell per job in
// its own process group (tree-kill via negative pid), output ring-buffered
// per stream, cursor-paginated reads, retention after completion.
package services

import (
	"fmt"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"time"
)

const (
	BashJobDefaultTimeoutMs = 10 * 60 * 1000
	BashJobMaxTimeoutMs     = 20 * 60 * 1000
	bashJobMaxOutputChars   = 256 * 1024
	bashJobMaxRunning       = 10
	bashJobMaxRetained      = 50
	bashJobRetention        = 30 * time.Minute
	BashJobMaxWaitMs        = 120 * 1000
)

type BashJobStatus string

const (
	BashJobRunning BashJobStatus = "running"
	BashJobExited  BashJobStatus = "exited"
	BashJobFailed  BashJobStatus = "failed"
	BashJobKilled  BashJobStatus = "killed"
	BashJobExpired BashJobStatus = "expired"
)

// BashJobSnapshot is the tool/route-facing view of a job (no live process
// handle or buffers).
type BashJobSnapshot struct {
	JobID      string
	Command    string
	Cwd        string
	Status     BashJobStatus
	ExitCode   *int
	StartedAt  string
	FinishedAt string
	TimedOut   bool
	Aborted    bool
}

// BashJobOutput is one paginated read of a job's buffered output.
type BashJobOutput struct {
	Snapshot   BashJobSnapshot
	Stdout     string
	Stderr     string
	NextCursor int
	Truncated  bool
}

type bashJobRecord struct {
	BashJobSnapshot
	mu                           sync.Mutex
	cmd                          *exec.Cmd
	stdout                       strings.Builder
	stderr                       strings.Builder
	stdoutTotal, stderrTotal     int
	stdoutDropped, stderrDropped int
	stdoutDone, stderrDone       bool
	exited                       bool
	rawExit                      int
	timer                        *time.Timer
	retentionTimer               *time.Timer
	waiters                      []chan struct{}
	ownerSessionID               string
}

// BashJobManager owns every background bash job for the process.
type BashJobManager struct {
	mu   sync.Mutex
	jobs map[string]*bashJobRecord

	// ports backs the same live dev-server detection as pty.manager.ts:
	// job output is scanned for localhost:PORT candidates, and the job's
	// owner entry is cleaned up once it settles. Nil disables detection
	// (e.g. tests).
	ports *PortRegistry
}

func NewBashJobManager(ports *PortRegistry) *BashJobManager {
	return &BashJobManager{jobs: make(map[string]*bashJobRecord), ports: ports}
}

func (m *BashJobManager) runningCount() int {
	n := 0
	for _, j := range m.jobs {
		j.mu.Lock()
		if j.Status == BashJobRunning {
			n++
		}
		j.mu.Unlock()
	}
	return n
}

// StartBashJobOptions configures one background job.
type StartBashJobOptions struct {
	Command        string
	Cwd            string
	Env            []string
	TimeoutMs      int
	OwnerSessionID string
}

// Start launches command as a detached background job and returns its
// snapshot immediately (the caller polls status/output/wait).
func (m *BashJobManager) Start(opts StartBashJobOptions) (BashJobSnapshot, error) {
	m.mu.Lock()
	if m.runningCount() >= bashJobMaxRunning {
		m.mu.Unlock()
		return BashJobSnapshot{}, fmt.Errorf("Too many running background jobs (max %d). Kill a job with bashJob action=\"kill\" first.", bashJobMaxRunning)
	}
	if len(m.jobs) >= bashJobMaxRetained {
		var oldest *bashJobRecord
		for _, j := range m.jobs {
			j.mu.Lock()
			done := j.Status != BashJobRunning
			started := j.StartedAt
			j.mu.Unlock()
			if done && (oldest == nil || started < oldest.StartedAt) {
				oldest = j
			}
		}
		if oldest != nil {
			delete(m.jobs, oldest.JobID)
		} else {
			m.mu.Unlock()
			return BashJobSnapshot{}, fmt.Errorf("Too many background jobs (max %d).", bashJobMaxRetained)
		}
	}
	m.mu.Unlock()

	jobID := "job_" + randomID()[:16]
	cmd := exec.Command("sh", "-c", opts.Command)
	cmd.Dir = opts.Cwd
	cmd.Env = append(append([]string{}, opts.Env...), "CONSOLE_BASH_JOB_ID="+jobID)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return BashJobSnapshot{}, err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return BashJobSnapshot{}, err
	}
	if err := cmd.Start(); err != nil {
		return BashJobSnapshot{}, fmt.Errorf("Failed to start background job: %v", err)
	}

	timeoutMs := opts.TimeoutMs
	if timeoutMs <= 0 {
		timeoutMs = BashJobDefaultTimeoutMs
	}
	ownerSessionID := opts.OwnerSessionID
	if ownerSessionID == "" {
		ownerSessionID = "default"
	}

	rec := &bashJobRecord{
		BashJobSnapshot: BashJobSnapshot{
			JobID: jobID, Command: opts.Command, Cwd: opts.Cwd,
			Status: BashJobRunning, StartedAt: nowISO(),
		},
		cmd:            cmd,
		ownerSessionID: ownerSessionID,
	}
	m.mu.Lock()
	m.jobs[jobID] = rec
	m.mu.Unlock()

	rec.timer = time.AfterFunc(time.Duration(timeoutMs)*time.Millisecond, func() { m.expire(rec) })
	go m.drain(rec, stdout, "stdout")
	go m.drain(rec, stderr, "stderr")
	go m.trackExit(rec)

	return rec.snapshot(), nil
}

func (m *BashJobManager) drain(rec *bashJobRecord, r interface{ Read([]byte) (int, error) }, stream string) {
	buf := make([]byte, 32*1024)
	decoder := newUTF8Decoder()
	for {
		n, err := r.Read(buf)
		if n > 0 {
			m.append(rec, stream, decoder.decode(buf[:n]))
		}
		if err != nil {
			break
		}
	}
	rec.mu.Lock()
	if stream == "stdout" {
		rec.stdoutDone = true
	} else {
		rec.stderrDone = true
	}
	rec.mu.Unlock()
	m.maybeFinish(rec)
}

func (m *BashJobManager) append(rec *bashJobRecord, stream, text string) {
	if text == "" {
		return
	}
	if m.ports != nil {
		m.ports.ObserveOutput(PortOwner{Kind: "job", ID: rec.JobID}, text, "")
	}
	rec.mu.Lock()
	defer rec.mu.Unlock()
	if stream == "stdout" {
		rec.stdout.WriteString(text)
		rec.stdoutTotal += len(text)
		if rec.stdout.Len() > bashJobMaxOutputChars {
			s := rec.stdout.String()
			excess := len(s) - bashJobMaxOutputChars
			rec.stdout.Reset()
			rec.stdout.WriteString(s[excess:])
			rec.stdoutDropped += excess
		}
	} else {
		rec.stderr.WriteString(text)
		rec.stderrTotal += len(text)
		if rec.stderr.Len() > bashJobMaxOutputChars {
			s := rec.stderr.String()
			excess := len(s) - bashJobMaxOutputChars
			rec.stderr.Reset()
			rec.stderr.WriteString(s[excess:])
			rec.stderrDropped += excess
		}
	}
}

func (m *BashJobManager) trackExit(rec *bashJobRecord) {
	err := rec.cmd.Wait()
	code := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			code = exitErr.ExitCode()
		} else {
			code = 1
		}
	}
	rec.mu.Lock()
	rec.rawExit = code
	rec.exited = true
	rec.mu.Unlock()
	m.maybeFinish(rec)
}

func (m *BashJobManager) maybeFinish(rec *bashJobRecord) {
	rec.mu.Lock()
	if rec.Status != BashJobRunning || !rec.exited || !rec.stdoutDone || !rec.stderrDone {
		rec.mu.Unlock()
		return
	}
	if rec.timer != nil {
		rec.timer.Stop()
	}
	code := rec.rawExit
	rec.ExitCode = &code
	rec.FinishedAt = nowISO()
	if code == 0 {
		rec.Status = BashJobExited
	} else {
		rec.Status = BashJobFailed
	}
	rec.mu.Unlock()
	if m.ports != nil {
		m.ports.RemoveOwner(PortOwner{Kind: "job", ID: rec.JobID})
	}
	m.scheduleRetention(rec)
	m.notify(rec)
}

func (m *BashJobManager) expire(rec *bashJobRecord) {
	rec.mu.Lock()
	if rec.Status != BashJobRunning {
		rec.mu.Unlock()
		return
	}
	rec.TimedOut = true
	rec.Status = BashJobExpired
	rec.FinishedAt = nowISO()
	pid := rec.cmd.Process.Pid
	rec.mu.Unlock()
	safeKillGroup(pid)
	if m.ports != nil {
		m.ports.RemoveOwner(PortOwner{Kind: "job", ID: rec.JobID})
	}
	m.scheduleRetention(rec)
	m.notify(rec)
}

// Status returns the current snapshot for jobID, scoped to ownerSessionID.
func (m *BashJobManager) Status(jobID, ownerSessionID string) (BashJobSnapshot, error) {
	rec, err := m.getOwned(jobID, ownerSessionID)
	if err != nil {
		return BashJobSnapshot{}, err
	}
	return rec.snapshot(), nil
}

// Output returns a cursor-paginated slice of a job's buffered stdout, plus
// a bounded tail of stderr.
func (m *BashJobManager) Output(jobID, ownerSessionID string, cursor, limit int) (BashJobOutput, error) {
	rec, err := m.getOwned(jobID, ownerSessionID)
	if err != nil {
		return BashJobOutput{}, err
	}
	if limit <= 0 {
		limit = 20_000
	}
	if limit > 50_000 {
		limit = 50_000
	}
	if cursor < 0 {
		cursor = 0
	}

	rec.mu.Lock()
	defer rec.mu.Unlock()
	stdout := rec.stdout.String()
	truncated := false
	var out string
	var nextCursor int
	if cursor < rec.stdoutDropped {
		truncated = true
		end := limit
		if end > len(stdout) {
			end = len(stdout)
		}
		out = stdout[:end]
		nextCursor = rec.stdoutDropped + end
	} else {
		start := cursor - rec.stdoutDropped
		if start < 0 {
			start = 0
		}
		if start > len(stdout) {
			start = len(stdout)
		}
		end := start + limit
		if end > len(stdout) {
			end = len(stdout)
		}
		out = stdout[start:end]
		nextCursor = cursor + len(out)
		if cursor > rec.stdoutTotal {
			nextCursor = rec.stdoutTotal
		}
	}
	stderr := rec.stderr.String()
	stderrTail := stderr
	if len(stderr) > limit {
		stderrTail = stderr[len(stderr)-limit:]
	}
	if rec.stdoutDropped > 0 || rec.stderrDropped > 0 {
		truncated = true
	}
	return BashJobOutput{
		Snapshot: rec.snapshotLocked(), Stdout: out, Stderr: stderrTail,
		NextCursor: nextCursor, Truncated: truncated,
	}, nil
}

// Wait blocks until jobID settles or waitMs elapses, whichever first.
func (m *BashJobManager) Wait(jobID, ownerSessionID string, waitMs int) (BashJobSnapshot, error) {
	rec, err := m.getOwned(jobID, ownerSessionID)
	if err != nil {
		return BashJobSnapshot{}, err
	}
	rec.mu.Lock()
	if rec.Status != BashJobRunning {
		snap := rec.snapshotLocked()
		rec.mu.Unlock()
		return snap, nil
	}
	ch := make(chan struct{})
	rec.waiters = append(rec.waiters, ch)
	rec.mu.Unlock()

	if waitMs <= 0 {
		waitMs = 10_000
	}
	if waitMs > BashJobMaxWaitMs {
		waitMs = BashJobMaxWaitMs
	}
	select {
	case <-ch:
	case <-time.After(time.Duration(waitMs) * time.Millisecond):
	}
	return rec.snapshot(), nil
}

// Kill terminates jobID's process tree (SIGTERM, escalating to SIGKILL).
func (m *BashJobManager) Kill(jobID, ownerSessionID string) (BashJobSnapshot, error) {
	rec, err := m.getOwned(jobID, ownerSessionID)
	if err != nil {
		return BashJobSnapshot{}, err
	}
	rec.mu.Lock()
	if rec.Status != BashJobRunning {
		snap := rec.snapshotLocked()
		rec.mu.Unlock()
		return snap, nil
	}
	pid := rec.cmd.Process.Pid
	if rec.timer != nil {
		rec.timer.Stop()
	}
	rec.Status = BashJobKilled
	rec.Aborted = true
	rec.FinishedAt = nowISO()
	rec.mu.Unlock()

	safeKillGroup(pid)
	if m.ports != nil {
		m.ports.RemoveOwner(PortOwner{Kind: "job", ID: rec.JobID})
	}
	m.scheduleRetention(rec)
	m.notify(rec)
	return rec.snapshot(), nil
}

// List returns every job owned by ownerSessionID.
func (m *BashJobManager) List(ownerSessionID string) []BashJobSnapshot {
	if ownerSessionID == "" {
		ownerSessionID = "default"
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]BashJobSnapshot, 0, len(m.jobs))
	for _, j := range m.jobs {
		j.mu.Lock()
		if j.ownerSessionID == ownerSessionID {
			out = append(out, j.snapshotLocked())
		}
		j.mu.Unlock()
	}
	return out
}

// KillAll terminates every running job (server shutdown).
func (m *BashJobManager) KillAll() {
	m.mu.Lock()
	recs := make([]*bashJobRecord, 0, len(m.jobs))
	for _, rec := range m.jobs {
		recs = append(recs, rec)
	}
	m.mu.Unlock()
	for _, rec := range recs {
		m.killRecord(rec)
	}
}

// KillOwnedBy terminates every running job owned by ownerSessionID (session
// abort: "stop" means stop everything the session started, including
// detached background jobs).
func (m *BashJobManager) KillOwnedBy(ownerSessionID string) {
	if ownerSessionID == "" {
		ownerSessionID = "default"
	}
	m.mu.Lock()
	recs := make([]*bashJobRecord, 0)
	for _, rec := range m.jobs {
		if rec.ownerSessionID == ownerSessionID {
			recs = append(recs, rec)
		}
	}
	m.mu.Unlock()
	for _, rec := range recs {
		m.killRecord(rec)
	}
}

// killRecord terminates rec's process tree if still running, marking it
// killed/aborted and cleaning up its port ownership. No-op when already
// settled.
func (m *BashJobManager) killRecord(rec *bashJobRecord) {
	rec.mu.Lock()
	if rec.Status != BashJobRunning {
		rec.mu.Unlock()
		return
	}
	if rec.timer != nil {
		rec.timer.Stop()
	}
	pid := 0
	if rec.cmd.Process != nil {
		pid = rec.cmd.Process.Pid
	}
	jobID := rec.JobID
	rec.Status = BashJobKilled
	rec.Aborted = true
	rec.FinishedAt = nowISO()
	rec.mu.Unlock()

	safeKillGroup(pid)
	if m.ports != nil {
		m.ports.RemoveOwner(PortOwner{Kind: "job", ID: jobID})
	}
	m.scheduleRetention(rec)
	m.notify(rec)
}

func (m *BashJobManager) scheduleRetention(rec *bashJobRecord) {
	rec.mu.Lock()
	if rec.retentionTimer != nil {
		rec.retentionTimer.Stop()
	}
	jobID := rec.JobID
	rec.retentionTimer = time.AfterFunc(bashJobRetention, func() { m.delete(jobID) })
	rec.mu.Unlock()
}

func (m *BashJobManager) notify(rec *bashJobRecord) {
	rec.mu.Lock()
	waiters := rec.waiters
	rec.waiters = nil
	rec.mu.Unlock()
	for _, ch := range waiters {
		close(ch)
	}
}

func (m *BashJobManager) getOwned(jobID, ownerSessionID string) (*bashJobRecord, error) {
	if ownerSessionID == "" {
		ownerSessionID = "default"
	}
	m.mu.Lock()
	rec, ok := m.jobs[jobID]
	m.mu.Unlock()
	if !ok || rec.ownerSessionID != ownerSessionID {
		return nil, fmt.Errorf("Background job %q not found or expired.", jobID)
	}
	return rec, nil
}

func (m *BashJobManager) delete(jobID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	rec, ok := m.jobs[jobID]
	if !ok {
		return
	}
	rec.mu.Lock()
	running := rec.Status == BashJobRunning
	rec.mu.Unlock()
	if running {
		return
	}
	delete(m.jobs, jobID)
}

func (r *bashJobRecord) snapshot() BashJobSnapshot {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.snapshotLocked()
}

func (r *bashJobRecord) snapshotLocked() BashJobSnapshot {
	return r.BashJobSnapshot
}

// safeKillGroup signals the whole process group; falls back to the direct
// pid if the group signal fails (e.g. already reaped).
func safeKillGroup(pid int) {
	if pid <= 0 {
		return
	}
	if err := syscall.Kill(-pid, syscall.SIGTERM); err != nil {
		_ = syscall.Kill(pid, syscall.SIGTERM)
	}
	time.AfterFunc(300*time.Millisecond, func() {
		_ = syscall.Kill(-pid, syscall.SIGKILL)
		_ = syscall.Kill(pid, syscall.SIGKILL)
	})
}
