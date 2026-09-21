// Session operations: create, list, load with cursor pagination, message
// append/replace, soft delete. Port of agent/src/session/session-ops.ts
// and session-messages.ts (initial slice).
package services

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/db"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/types"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/utils"
)

type SessionService struct {
	manager *db.DB
}

func NewSessionService(manager *db.DB) *SessionService {
	return &SessionService{manager: manager}
}

func (s *SessionService) Create(opts types.CreateSessionOptions) (types.SessionHeader, error) {
	id := opts.ID
	if id == "" {
		id = utils.RandomID()
	}
	now := utils.NowMillis()
	title := strings.TrimSpace(opts.Title)
	if title == "" {
		title = "New Session"
	}
	approvalMode := opts.ApprovalMode
	if approvalMode == "" {
		approvalMode = "always-ask"
	}

	if _, err := s.manager.Global().Exec(`
		INSERT INTO sessions
			(id, title, cwd, project_id, model_id, provider, message_count, status, approval_mode, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, 0, 'idle', ?, ?, ?)`,
		id, title, opts.Cwd, opts.ProjectID, opts.ModelID, opts.Provider, approvalMode, now, now,
	); err != nil {
		return types.SessionHeader{}, err
	}

	conn, err := s.manager.Session(id, derefString(opts.ProjectID))
	if err != nil {
		return types.SessionHeader{}, err
	}
	if _, err := conn.Exec(`
		INSERT INTO session_meta
			(id, title, cwd, project_id, model_id, provider, approval_mode, created_at, updated_at)
		VALUES (1, ?, ?, ?, ?, ?, ?, ?, ?)`,
		title, opts.Cwd, opts.ProjectID, opts.ModelID, opts.Provider, approvalMode, now, now,
	); err != nil {
		return types.SessionHeader{}, err
	}

	return types.SessionHeader{
		ID: id, Title: title, Cwd: opts.Cwd, ProjectID: opts.ProjectID,
		ModelID: opts.ModelID, Provider: opts.Provider, ApprovalMode: approvalMode,
		CreatedAt: now, UpdatedAt: now, MessageCount: 0, Status: "idle",
	}, nil
}

func (s *SessionService) List(minUpdatedAt int64) ([]types.SessionHeader, error) {
	rows, err := s.manager.Global().Query(`
		SELECT id, title, cwd, project_id, model_id, provider, approval_mode,
			created_at, updated_at, message_count, status, deleted_at
		FROM sessions
		WHERE deleted_at IS NULL AND updated_at > ?
		ORDER BY updated_at DESC`, minUpdatedAt)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanSessionRows(rows)
}

func scanSessionRows(rows *sql.Rows) ([]types.SessionHeader, error) {
	out := make([]types.SessionHeader, 0)
	for rows.Next() {
		var h types.SessionHeader
		var projectID sql.NullString
		var deletedAt sql.NullInt64
		if err := rows.Scan(&h.ID, &h.Title, &h.Cwd, &projectID, &h.ModelID, &h.Provider,
			&h.ApprovalMode, &h.CreatedAt, &h.UpdatedAt, &h.MessageCount, &h.Status, &deletedAt); err != nil {
			return nil, err
		}
		if projectID.Valid && projectID.String != "" && projectID.String != "scratch" {
			id := projectID.String
			h.ProjectID = &id
		}
		if deletedAt.Valid {
			v := deletedAt.Int64
			h.DeletedAt = &v
		}
		out = append(out, h)
	}
	return out, rows.Err()
}

