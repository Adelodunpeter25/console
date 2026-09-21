// Queued prompt operations: save, get, clear staged prompts.
package session

import (
	"database/sql"
	"encoding/json"
	"time"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/types"
)

// SaveQueuedPrompt upserts the single staged prompt for a session.
func (s *Service) SaveQueuedPrompt(sessionID string, qp types.QueuedPrompt) error {
	projectID, ok, err := s.projectIDBySession(sessionID)
	if err != nil {
		return err
	}
	if !ok {
		return nil
	}
	conn, err := s.manager.Session(sessionID, projectID)
	if err != nil {
		return err
	}
	var attachments any
	if len(qp.Attachments) > 0 {
		raw, err := json.Marshal(qp.Attachments)
		if err != nil {
			return err
		}
		attachments = string(raw)
	}
	createdAt, err := time.Parse(time.RFC3339, qp.CreatedAt)
	if err != nil {
		createdAt = time.Now()
	}
	_, err = conn.Exec(
		`INSERT INTO session_queued_prompt (id, queue_id, prompt, attachments, model_id, provider, approval_mode, created_at)
		 VALUES (1, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(id) DO UPDATE SET queue_id = excluded.queue_id, prompt = excluded.prompt,
		 attachments = excluded.attachments, model_id = excluded.model_id,
		 provider = excluded.provider, approval_mode = excluded.approval_mode,
		 created_at = excluded.created_at`,
		qp.ID, qp.Prompt, attachments, nullString(qp.ModelID), nullString(qp.Provider),
		nullString(qp.ApprovalMode), createdAt.UnixMilli())
	return err
}

// GetQueuedPrompt returns the staged prompt, or nil when none is staged.
func (s *Service) GetQueuedPrompt(sessionID string) (*types.QueuedPrompt, error) {
	projectID, ok, err := s.projectIDBySession(sessionID)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, nil
	}
	conn, err := s.manager.Session(sessionID, projectID)
	if err != nil {
		return nil, err
	}
	var row struct {
		queueID      string
		prompt       string
		attachments  sql.NullString
		modelID      sql.NullString
		provider     sql.NullString
		approvalMode sql.NullString
		createdAt    int64
	}
	err = conn.QueryRow(
		`SELECT queue_id, prompt, attachments, model_id, provider, approval_mode, created_at
		 FROM session_queued_prompt WHERE id = 1`).Scan(
		&row.queueID, &row.prompt, &row.attachments, &row.modelID,
		&row.provider, &row.approvalMode, &row.createdAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	qp := &types.QueuedPrompt{
		ID: row.queueID, SessionID: sessionID, Prompt: row.prompt,
		ModelID: row.modelID.String, Provider: row.provider.String,
		ApprovalMode: row.approvalMode.String,
		CreatedAt:    time.UnixMilli(row.createdAt).UTC().Format(time.RFC3339),
	}
	if row.attachments.Valid && row.attachments.String != "" {
		if err := json.Unmarshal([]byte(row.attachments.String), &qp.Attachments); err != nil {
			qp.Attachments = nil
		}
	}
	return qp, nil
}

// ClearQueuedPrompt discards the staged prompt for a session.
func (s *Service) ClearQueuedPrompt(sessionID string) error {
	projectID, ok, err := s.projectIDBySession(sessionID)
	if err != nil {
		return err
	}
	if !ok {
		return nil
	}
	conn, err := s.manager.Session(sessionID, projectID)
	if err != nil {
		return err
	}
	_, err = conn.Exec(`DELETE FROM session_queued_prompt`)
	return err
}

func nullString(v string) any {
	if v == "" {
		return nil
	}
	return v
}
