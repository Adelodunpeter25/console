// PTY manager for the terminal WebSocket. Port of
// apps/server/api/src/terminal/pty.manager.ts (JSON + binary protocol).
//
// Performance hardening mirrors the Bun version:
//   - output coalescing (8ms window / 4KB early-flush / 64KB frame cap)
//   - bounded queues with oldest-drop (8MB cap) so flooding programs can't OOM
//   - pause/resume for send-buffer backpressure
//   - cached base env, shell allowlist, concurrency cap, input size cap
package services

import (
	"fmt"
	"os"
	"os/exec"
	"path"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/types"
	"github.com/creack/pty"
)

const maxSpawnsPerMinute = 30

// Coalescing window: PTY data inside one window ships as fewer frames.
const (
	outputFlushMs    = 8 * time.Millisecond
	outputFlushBytes = 4 * 1024
	outputFrameBytes = 64 * 1024
)

// Cap for buffered output so a flooding program can't balloon memory.
const pausedBufferLimitBytes = 8 * 1024 * 1024

// Max input bytes per Write (mirrors Bun's 256KB guard).
const maxTerminalInputBytes = 256 * 1024

const maxConcurrentTerminals = 20

// Shell allowlist mirroring Bun's ALLOWED_SHELLS.
var allowedShells = map[string]struct{}{
	"/bin/bash":          {},
	"/bin/zsh":           {},
	"/bin/sh":            {},
	"/usr/bin/bash":      {},
	"/usr/bin/zsh":       {},
	"/usr/local/bin/bash": {},
	"/usr/local/bin/zsh": {},
	"/bin/fish":          {},
	"/usr/bin/fish":      {},
	"powershell.exe":     {},
	"pwsh.exe":           {},
	"cmd.exe":            {},
}

func isAllowedShell(shell string) bool {
	if _, ok := allowedShells[shell]; ok {
		return true
	}
	if hostShell := os.Getenv("SHELL"); hostShell != "" && shell == hostShell {
		return true
	}
	base := path.Base(shell)
	for allowed := range allowedShells {
		if path.Base(allowed) == base {
			return true
		}
	}
	return false
}

// Cached base env: spawns are frequent enough that re-copying os.Environ per
// spawn is pure GC pressure — the server env is static after boot.
var (
	cachedBaseEnvOnce sync.Once
	cachedBaseEnv     []string
)

func baseEnv() []string {
	cachedBaseEnvOnce.Do(func() {
		cachedBaseEnv = os.Environ()
	})
	return cachedBaseEnv
}

type PtySession struct {
	ID  string
	cmd *exec.Cmd
	ptmx *os.File

	manager *PtyManager

	// Callbacks registered by the WS route. Guarded by outputMu.
	onData func([]byte)
	onExit func(int)

	// killed is set once; checked atomically so Write/Resize never hold a
	// mutex across blocking ptmx syscalls.
	killed atomic.Bool
	// writeMu serializes ptmx Write vs Setsize.
	writeMu sync.Mutex

	// Output coalescing + backpressure state, all guarded by outputMu.
	outputMu      sync.Mutex
	outputQueue   [][]byte
	outputBytes   int
	flushTimer    *time.Timer
	paused        bool
	pausedBuffer  [][]byte
	pausedBytes   int
	pending       [][]byte
	pendingCount  int
}

// SetCallbacks attaches the WS pump and flushes early output that arrived
// between spawn and attach (e.g. the shell prompt).
func (s *PtySession) SetCallbacks(onData func([]byte), onExit func(int)) {
	var toFlush [][]byte
	s.outputMu.Lock()
	s.onData = onData
	s.onExit = onExit
	if len(s.pending) > 0 {
		toFlush = s.pending
		s.pending = nil
		s.pendingCount = 0
	}
	s.outputMu.Unlock()

	for _, chunk := range toFlush {
		onData(chunk)
	}
}

