// PTY manager for the terminal WebSocket. Port of the spawn/kill subset of
// apps/server/api/src/terminal/pty.manager.ts (JSON + binary protocol).
package services

import (
	"fmt"
	"os"
	"os/exec"
	"sync"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/types"
	"github.com/creack/pty"
)

const maxSpawnsPerMinute = 30

type PtySession struct {
	ID     string
	cmd    *exec.Cmd
	ptmx   *os.File
	OnData func([]byte)
	OnExit func(int)
	mu     sync.Mutex
	killed bool
}

func (s *PtySession) Write(data []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.killed {
		return fmt.Errorf("session killed")
	}
	_, err := s.ptmx.Write(data)
	return err
}

// Resize updates the PTY window size.
func (s *PtySession) Resize(cols, rows int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.killed {
		return fmt.Errorf("session killed")
	}
	return pty.Setsize(s.ptmx, &pty.Winsize{Cols: uint16(cols), Rows: uint16(rows)})
}

func (s *PtySession) Kill() {
	s.mu.Lock()
	if s.killed {
		s.mu.Unlock()
		return
	}
	s.killed = true
	ptmx := s.ptmx
	cmd := s.cmd
	s.mu.Unlock()

	if cmd.Process != nil {
		_ = cmd.Process.Kill()
	}
	_ = ptmx.Close()
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
	m.spawnTimes = append(m.spawnTimes, now)
	m.mu.Unlock()

	shell := params.Shell
	if shell == "" {
		shell = os.Getenv("SHELL")
	}
	if shell == "" {
		shell = "/bin/bash"
	}
	cmd := exec.Command(shell)
	cmd.Dir = params.Cwd
	cmd.Env = append(os.Environ(), "TERM=xterm-256color")

	ptmx, err := pty.StartWithSize(cmd, &pty.Winsize{Cols: uint16(params.Cols), Rows: uint16(params.Rows)})
	if err != nil {
		return nil, err
	}
	session := &PtySession{ID: randomID(), cmd: cmd, ptmx: ptmx}

	// Pump PTY output to the socket callback.
	go func() {
		buf := make([]byte, 32*1024)
		for {
			n, err := ptmx.Read(buf)
			if n > 0 && session.OnData != nil {
				session.OnData(buf[:n])
			}
			if err != nil {
				break
			}
		}
		code := 0
		if werr := cmd.Wait(); werr != nil {
			if exitErr, ok := werr.(*exec.ExitError); ok && exitErr.ExitCode() > 0 {
				code = exitErr.ExitCode()
			}
		}
		session.Kill()
		if session.OnExit != nil {
			session.OnExit(code)
		}
	}()

	m.mu.Lock()
	m.sessions[session.ID] = session
	m.mu.Unlock()
	return session, nil
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