// Load reads the header from the global index and history from the
// per-session DB. limit == 0 means all messages.
func (s *SessionService) Load(sessionID string, limit int64, before int64) (*types.LoadedSession, error) {
	rows, err := s.manager.Global().Query(`
		SELECT id, title, cwd, project_id, model_id, provider, approval_mode,
			created_at, updated_at, message_count, status, deleted_at
		FROM sessions WHERE id = ?`, sessionID)
	if err != nil {
		return nil, err
	}
	headers, err := scanSessionRows(rows)
	rows.Close()
	if err != nil {
		return nil, err
	}
	if len(headers) == 0 || headers[0].DeletedAt != nil {
		return nil, nil
	}

	conn, err := s.manager.Session(sessionID, projectIDFrom(headers[0].ProjectID))
	if err != nil {
		return nil, err
	}

	query := `SELECT id, role, content, created_at FROM messages`
	var args []any
	if before > 0 {
		query += ` WHERE created_at < ?`
		args = append(args, before)
	}
	query += ` ORDER BY created_at ASC, id ASC`
	if limit > 0 {
		query += ` LIMIT ?`
		args = append(args, limit+1)
	}
	msgRows, err := conn.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer msgRows.Close()

	messages := make([]types.AgentMessage, 0)
	var createdAts []int64
	for msgRows.Next() {
		var m types.AgentMessage
		var content string
		var createdAt int64
		if err := msgRows.Scan(&m.ID, &m.Role, &content, &createdAt); err != nil {
			return nil, err
		}
		m.Data = json.RawMessage(content)
		messages = append(messages, m)
		createdAts = append(createdAts, createdAt)
	}
	if err := msgRows.Err(); err != nil {
		return nil, err
	}

	result := &types.LoadedSession{Header: headers[0], Messages: messages}
	if limit > 0 && int64(len(messages)) > limit {
		result.HasMore = true
		result.Messages = messages[:limit]
		cursor := createdAts[limit-1]
		result.NextCursor = &cursor
	}
	return result, nil
}

func (s *SessionService) AppendMessage(sessionID string, msg types.AgentMessage) error {
	return s.AppendMessages(sessionID, []types.AgentMessage{msg})
}

