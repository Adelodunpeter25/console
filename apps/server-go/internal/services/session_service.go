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
