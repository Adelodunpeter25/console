// Managed script runs. Port of
// apps/server/api/src/services/project-scripts/service.ts: one child shell
// per run in the project's own process group, output ring-buffered to
// 256KB, subscriber fan-out, SIGTERM-to-group stop.
package services

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/types"
)

const scriptMaxOutput = 256 * 1024

type managedRun struct {
	types.ScriptRun
	process     *exec.Cmd
	mu          sync.Mutex
	subscribers map[chan types.ScriptRunEvent]bool
}

func (r *managedRun) publish(event types.ScriptRunEvent) {
	r.mu.Lock()
	subs := make([]chan types.ScriptRunEvent, 0, len(r.subscribers))
	for ch := range r.subscribers {
		subs = append(subs, ch)
	}
	r.mu.Unlock()
	for _, ch := range subs {
		if event.Type == "exit" {
			// The exit event is the only signal that flips a script tab out
			// of "Running" client-side, and subscribers are wiped right
			// after this call — a dropped exit leaves the client watching a
			// dead channel until it manually refreshes. Chatty output can
			// fill the 256-slot buffer before the drop, so give this one a
			// bounded blocking send instead of the fire-and-forget default.
			select {
			case ch <- event:
			case <-time.After(2 * time.Second):
			}
			continue
		}
		select {
		case ch <- event:
		default:
		}
	}
}

func (r *managedRun) appendOutput(stream, text string) {
	r.mu.Lock()
	if stream == "stdout" {
		r.Stdout = capOutput(r.Stdout + text)
	} else {
		r.Stderr = capOutput(r.Stderr + text)
	}
	r.mu.Unlock()
}

func (r *managedRun) snapshot() types.ScriptRun {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := r.ScriptRun
	out.Stdout = r.Stdout
	out.Stderr = r.Stderr
	return out
}

func capOutput(s string) string {
	if len(s) > scriptMaxOutput {
		return s[len(s)-scriptMaxOutput:]
	}
	return s
}

func nowISO() string {
	return time.Now().UTC().Format("2006-01-02T15:04:05.000Z07:00")
}

// BuildScriptEnv strips daemon-owned env vars so project dev servers that
// honor $PORT don't inherit the console daemon's port. Port of
// buildScriptEnv in the TS service.
func BuildScriptEnv() []string {
	out := make([]string, 0)
	for _, kv := range os.Environ() {
		key := kv
		if idx := strings.IndexByte(kv, '='); idx >= 0 {
			key = kv[:idx]
		}
		if key == "PORT" || key == "HOST" || key == "CONSOLE_DAEMON" {
			continue
		}
		out = append(out, kv)
	}
	return out
}

type ProjectScriptsService struct {
	projects *ProjectService
	ports    *PortRegistry

	mu   sync.Mutex
	runs map[string]*managedRun

	configMu    sync.Mutex
	configCache map[string]struct {
		expires time.Time
		data    types.ProjectScriptsResult
	}
}

func NewProjectScriptsService(projects *ProjectService, ports *PortRegistry) *ProjectScriptsService {
	return &ProjectScriptsService{
		projects: projects,
		ports:    ports,
		runs:     make(map[string]*managedRun),
		configCache: make(map[string]struct {
			expires time.Time
			data    types.ProjectScriptsResult
		}),
	}
}

// List parses [scripts.scripts] from console.toml (2s cache).
func (s *ProjectScriptsService) List(projectID string) (types.ProjectScriptsResult, error) {
	project, err := s.projects.Get(projectID)
	if err != nil {
		return types.ProjectScriptsResult{}, fmt.Errorf("Project '%s' not found.", projectID)
	}
	return s.loadProjectScripts(projectID, project.Path)
}