func (s *PtySession) Write(data []byte) error {
	if s.killed.Load() {
		return fmt.Errorf("session killed")
	}
	if len(data) > maxTerminalInputBytes {
		return fmt.Errorf("input too large")
	}
	// No mutex held across the blocking write; killed is atomic so Kill
	// never blocks behind a slow ptmx.
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	if s.killed.Load() {
		return fmt.Errorf("session killed")
	}
	_, err := s.ptmx.Write(data)
	return err
}

// Pid returns the spawned shell's process id (0 when unknown). The desktop
// requires pid in the `spawned` frame to flip a terminal to Running.
// cmd is assigned once during Spawn before the session is shared.
func (s *PtySession) Pid() int {
	if s.cmd == nil || s.cmd.Process == nil {
		return 0
	}
	return s.cmd.Process.Pid
}

// Resize updates the PTY window size.
func (s *PtySession) Resize(cols, rows int) error {
	if cols < 1 || cols > 500 || rows < 1 || rows > 200 {
		return fmt.Errorf("invalid size")
	}
	if s.killed.Load() {
		return fmt.Errorf("session killed")
	}
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	if s.killed.Load() {
		return fmt.Errorf("session killed")
	}
	return pty.Setsize(s.ptmx, &pty.Winsize{Cols: uint16(cols), Rows: uint16(rows)})
}

func (s *PtySession) Kill() {
	if s.killed.Swap(true) {
		return
	}
	s.outputMu.Lock()
	if s.flushTimer != nil {
		s.flushTimer.Stop()
		s.flushTimer = nil
	}
	// Drop buffered output so a killed session releases memory promptly.
	s.outputQueue = nil
	s.outputBytes = 0
	s.pausedBuffer = nil
	s.pausedBytes = 0
	s.pending = nil
	s.outputMu.Unlock()

	if s.cmd.Process != nil {
		_ = s.cmd.Process.Kill()
	}
	_ = s.ptmx.Close()
}

// Pause holds output in a bounded buffer until Resume.
func (s *PtySession) Pause() {
	s.outputMu.Lock()
	defer s.outputMu.Unlock()
	if !s.killed.Load() {
		s.paused = true
	}
}

// Resume drains the held buffer back through the coalescing queue.
func (s *PtySession) Resume() {
	s.outputMu.Lock()
	if !s.paused || s.killed.Load() {
		s.outputMu.Unlock()
		return
	}
	s.paused = false
	held := s.pausedBuffer
	s.pausedBuffer = nil
	s.pausedBytes = 0
	s.outputMu.Unlock()

	for _, chunk := range held {
		s.enqueueOutput(chunk)
	}
}

// enqueueOutput routes one PTY read into the coalescing queue or the paused
// buffer. The caller must pass an owned copy (the read buffer is reused).
func (s *PtySession) enqueueOutput(chunk []byte) {
	s.outputMu.Lock()
	if s.killed.Load() {
		s.outputMu.Unlock()
		return
	}
	// No callbacks yet (spawn → attach gap): hold early output, capped.
	if s.onData == nil && !s.paused {
		cp := make([]byte, len(chunk))
		copy(cp, chunk)
		s.pending = append(s.pending, cp)
		s.pendingCount++
		if len(s.pending) > 100 {
			dropped := s.pending[0]
			s.pending = s.pending[1:]
			_ = dropped
			s.pendingCount--
		}
		s.outputMu.Unlock()
		return
	}
	if s.paused {
		cp := make([]byte, len(chunk))
		copy(cp, chunk)
		s.pausedBuffer = append(s.pausedBuffer, cp)
		s.pausedBytes += len(cp)
		for s.pausedBytes > pausedBufferLimitBytes && len(s.pausedBuffer) > 0 {
			dropped := s.pausedBuffer[0]
			s.pausedBuffer = s.pausedBuffer[1:]
			s.pausedBytes -= len(dropped)
		}
		s.outputMu.Unlock()
		return
	}
	cp := make([]byte, len(chunk))
	copy(cp, chunk)
	s.outputQueue = append(s.outputQueue, cp)
	s.outputBytes += len(cp)
	for s.outputBytes > pausedBufferLimitBytes && len(s.outputQueue) > 0 {
		dropped := s.outputQueue[0]
		s.outputQueue = s.outputQueue[1:]
		s.outputBytes -= len(dropped)
	}
	shouldFlush := s.outputBytes >= outputFlushBytes
	if shouldFlush {
		if s.flushTimer != nil {
			s.flushTimer.Stop()
			s.flushTimer = nil
		}
	} else if s.flushTimer == nil {
		s.flushTimer = time.AfterFunc(outputFlushMs, func() { s.flushOutput() })
	}
	s.outputMu.Unlock()

	if shouldFlush {
		s.flushOutput()
	}
}

