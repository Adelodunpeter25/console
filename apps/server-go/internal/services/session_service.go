// Session service: facade for session operations split into sub-files.
// This maintains backward compatibility while organizing code by functionality.
package services

import (
	"errors"
	"os"
	"path/filepath"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/db"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/services/session"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/types"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/utils"
)

type SessionService struct {
	inner *session.Service
}

func NewSessionService(manager *db.DB) *SessionService {
	return &SessionService{inner: session.New(manager)}
}

// ErrWorktreeScratchpad rejects a worktree spec on a scratchpad session:
// scratchpads own a sandboxed dir, not a repo checkout.
var ErrWorktreeScratchpad = errors.New("worktree requires a project directory, not a scratchpad session")

// ErrWorktreeNeedsCwd rejects a worktree spec without an explicit repo dir.
var ErrWorktreeNeedsCwd = errors.New("worktree requires an explicit cwd pointing at a git repository")

// Core operations
func (s *SessionService) Create(opts types.CreateSessionOptions) (types.SessionHeader, error) {
	provisioned := false
	var wtRepo, wtPath string
	if opts.Worktree != nil {
		if opts.ProjectNull {
			return types.SessionHeader{}, ErrWorktreeScratchpad
		}
		if opts.Cwd == "" {
			return types.SessionHeader{}, ErrWorktreeNeedsCwd
		}
		wtRepo = opts.Cwd
		if opts.ID == "" {
			opts.ID = utils.RandomID()
		}
		branch := opts.Worktree.Branch
		if branch == "" {
			branch = SlugBranch(opts.Title, opts.ID)
		}
		root, err := DefaultRoot()
		if err != nil {
			return types.SessionHeader{}, err
		}
		wtPath = filepath.Join(root, opts.ID)
		if err := os.MkdirAll(root, 0o755); err != nil {
			return types.SessionHeader{}, err
		}
		if err := NewWorktreeService().WorktreeAdd(wtRepo, wtPath, branch); err != nil {
			return types.SessionHeader{}, err
		}
		provisioned = true
		opts.Cwd = wtPath
		opts.ResolvedWorktree = &types.SessionWorktree{Path: wtPath, Branch: branch, Repo: wtRepo}
	}
	header, err := s.inner.Create(opts)
	if err != nil && provisioned {
		// Roll back the provisioned worktree so a half-created session
		// never lingers.
		_ = NewWorktreeService().WorktreeRemove(wtRepo, wtPath, true)
		return types.SessionHeader{}, err
	}
	if err == nil && header.Cwd != "" && manager != nil {
		// A new session means its project was just opened: start the
		// file-search index scan now so the first @-mention/grep is warm.
		manager.Prewarm(header.Cwd)
	}
	return header, err
}

func (s *SessionService) ListFiltered(f session.ListFilter) ([]types.SessionHeader, error) {
	return s.inner.ListFiltered(f)
}

func (s *SessionService) Load(sessionID string, limit int64, before int64) (*types.LoadedSession, error) {
	loaded, err := s.inner.Load(sessionID, limit, before)
	if err == nil && loaded != nil && loaded.Header.Cwd != "" && manager != nil {
		// Opening an existing session re-opens its project: same warm-up.
		manager.Prewarm(loaded.Header.Cwd)
	}
	return loaded, err
}

func (s *SessionService) Header(sessionID string) (*types.SessionHeader, error) {
	return s.inner.Header(sessionID)
}

func (s *SessionService) SoftDelete(sessionID string) (bool, error) {
	return s.inner.SoftDelete(sessionID)
}

func (s *SessionService) Restore(sessionID string) (bool, error) {
	return s.inner.Restore(sessionID)
}

func (s *SessionService) ExpiredDeletedSessions(cutoffMillis int64) ([]string, error) {
	return s.inner.ExpiredDeletedSessions(cutoffMillis)
}

