// Queued prompt operations: the per-session FIFO of prompts that run, oldest
// first, as each turn settles.
package session

import (
	"database/sql"
	"encoding/json"
	"time"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/types"
)

const queuedColumns = `queue_id, prompt, context_files, attachments, annotations, model_id, provider, approval_mode, created_at`

// jsonColumn stores v as JSON text, or NULL when it is empty.
func jsonColumn(empty bool, v any) (any, error) {
	if empty {
		return nil, nil
	}
	raw, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	return string(raw), nil
}

// SaveQueuedPrompt adds the prompt at the back of the queue, or updates it in
// place when its id is already queued.
func (s *Service) SaveQueuedPrompt(sessionID string, qp types.QueuedPrompt) error {
	return s.saveQueuedPrompt(sessionID, qp, false)
}

// SaveQueuedPromptFirst puts a new prompt at the front of the queue so it runs
// next (steering). An id that is already queued is updated in place.
func (s *Service) SaveQueuedPromptFirst(sessionID string, qp types.QueuedPrompt) error {
	return s.saveQueuedPrompt(sessionID, qp, true)
}

func (s *Service) saveQueuedPrompt(sessionID string, qp types.QueuedPrompt, front bool) error {
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
	attachments, err := jsonColumn(len(qp.Attachments) == 0, qp.Attachments)
	if err != nil {
		return err
	}
	contextFiles, err := jsonColumn(len(qp.ContextFiles) == 0, qp.ContextFiles)
	if err != nil {
		return err
	}
	annotations, err := jsonColumn(len(qp.Annotations) == 0, qp.Annotations)
	if err != nil {
		return err
	}
	createdAt, err := time.Parse(time.RFC3339, qp.CreatedAt)
	if err != nil {
		createdAt = time.Now()
	}
	seq := `(SELECT COALESCE(MAX(seq), 0) + 1 FROM session_queue)`
	if front {
		seq = `(SELECT COALESCE(MIN(seq), 1) - 1 FROM session_queue)`
	}
	_, err = conn.Exec(
		`INSERT INTO session_queue (seq, `+queuedColumns+`)
		 VALUES (`+seq+`, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(queue_id) DO UPDATE SET prompt = excluded.prompt,
		 context_files = excluded.context_files,
		 attachments = excluded.attachments, annotations = excluded.annotations, model_id = excluded.model_id,
		 provider = excluded.provider, approval_mode = excluded.approval_mode,
		 created_at = excluded.created_at`,
		qp.ID, qp.Prompt, contextFiles, attachments, annotations, nullString(qp.ModelID), nullString(qp.Provider),
		nullString(qp.ApprovalMode), createdAt.UnixMilli())
	return err
}

// ListQueuedPrompts returns the queue, next-to-run first. Empty when nothing
// is queued.
func (s *Service) ListQueuedPrompts(sessionID string) ([]types.QueuedPrompt, error) {
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
	rows, err := conn.Query(`SELECT ` + queuedColumns + ` FROM session_queue ORDER BY seq`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []types.QueuedPrompt
	for rows.Next() {
		var row struct {
			queueID      string
			prompt       string
			contextFiles sql.NullString
			attachments  sql.NullString
			annotations  sql.NullString
			modelID      sql.NullString
			provider     sql.NullString
			approvalMode sql.NullString
			createdAt    int64
		}
		if err := rows.Scan(&row.queueID, &row.prompt, &row.contextFiles, &row.attachments, &row.annotations,
			&row.modelID, &row.provider, &row.approvalMode, &row.createdAt); err != nil {
			return nil, err
		}
		qp := types.QueuedPrompt{
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
		if row.contextFiles.Valid && row.contextFiles.String != "" {
			if err := json.Unmarshal([]byte(row.contextFiles.String), &qp.ContextFiles); err != nil {
				qp.ContextFiles = nil
			}
		}
		if row.annotations.Valid && row.annotations.String != "" {
			if err := json.Unmarshal([]byte(row.annotations.String), &qp.Annotations); err != nil {
				qp.Annotations = nil
			}
		}
		out = append(out, qp)
	}
	return out, rows.Err()
}

// GetQueuedPrompt returns the prompt that runs next, or nil when none is queued.
func (s *Service) GetQueuedPrompt(sessionID string) (*types.QueuedPrompt, error) {
	queue, err := s.ListQueuedPrompts(sessionID)
	if err != nil || len(queue) == 0 {
		return nil, err
	}
	return &queue[0], nil
}

// PopQueuedPrompt removes and returns the prompt that runs next, or nil when
// none is queued.
func (s *Service) PopQueuedPrompt(sessionID string) (*types.QueuedPrompt, error) {
	first, err := s.GetQueuedPrompt(sessionID)
	if err != nil || first == nil {
		return nil, err
	}
	if _, err := s.DeleteQueuedPrompt(sessionID, first.ID); err != nil {
		return nil, err
	}
	return first, nil
}

// DeleteQueuedPrompt removes one queued prompt and reports whether it existed.
func (s *Service) DeleteQueuedPrompt(sessionID, queueID string) (bool, error) {
	projectID, ok, err := s.projectIDBySession(sessionID)
	if err != nil {
		return false, err
	}
	if !ok {
		return false, nil
	}
	conn, err := s.manager.Session(sessionID, projectID)
	if err != nil {
		return false, err
	}
	res, err := conn.Exec(`DELETE FROM session_queue WHERE queue_id = ?`, queueID)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n > 0, err
}

// ClearQueuedPrompt discards every queued prompt for a session.
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
	_, err = conn.Exec(`DELETE FROM session_queue`)
	return err
}

func nullString(v string) any {
	if v == "" {
		return nil
	}
	return v
}