// flushOutput ships the coalesced window as 64KB-capped frames.
func (s *PtySession) flushOutput() {
	s.outputMu.Lock()
	if s.flushTimer != nil {
		s.flushTimer.Stop()
		s.flushTimer = nil
	}
	if len(s.outputQueue) == 0 || s.killed.Load() {
		s.outputMu.Unlock()
		return
	}
	if s.paused || s.onData == nil {
		// Paused (or detached) mid-window: hold for resume/attach.
		for _, chunk := range s.outputQueue {
			s.pausedBuffer = append(s.pausedBuffer, chunk)
			s.pausedBytes += len(chunk)
		}
		for s.pausedBytes > pausedBufferLimitBytes && len(s.pausedBuffer) > 0 {
			dropped := s.pausedBuffer[0]
			s.pausedBuffer = s.pausedBuffer[1:]
			s.pausedBytes -= len(dropped)
		}
		s.outputQueue = nil
		s.outputBytes = 0
		s.outputMu.Unlock()
		return
	}
	queued := s.outputQueue
	s.outputQueue = nil
	s.outputBytes = 0
	onData := s.onData
	s.outputMu.Unlock()

	var joined []byte
	if len(queued) == 1 {
		joined = queued[0]
	} else {
		total := 0
		for _, c := range queued {
			total += len(c)
		}
		joined = make([]byte, 0, total)
		for _, c := range queued {
			joined = append(joined, c...)
		}
	}
	for i := 0; i < len(joined); i += outputFrameBytes {
		end := i + outputFrameBytes
		if end > len(joined) {
			end = len(joined)
		}
		onData(joined[i:end])
	}
}

type PtyManager struct {
	mu         sync.Mutex
	sessions   map[string]*PtySession
	spawnTimes []int64
}

func NewPtyManager() *PtyManager {
	return &PtyManager{sessions: make(map[string]*PtySession)}
}