func (s *SessionService) PermanentDelete(sessionID string) (bool, error) {
	wt, err := s.inner.WorktreeOf(sessionID)
	if err != nil {
		return false, err
	}
	if wt != nil {
		// Owned worktree goes first: dirty blocks the whole delete.
		if err := NewWorktreeService().WorktreeRemove(wt.Repo, wt.Path, false); err != nil {
			return false, err
		}
	}
	return s.inner.PermanentDelete(sessionID)
}

// OwnedWorktreePaths lists worktree dirs claimed by session rows (trash
// included) for orphan detection.
func (s *SessionService) OwnedWorktreePaths() ([]string, error) {
	return s.inner.OwnedWorktreePaths()
}

func (s *SessionService) UpdateTitle(sessionID, title string) error {
	return s.inner.UpdateTitle(sessionID, title)
}

func (s *SessionService) UpdateModel(sessionID, modelID, provider string) error {
	return s.inner.UpdateModel(sessionID, modelID, provider)
}

func (s *SessionService) UpdateCwd(sessionID, cwd string, projectID *string) error {
	err := s.inner.UpdateCwd(sessionID, cwd, projectID)
	if err == nil && cwd != "" && manager != nil {
		// Session moved to another project root: warm that index too.
		manager.Prewarm(cwd)
	}
	return err
}

func (s *SessionService) UpdateApprovalMode(sessionID, approvalMode string) error {
	return s.inner.UpdateApprovalMode(sessionID, approvalMode)
}

func (s *SessionService) UpdateStatus(sessionID, status string) error {
	return s.inner.UpdateStatus(sessionID, status)
}

func (s *SessionService) GetSubagents(sessionID string) ([]types.SubagentInfo, error) {
	return s.inner.GetSubagents(sessionID)
}

func (s *SessionService) ProjectByDir(dir string) (string, error) {
	return s.inner.ProjectByDir(dir)
}

// Message operations
func (s *SessionService) AppendMessage(sessionID string, msg types.AgentMessage) error {
	return s.inner.AppendMessage(sessionID, msg)
}

func (s *SessionService) AppendMessages(sessionID string, messages []types.AgentMessage) error {
	return s.inner.AppendMessages(sessionID, messages)
}

func (s *SessionService) ReplaceMessages(sessionID string, messages []types.AgentMessage) error {
	return s.inner.ReplaceMessages(sessionID, messages)
}

// Todo operations
func (s *SessionService) SaveSessionTodos(sessionID string, items []types.TodoItem) error {
	return s.inner.SaveSessionTodos(sessionID, items)
}

func (s *SessionService) GetSessionTodos(sessionID string) ([]types.TodoItem, error) {
	return s.inner.GetSessionTodos(sessionID)
}

func (s *SessionService) ClearSessionTodos(sessionID string) error {
	return s.inner.ClearSessionTodos(sessionID)
}

func (s *SessionService) ClearCompletedTodos(sessionID string) error {
	return s.inner.ClearCompletedTodos(sessionID)
}

// File change operations
func (s *SessionService) RecordFileChange(sessionID string, change types.SessionFileChange) error {
	return s.inner.RecordFileChange(sessionID, change)
}

func (s *SessionService) GetSessionFileChanges(sessionID string, turnIndex int) ([]types.SessionFileChange, error) {
	return s.inner.GetSessionFileChanges(sessionID, turnIndex)
}

func (s *SessionService) ClearSessionFileChanges(sessionID string) error {
	return s.inner.ClearSessionFileChanges(sessionID)
}

// Queued prompt operations
func (s *SessionService) SaveQueuedPrompt(sessionID string, qp types.QueuedPrompt) error {
	return s.inner.SaveQueuedPrompt(sessionID, qp)
}

func (s *SessionService) GetQueuedPrompt(sessionID string) (*types.QueuedPrompt, error) {
	return s.inner.GetQueuedPrompt(sessionID)
}

func (s *SessionService) ClearQueuedPrompt(sessionID string) error {
	return s.inner.ClearQueuedPrompt(sessionID)
}
