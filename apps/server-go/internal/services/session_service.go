// Session service: facade for session operations split into sub-files.
// This maintains backward compatibility while organizing code by functionality.
package services

import (
	"github.com/Adelodunpeter25/console/apps/server-go/internal/db"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/services/session"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/types"
)

type SessionService struct {
	inner *session.Service
}

func NewSessionService(manager *db.DB) *SessionService {
	return &SessionService{inner: session.New(manager)}
}

// Core operations
func (s *SessionService) Create(opts types.CreateSessionOptions) (types.SessionHeader, error) {
	return s.inner.Create(opts)
}

func (s *SessionService) ListFiltered(f session.ListFilter) ([]types.SessionHeader, error) {
	return s.inner.ListFiltered(f)
}

func (s *SessionService) Load(sessionID string, limit int64, before int64) (*types.LoadedSession, error) {
	return s.inner.Load(sessionID, limit, before)
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

func (s *SessionService) PermanentDelete(sessionID string) (bool, error) {
	return s.inner.PermanentDelete(sessionID)
}

func (s *SessionService) UpdateTitle(sessionID, title string) error {
	return s.inner.UpdateTitle(sessionID, title)
}

func (s *SessionService) UpdateModel(sessionID, modelID, provider string) error {
	return s.inner.UpdateModel(sessionID, modelID, provider)
}

func (s *SessionService) UpdateCwd(sessionID, cwd string, projectID *string) error {
	return s.inner.UpdateCwd(sessionID, cwd, projectID)
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
