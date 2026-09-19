// Session operations. Port of agent/src/session/session-ops.ts and
// session-messages.ts (initial slice: create, list, load, append,
// replace, soft delete; repair and subagent ops land later in Phase 1).
package session

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
)

const deletedSessionRetentionMs = 7 * 24 * 60 * 60 * 1000

func (s *Storage) CreateSession(opts CreateSessionOptions) (SessionHeader, error) {
	id := opts.ID
	if id == "" {
		id = randomID()
	}
	now := nowMillis()
	title := strings.TrimSpace(opts.Title)
	if title == "" {
		title = "New Session"
	}
	approvalMode := opts.ApprovalMode
	if approvalMode == "" {
		approvalMode = "always-ask"
	}

	if _, err := s.globalDB.Exec(`
		INSERT INTO sessions
			(id, title, cwd, project_id, model_id, provider, message_count, status, approval_mode, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, 0, 'idle', ?, ?, ?)`,
		id, title, opts.Cwd, opts.ProjectID, opts.ModelID, opts.Provider, approvalMode, now, now,
	); err != nil {
		return SessionHeader{}, err
	}

	db, err := s.sessionDB(id, derefString(opts.ProjectID))
	if err != nil {
		return SessionHeader{}, err
	}
	if _, err := db.Exec(`
		INSERT INTO session_meta
			(id, title, cwd, project_id, model_id, provider, approval_mode, created_at, updated_at)
		VALUES (1, ?, ?, ?, ?, ?, ?, ?, ?)`,
		title, opts.Cwd, opts.ProjectID, opts.ModelID, opts.Provider, approvalMode, now, now,
	); err != nil {
		return SessionHeader{}, err
	}

	return SessionHeader{
		ID: id, Title: title, Cwd: opts.Cwd, ProjectID: opts.ProjectID,
		ModelID: opts.ModelID, Provider: opts.Provider, ApprovalMode: approvalMode,
		CreatedAt: now, UpdatedAt: now, MessageCount: 0, Status: "idle",
	}, nil
}

func (s *Storage) ListSessions(minUpdatedAt int64) ([]SessionHeader, error) {
	rows, err := s.globalDB.Query(`
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

func scanSessionRows(rows *sql.Rows) ([]SessionHeader, error) {
	var out []SessionHeader
	for rows.Next() {
		var h SessionHeader
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

type LoadedSession struct {
	Header   SessionHeader  `json:"header"`
	Messages []AgentMessage `json:"messages"`
	HasMore  bool           `json:"hasMore"`
	NextCursor *int64 `json:"nextCursor"`
}

// LoadSession reads the header from the global index and history from the
// per-session DB. limit == 0 means all messages.
func (s *Storage) LoadSession(sessionID string, limit int64, before int64) (*LoadedSession, error) {
	rows, err := s.globalDB.Query(`
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

	projectID, ok := s.projectIDBySession(sessionID)
	if !ok {
		return nil, nil
	}
	db, err := s.sessionDB(sessionID, projectID)
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
	msgRows, err := db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer msgRows.Close()

	var messages []AgentMessage
	var createdAts []int64
	for msgRows.Next() {
		var m AgentMessage
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

	result := &LoadedSession{Header: headers[0], Messages: messages}
	if limit > 0 && int64(len(messages)) > limit {
		result.HasMore = true
		result.Messages = messages[:limit]
		cursor := createdAts[limit-1]
		result.NextCursor = &cursor
	}
	return result, nil
}

func (s *Storage) AppendMessage(sessionID string, msg AgentMessage) error {
	return s.AppendMessages(sessionID, []AgentMessage{msg})
}

// AppendMessages inserts messages transactionally; duplicates by id are
// ignored (INSERT OR IGNORE, matching the TS path).
func (s *Storage) AppendMessages(sessionID string, messages []AgentMessage) error {
	if len(messages) == 0 {
		return nil
	}
	projectID, ok := s.projectIDBySession(sessionID)
	if !ok {
		return fmt.Errorf("session %s not found", sessionID)
	}
	db, err := s.sessionDB(sessionID, projectID)
	if err != nil {
		return err
	}

	now := nowMillis()
	tx, err := db.Begin()
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
			_ = tx.Rollback()
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
func (s *Storage) ReplaceMessages(sessionID string, messages []AgentMessage) error {
	projectID, ok := s.projectIDBySession(sessionID)
	if !ok {
		return fmt.Errorf("session %s not found", sessionID)
	}
	db, err := s.sessionDB(sessionID, projectID)
	if err != nil {
		return err
	}

	now := nowMillis()
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM messages`); err != nil {
		_ = tx.Rollback()
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
			_ = tx.Rollback()
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	s.bumpSessionUpdated(sessionID, now, len(messages))
	return nil
}

// SoftDeleteSession marks a session deleted; retention purge is 7 days.
func (s *Storage) SoftDeleteSession(sessionID string) (bool, error) {
	res, err := s.globalDB.Exec(
		`UPDATE sessions SET deleted_at = ?, updated_at = ? WHERE id = ? AND deleted_at IS NULL`,
		nowMillis(), nowMillis(), sessionID)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

func derefString(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}