// AppendMessages inserts messages transactionally; duplicates by id are
// ignored (INSERT OR IGNORE, matching the TS path).
func (s *SessionService) AppendMessages(sessionID string, messages []types.AgentMessage) error {
	if len(messages) == 0 {
		return nil
	}
	projectID, err := s.projectIDBySession(sessionID)
	if err != nil {
		return err
	}
	conn, err := s.manager.Session(sessionID, projectID)
	if err != nil {
		return err
	}

	now := utils.NowMillis()
	tx, err := conn.Begin()
	if err != nil {
		return err
	}
	inserted := 0
	for _, msg := range messages {
		id := msg.ID
		if id == "" {
			sum := sha256.Sum256(msg.Data)
			id = hex.EncodeToString(sum[:])[:32]
		}
		res, err := tx.Exec(
			`INSERT OR IGNORE INTO messages (id, role, content, created_at) VALUES (?, ?, ?, ?)`,
			id, msg.Role, string(msg.Data), now)
		if err != nil {
			tx.Rollback()
			return err
		}
		if n, _ := res.RowsAffected(); n > 0 {
			inserted++
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	s.bumpSessionUpdated(sessionID, now, inserted)
	return nil
}

// ReplaceMessages rewrites session history after repairing an interrupted
// tool turn.
func (s *SessionService) ReplaceMessages(sessionID string, messages []types.AgentMessage) error {
	projectID, err := s.projectIDBySession(sessionID)
	if err != nil {
		return err
	}
	conn, err := s.manager.Session(sessionID, projectID)
	if err != nil {
		return err
	}

	now := utils.NowMillis()
	tx, err := conn.Begin()
	if err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM messages`); err != nil {
		tx.Rollback()
		return err
	}
	for i, msg := range messages {
		id := msg.ID
		if id == "" {
			sum := sha256.Sum256([]byte(fmt.Sprintf("%d:%s", i, msg.Data)))
			id = hex.EncodeToString(sum[:])
		}
		if _, err := tx.Exec(
			`INSERT INTO messages (id, role, content, created_at) VALUES (?, ?, ?, ?)`,
			id, msg.Role, string(msg.Data), now); err != nil {
			tx.Rollback()
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	s.bumpSessionUpdated(sessionID, now, len(messages))
	return nil
}

// SoftDelete marks a session deleted; retention purge is 7 days.
func (s *SessionService) SoftDelete(sessionID string) (bool, error) {
	res, err := s.manager.Global().Exec(
		`UPDATE sessions SET deleted_at = ?, updated_at = ? WHERE id = ? AND deleted_at IS NULL`,
		utils.NowMillis(), utils.NowMillis(), sessionID)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// SaveSessionTodos replaces the persisted todo list for a session
// (delete-then-insert, matching the TS session-todos.ts semantics).
func (s *SessionService) SaveSessionTodos(sessionID string, items []types.TodoItem) error {
	projectID, err := s.projectIDBySession(sessionID)
	if err != nil {
		return err
	}
	conn, err := s.manager.Session(sessionID, projectID)
	if err != nil {
		return err
	}
	tx, err := conn.Begin()
	if err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM session_todos`); err != nil {
		tx.Rollback()
		return err
	}
	now := utils.NowMillis()
	for _, item := range items {
		if _, err := tx.Exec(
			`INSERT INTO session_todos (id, content, status, updated_at) VALUES (?, ?, ?, ?)`,
			item.ID, item.Content, item.Status, now); err != nil {
			tx.Rollback()
			return err
		}
	}
	return tx.Commit()
}

// GetSessionTodos returns the persisted todo list for a session, ordered
// by id (empty slice, not nil, when there is none).
func (s *SessionService) GetSessionTodos(sessionID string) ([]types.TodoItem, error) {
	projectID, err := s.projectIDBySession(sessionID)
	if err != nil {
		return nil, err
	}
	conn, err := s.manager.Session(sessionID, projectID)
	if err != nil {
		return nil, err
	}
	rows, err := conn.Query(`SELECT id, content, status FROM session_todos ORDER BY id ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]types.TodoItem, 0)
	for rows.Next() {
		var item types.TodoItem
		if err := rows.Scan(&item.ID, &item.Content, &item.Status); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

// ClearSessionTodos deletes the persisted todo list for a session.
func (s *SessionService) ClearSessionTodos(sessionID string) error {
	projectID, err := s.projectIDBySession(sessionID)
	if err != nil {
		return err
	}
	conn, err := s.manager.Session(sessionID, projectID)
	if err != nil {
		return err
	}
	_, err = conn.Exec(`DELETE FROM session_todos`)
	return err
}

// ClearCompletedTodos wipes the session's todo list once every item is
// "completed" — matching RunService's end-of-run cleanup in the TS server
// (finally block of runAgentStream): a finished list is cleared so the next
// run starts fresh, but a partially-done list is left alone. Call this
// after an agent run settles, not from the todo tool itself.
func (s *SessionService) ClearCompletedTodos(sessionID string) error {
	items, err := s.GetSessionTodos(sessionID)
	if err != nil {
		return err
	}
	if len(items) == 0 {
		return nil
	}
	for _, item := range items {
		if item.Status != "completed" {
			return nil
		}
	}
	return s.ClearSessionTodos(sessionID)
}

// SaveQueuedPrompt upserts the single staged prompt for a session.
// Mirrors saveQueuedPrompt in session-queue.ts.
func (s *SessionService) SaveQueuedPrompt(sessionID string, qp types.QueuedPrompt) error {
	projectID, err := s.projectIDBySession(sessionID)
	if err != nil {
		return err
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
func (s *SessionService) GetQueuedPrompt(sessionID string) (*types.QueuedPrompt, error) {
	projectID, err := s.projectIDBySession(sessionID)
	if err != nil {
		return nil, err
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
func (s *SessionService) ClearQueuedPrompt(sessionID string) error {
	projectID, err := s.projectIDBySession(sessionID)
	if err != nil {
		return err
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

// UpdateTitle sets the session title in the global index and the
// per-session meta row. Mirrors updateTitle in session-ops.
func (s *SessionService) UpdateTitle(sessionID, title string) error {
	projectID, err := s.projectIDBySession(sessionID)
	if err != nil {
		return err
	}
	conn, err := s.manager.Session(sessionID, projectID)
	if err != nil {
		return err
	}
	now := utils.NowMillis()
	if _, err := conn.Exec(`UPDATE session_meta SET title = ?, updated_at = ? WHERE id = 1`, title, now); err != nil {
		return err
	}
	if _, err := s.manager.Global().Exec(`UPDATE sessions SET title = ?, updated_at = ? WHERE id = ?`, title, now, sessionID); err != nil {
		return err
	}
	return nil
}

func (s *SessionService) projectIDBySession(sessionID string) (string, error) {
	var projectID sql.NullString
	err := s.manager.Global().QueryRow(
		`SELECT project_id FROM sessions WHERE id = ?`, sessionID).Scan(&projectID)
	if err != nil {
		return "", err
	}
	if projectID.Valid && projectID.String != "" && projectID.String != "scratch" {
		return projectID.String, nil
	}
	return "", nil
}

func (s *SessionService) bumpSessionUpdated(sessionID string, now int64, delta int) {
	_, _ = s.manager.Global().Exec(
		`UPDATE sessions SET updated_at = ?, message_count = message_count + ? WHERE id = ?`,
		now, delta, sessionID)
}

func derefString(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

func projectIDFrom(p *string) string {
	return derefString(p)
}

// RecordFileChange inserts or updates a file change for a session.
// Mirrors recordFileChange in session-changes.ts.
func (s *SessionService) RecordFileChange(sessionID string, change types.SessionFileChange) error {
	projectID, err := s.projectIDBySession(sessionID)
	if err != nil {
		return err
	}
	conn, err := s.manager.Session(sessionID, projectID)
	if err != nil {
		return err
	}
	now := utils.NowMillis()
	_, err = conn.Exec(`
		INSERT INTO session_file_changes (path, turn_index, status, additions, deletions, diff_text, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(path, turn_index) DO UPDATE SET
			status = excluded.status,
			additions = excluded.additions,
			deletions = excluded.deletions,
			diff_text = excluded.diff_text,
			updated_at = excluded.updated_at`,
		change.Path, change.TurnIndex, change.Status, change.Additions, change.Deletions,
		change.DiffText, now)
	return err
}

// GetSessionFileChanges retrieves file changes for a session.
// If turnIndex is >= 0, filters to that turn only; otherwise returns all changes.
// Mirrors getSessionFileChanges in session-changes.ts.
func (s *SessionService) GetSessionFileChanges(sessionID string, turnIndex int) ([]types.SessionFileChange, error) {
	projectID, err := s.projectIDBySession(sessionID)
	if err != nil {
		return nil, err
	}
	conn, err := s.manager.Session(sessionID, projectID)
	if err != nil {
		return nil, err
	}

	query := `SELECT path, turn_index, status, additions, deletions, diff_text, updated_at FROM session_file_changes`
	var args []any
	if turnIndex >= 0 {
		query += ` WHERE turn_index = ?`
		args = append(args, turnIndex)
	}
	query += ` ORDER BY updated_at DESC`

	rows, err := conn.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	changes := make([]types.SessionFileChange, 0)
	for rows.Next() {
		var change types.SessionFileChange
		var diffText sql.NullString
		if err := rows.Scan(&change.Path, &change.TurnIndex, &change.Status, &change.Additions,
			&change.Deletions, &diffText, &change.UpdatedAt); err != nil {
			return nil, err
		}
		if diffText.Valid {
			change.DiffText = &diffText.String
		}
		changes = append(changes, change)
	}
	return changes, rows.Err()
}

// ClearSessionFileChanges removes all file changes for a session.
// Mirrors clearSessionFileChanges in session-changes.ts.
func (s *SessionService) ClearSessionFileChanges(sessionID string) error {
	projectID, err := s.projectIDBySession(sessionID)
	if err != nil {
		return err
	}
	conn, err := s.manager.Session(sessionID, projectID)
	if err != nil {
		return err
	}
	_, err = conn.Exec(`DELETE FROM session_file_changes`)
	return err
}