// Spawn validates params and starts the shell under a PTY.
func (m *PtyManager) Spawn(params types.TerminalSpawnParams) (*PtySession, error) {
	if strings.Contains(params.Cwd, "\x00") {
		return nil, fmt.Errorf("Invalid cwd")
	}
	if info, err := os.Stat(params.Cwd); err != nil {
		return nil, fmt.Errorf("Cannot spawn terminal: working directory does not exist: %s", params.Cwd)
	} else if !info.IsDir() {
		return nil, fmt.Errorf("Cannot spawn terminal: working directory is not a directory: %s", params.Cwd)
	}

	m.mu.Lock()
	now := nowMillis()
	recent := m.spawnTimes[:0]
	for _, t := range m.spawnTimes {
		if now-t < 60_000 {
			recent = append(recent, t)
		}
	}
	m.spawnTimes = recent
	if len(m.spawnTimes) >= maxSpawnsPerMinute {
		m.mu.Unlock()
		return nil, fmt.Errorf("Too many terminal spawns — rate limited. Try again later.")
	}
	if len(m.sessions) >= maxConcurrentTerminals {
		m.mu.Unlock()
		return nil, fmt.Errorf("Too many concurrent terminals.")
	}
	m.spawnTimes = append(m.spawnTimes, now)
	m.mu.Unlock()

	shell := params.Shell
	if shell == "" {
		shell = os.Getenv("SHELL")
	}
	if shell == "" {
		shell = "/bin/bash"
	}
	if strings.Contains(shell, "\x00") {
		return nil, fmt.Errorf("Invalid shell")
	}
	if !isAllowedShell(shell) && !isAllowedShell(path.Base(shell)) {
		return nil, fmt.Errorf("Shell not allowed: %s", shell)
	}

	cols := params.Cols
	if cols <= 0 {
		cols = 80
	}
	rows := params.Rows
	if rows <= 0 {
		rows = 24
	}

	id := randomID()
	cmd := exec.Command(shell)
	cmd.Dir = params.Cwd
	base := baseEnv()
	env := make([]string, 0, len(base)+3)
	env = append(env, base...)
	env = append(env, "TERM=xterm-256color", "CONSOLE_TERMINAL=true", "CONSOLE_TERMINAL_ID="+id)
	cmd.Env = env

	ptmx, err := pty.StartWithSize(cmd, &pty.Winsize{Cols: uint16(cols), Rows: uint16(rows)})
	if err != nil {
		return nil, err
	}
	session := &PtySession{ID: id, cmd: cmd, ptmx: ptmx, manager: m}

	// Pump PTY output through the coalescing queue.
	go func() {
		buf := make([]byte, 32*1024)
		for {
			n, err := ptmx.Read(buf)
			if n > 0 {
				session.enqueueOutput(buf[:n])
			}
			if err != nil {
				break
			}
		}
		// Flush any coalesced output before reporting exit.
		session.flushOutput()
		code := 0
		if werr := cmd.Wait(); werr != nil {
			if exitErr, ok := werr.(*exec.ExitError); ok && exitErr.ExitCode() > 0 {
				code = exitErr.ExitCode()
			}
		}
		// Natural exit: remove from the registry so entries don't leak
		// under churn (explicit Kill also deletes; this covers the
		// process-exits-first path).
		if session.manager != nil {
			session.manager.remove(session.ID)
		}
		session.Kill()
		session.outputMu.Lock()
		onExit := session.onExit
		session.outputMu.Unlock()
		if onExit != nil {
			onExit(code)
		}
	}()

	m.mu.Lock()
	m.sessions[session.ID] = session
	m.mu.Unlock()
	return session, nil
}

func (m *PtyManager) remove(id string) {
	m.mu.Lock()
	delete(m.sessions, id)
	m.mu.Unlock()
}

func (m *PtyManager) Get(id string) *PtySession {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.sessions[id]
}

func (m *PtyManager) Kill(id string) {
	m.mu.Lock()
	session := m.sessions[id]
	delete(m.sessions, id)
	m.mu.Unlock()
	if session != nil {
		session.Kill()
	}
}

func (m *PtyManager) Pause(id string) {
	m.mu.Lock()
	session := m.sessions[id]
	m.mu.Unlock()
	if session != nil {
		session.Pause()
	}
}

func (m *PtyManager) Resume(id string) {
	m.mu.Lock()
	session := m.sessions[id]
	m.mu.Unlock()
	if session != nil {
		session.Resume()
	}
}

func (m *PtyManager) Write(id string, data []byte) bool {
	m.mu.Lock()
	session := m.sessions[id]
	m.mu.Unlock()
	if session == nil {
		return false
	}
	return session.Write(data) == nil
}

func (m *PtyManager) KillAll() {
	m.mu.Lock()
	sessions := make([]*PtySession, 0, len(m.sessions))
	for _, s := range m.sessions {
		sessions = append(sessions, s)
	}
	m.sessions = make(map[string]*PtySession)
	m.mu.Unlock()
	for _, s := range sessions {
		s.Kill()
	}
}
