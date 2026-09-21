// Core session operations: create, list, load, soft delete.
package session

import (
	"database/sql"
	"encoding/json"
	"strings"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/db"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/types"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/utils"
)

type Service struct {
	manager *db.DB
}

func New(manager *db.DB) *Service {
	return &Service{manager: manager}
}

func (s *Service) Create(opts types.CreateSessionOptions) (types.SessionHeader, error) {
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

func (s *Service) List(minUpdatedAt int64) ([]types.SessionHeader, error) {
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
func (s *Service) Load(sessionID string, limit int64, before int64) (*types.LoadedSession, error) {
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

// SoftDelete marks a session deleted; retention purge is 7 days.
func (s *Service) SoftDelete(sessionID string) (bool, error) {
	res, err := s.manager.Global().Exec(
		`UPDATE sessions SET deleted_at = ?, updated_at = ? WHERE id = ? AND deleted_at IS NULL`,
		utils.NowMillis(), utils.NowMillis(), sessionID)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// UpdateTitle sets the session title in the global index and the
// per-session meta row.
func (s *Service) UpdateTitle(sessionID, title string) error {
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