func (s *ProjectScriptsService) loadProjectScripts(projectID, projectRoot string) (types.ProjectScriptsResult, error) {
	s.configMu.Lock()
	if cached, ok := s.configCache[projectRoot]; ok && cached.expires.After(time.Now()) {
		s.configMu.Unlock()
		return cached.data, nil
	}
	s.configMu.Unlock()

	data, err := os.ReadFile(projectRoot + "/console.toml")
	result := types.ProjectScriptsResult{ProjectID: projectID, Scripts: []types.ProjectScript{}, Source: "missing"}
	if err == nil {
		scripts, perr := parseProjectScripts(string(data))
		if perr != nil {
			return types.ProjectScriptsResult{}, perr
		}
		result = types.ProjectScriptsResult{ProjectID: projectID, Scripts: scripts, Source: "console.toml"}
	} else if !os.IsNotExist(err) {
		return types.ProjectScriptsResult{}, err
	}

	s.configMu.Lock()
	s.configCache[projectRoot] = struct {
		expires time.Time
		data    types.ProjectScriptsResult
	}{time.Now().Add(scriptCacheTTL), result}
	s.configMu.Unlock()
	return result, nil
}

// Run starts a script as a detached process group in the project dir.
func (s *ProjectScriptsService) Run(projectID, scriptID string) (types.ScriptRun, error) {
	project, err := s.projects.Get(projectID)
	if err != nil {
		return types.ScriptRun{}, fmt.Errorf("Project '%s' not found.", projectID)
	}
	config, err := s.loadProjectScripts(projectID, project.Path)
	if err != nil {
		return types.ScriptRun{}, err
	}
	var script *types.ProjectScript
	for i := range config.Scripts {
		if config.Scripts[i].ID == scriptID {
			script = &config.Scripts[i]
			break
		}
	}
	if script == nil {
		return types.ScriptRun{}, fmt.Errorf("Script '%s' not found.", scriptID)
	}

	s.mu.Lock()
	candidates := make([]*managedRun, 0, len(s.runs))
	for _, run := range s.runs {
		if run.ProjectID == projectID && run.ScriptID == scriptID {
			candidates = append(candidates, run)
		}
	}
	s.mu.Unlock()
	for _, run := range candidates {
		run.mu.Lock()
		running := run.Status == "running"
		run.mu.Unlock()
		if running {
			return types.ScriptRun{}, fmt.Errorf("Script '%s' is already running.", scriptID)
		}
	}

	cmd := exec.Command("sh", "-c", script.Command)
	cmd.Dir = project.Path
	cmd.Env = BuildScriptEnv()
	cmd.Stdin = nil
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true} // own process group
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return types.ScriptRun{}, err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return types.ScriptRun{}, err
	}
	if err := cmd.Start(); err != nil {
		return types.ScriptRun{}, err
	}

	run := &managedRun{
		ScriptRun: types.ScriptRun{
			RunID: randomID(), ProjectID: projectID, ScriptID: scriptID,
			Label: script.Label, Persistent: script.Persistent,
			Status: "running", StartedAt: nowISO(),
		},
		process:     cmd,
		subscribers: make(map[chan types.ScriptRunEvent]bool),
	}
	s.mu.Lock()
	s.runs[run.RunID] = run
	s.mu.Unlock()
	run.publish(types.ScriptRunEvent{Type: "status", Status: "running"})

	// Script-run dev servers expose their ports through the registry like
	// any other job owner; on exit the owner is removed so the dropdown
	// updates immediately instead of waiting for the liveness reaper.
	var observe func(text string)
	var clearPorts func()
	if s.ports != nil {
		owner := PortOwner{Kind: "job", ID: run.RunID}
		projectID := run.ProjectID
		observe = func(text string) {
			s.ports.ObserveOutput(owner, text, projectID)
		}
		clearPorts = func() { s.ports.RemoveOwner(owner) }
	}

	go func() {
		// cmd.Wait must not run until both StdoutPipe/StderrPipe readers
		// have drained (see os/exec docs); otherwise the run can flip to a
		// terminal status before its output is fully captured.
		var pumps sync.WaitGroup
		pumps.Add(2)
		go func() { defer pumps.Done(); pumpOutput(run, stdout, "stdout", observe) }()
		go func() { defer pumps.Done(); pumpOutput(run, stderr, "stderr", observe) }()
		pumps.Wait()
		err := cmd.Wait()
		run.mu.Lock()
		if run.Status != "stopped" {
			code := 0
			if exitErr, ok := err.(*exec.ExitError); ok {
				code = exitErr.ExitCode()
			}
			run.ExitCode = &code
			ended := nowISO()
			run.EndedAt = &ended
			if code == 0 {
				run.Status = "succeeded"
			} else {
				run.Status = "failed"
			}
			finalStatus := run.Status
			run.mu.Unlock()
			run.publish(types.ScriptRunEvent{Type: "exit", Status: finalStatus, ExitCode: &code})
			run.mu.Lock()
			run.subscribers = make(map[chan types.ScriptRunEvent]bool)
		}
		run.mu.Unlock()
		if clearPorts != nil {
			clearPorts()
		}
	}()
	return run.snapshot(), nil
}

