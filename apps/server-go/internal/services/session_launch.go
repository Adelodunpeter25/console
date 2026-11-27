// In-memory registry of in-flight or failed worktree launches
// (POST /api/sessions/with-worktree). The route creates the session row
// immediately and provisions the worktree in the background; this registry
// holds the launch's prompt payload and state so the session header can
// report launchState/launchError and a failed launch can retry.
//
// Known limitation: registry state is process-local. A server restart
// mid-launch leaves a plain, worktree-less session — degraded but
// consistent, and acceptable for v1.
package services

import (
	"sync"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/types"
)

// Launch states surfaced to clients through SessionHeader.launchState.
// Absent (no registry entry) means no launch is pending or failed.
const (
	LaunchProvisioning = "provisioning"
	LaunchFailed       = "failed"
)

// LaunchAttachment mirrors one inline base64 image from the run prompt body.
type LaunchAttachment struct {
	Data     string
	MimeType string
}

// LaunchPrompt is the stored first prompt of a worktree launch, kept as
// plain fields (no run package types: run imports services, so the
// dependency only flows one way).
type LaunchPrompt struct {
	Text         string
	ContextFiles []string
	ModelID      string
	Provider     string
	ApprovalMode string
	Thinking     string
	Attachments  []LaunchAttachment
}

// SessionLaunch is one pending or failed worktree launch.
type SessionLaunch struct {
	SessionID string
	Worktree  types.CreateWorktreeSpec
	Prompt    LaunchPrompt
	State     string
	Error     string
}

// WorktreeLaunchRegistry tracks launches by session id.
type WorktreeLaunchRegistry struct {
	mu       sync.Mutex
	launches map[string]*SessionLaunch
}

// WorktreeLaunches is the process-wide registry consulted by the header
// mapping (sessionHeaderToProto) as well as the launch routes.
var WorktreeLaunches = NewWorktreeLaunchRegistry()

func NewWorktreeLaunchRegistry() *WorktreeLaunchRegistry {
	return &WorktreeLaunchRegistry{launches: make(map[string]*SessionLaunch)}
}

// Start registers a provisioning launch, replacing any prior entry for the
// session (a retry re-registers with the stored payload).
func (r *WorktreeLaunchRegistry) Start(sessionID string, spec types.CreateWorktreeSpec, prompt LaunchPrompt) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.launches[sessionID] = &SessionLaunch{
		SessionID: sessionID, Worktree: spec, Prompt: prompt,
		State: LaunchProvisioning,
	}
}

// Get returns a copy of the launch for the session, or nil when none is
// pending or failed.
func (r *WorktreeLaunchRegistry) Get(sessionID string) *SessionLaunch {
	r.mu.Lock()
	defer r.mu.Unlock()
	launch, ok := r.launches[sessionID]
	if !ok {
		return nil
	}
	copy := *launch
	return &copy
}

// MarkFailed records a launch failure with the message clients display.
func (r *WorktreeLaunchRegistry) MarkFailed(sessionID string, message string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	launch, ok := r.launches[sessionID]
	if !ok {
		return
	}
	launch.State = LaunchFailed
	launch.Error = message
}

// Clear drops the launch entry (provisioning finished cleanly). Absent
// launch_state means ready.
func (r *WorktreeLaunchRegistry) Clear(sessionID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.launches, sessionID)
}

// TakeFailed returns the stored payload for a failed launch and resets it
// to provisioning, so a retry re-uses exactly what the client sent. The
// second return is false when no failed launch exists for the session.
func (r *WorktreeLaunchRegistry) TakeFailed(sessionID string) (*SessionLaunch, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	launch, ok := r.launches[sessionID]
	if !ok || launch.State != LaunchFailed {
		return nil, false
	}
	copy := *launch
	launch.State = LaunchProvisioning
	launch.Error = ""
	return &copy, true
}