func pumpOutput(run *managedRun, file interface{ Read([]byte) (int, error) }, stream string, observe func(string)) {
	buf := make([]byte, 32*1024)
	decoder := newUTF8Decoder()
	for {
		n, err := file.Read(buf)
		if n > 0 {
			text := decoder.decode(buf[:n])
			run.appendOutput(stream, text)
			run.publish(types.ScriptRunEvent{Type: "output", Stream: stream, Text: text})
			if observe != nil {
				observe(text)
			}
		}
		if err != nil {
			return
		}
	}
}

func (s *ProjectScriptsService) ListRuns(projectID string) []types.ScriptRun {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]types.ScriptRun, 0)
	for _, run := range s.runs {
		if run.ProjectID == projectID {
			out = append(out, run.snapshot())
		}
	}
	return out
}

func (s *ProjectScriptsService) GetRun(projectID, runID string) *types.ScriptRun {
	s.mu.Lock()
	run := s.runs[runID]
	s.mu.Unlock()
	if run == nil || run.ProjectID != projectID {
		return nil
	}
	snap := run.snapshot()
	return &snap
}

func (s *ProjectScriptsService) Stop(projectID, runID string) bool {
	s.mu.Lock()
	run := s.runs[runID]
	s.mu.Unlock()
	if run == nil || run.ProjectID != projectID {
		return false
	}
	run.mu.Lock()
	if run.Status != "running" {
		run.mu.Unlock()
		return false
	}
	// Signal the whole process group; fall back to the direct child.
	if run.process.Process != nil {
		if err := syscall.Kill(-run.process.Process.Pid, syscall.SIGTERM); err != nil {
			_ = run.process.Process.Kill()
		}
	}
	run.Status = "stopped"
	ended := nowISO()
	run.EndedAt = &ended
	run.mu.Unlock()

	run.publish(types.ScriptRunEvent{Type: "exit", Status: "stopped"})
	run.mu.Lock()
	run.subscribers = make(map[chan types.ScriptRunEvent]bool)
	run.mu.Unlock()
	if s.ports != nil {
		s.ports.RemoveOwner(PortOwner{Kind: "job", ID: runID})
	}
	return true
}

// Subscribe replays current status + buffered output, then streams live
// events until the run finishes. Returns the event channel or nil.
func (s *ProjectScriptsService) Subscribe(projectID, runID string) (chan types.ScriptRunEvent, bool) {
	s.mu.Lock()
	run := s.runs[runID]
	s.mu.Unlock()
	if run == nil || run.ProjectID != projectID {
		return nil, false
	}
	snap := run.snapshot()
	ch := make(chan types.ScriptRunEvent, 256)
	ch <- types.ScriptRunEvent{Type: "status", Status: snap.Status}
	if snap.Stdout != "" {
		ch <- types.ScriptRunEvent{Type: "output", Stream: "stdout", Text: snap.Stdout}
	}
	if snap.Stderr != "" {
		ch <- types.ScriptRunEvent{Type: "output", Stream: "stderr", Text: snap.Stderr}
	}
	if snap.Status != "running" {
		ch <- types.ScriptRunEvent{Type: "exit", Status: snap.Status, ExitCode: snap.ExitCode}
	} else {
		run.mu.Lock()
		run.subscribers[ch] = true
		run.mu.Unlock()
	}
	return ch, true
}

func (s *ProjectScriptsService) Unsubscribe(projectID, runID string, ch chan types.ScriptRunEvent) {
	s.mu.Lock()
	run := s.runs[runID]
	s.mu.Unlock()
	if run == nil {
		return
	}
	run.mu.Lock()
	delete(run.subscribers, ch)
	run.mu.Unlock()
}

// IsRunning reports whether the run is still live (stream termination).
func (s *ProjectScriptsService) IsRunning(projectID, runID string) bool {
	snap := s.GetRun(projectID, runID)
	return snap != nil && snap.Status == "running"
}
