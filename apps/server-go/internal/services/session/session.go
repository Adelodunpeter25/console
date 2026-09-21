// Core session operations: create, list, load, soft delete.
package session

import (
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
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

// TS parity constants from SessionService.createSession.
const (
	DefaultFallbackModel    = "claude-opus-4-6-thinking"
	DefaultFallbackProvider = "antigravity"
)

// Create resolves TS-parity defaults (model/provider/title/cwd/project,
// scratchpad for explicit-null projectId) then writes the index + meta rows.
func (s *Service) Create(opts types.CreateSessionOptions) (types.SessionHeader, error) {
	id := opts.ID
	title := strings.TrimSpace(opts.Title)
	if title == "" {
		title = "New Session"
	}
	modelID := opts.ModelID
	if modelID == "" {
		modelID = DefaultFallbackModel
	}
	provider := opts.Provider
	if provider == "" {
		provider = DefaultFallbackProvider
	}
	approvalMode := opts.ApprovalMode
	if approvalMode == "" {
		approvalMode = "always-ask"
	}

	cwd := opts.Cwd
	var projectID *string
	if opts.ProjectNull {
		// Explicit null => scratchpad session: fresh id, sandboxed cwd.
		id = utils.RandomID()
		if cwd == "" {
			cwd = filepath.Join(utils.ConsoleStorageDir(), "scratch", id)
		}
		if err := os.MkdirAll(cwd, 0o755); err != nil {
			return types.SessionHeader{}, err
		}
	} else {
		if cwd == "" {
			if wd, err := os.Getwd(); err == nil {
				cwd = wd
			}
		}
		if opts.ProjectID != nil {
			projectID = opts.ProjectID
		} else if cwd != "" {
			if found, err := s.projectByDir(cwd); err == nil && found != "" {
				projectID = &found
			}
		}
	}
	if id == "" {
		id = utils.RandomID()
	}
	now := utils.NowMillis()

	if _, err := s.manager.Global().Exec(`
		INSERT INTO sessions
			(id, title, cwd, project_id, model_id, provider, message_count, status, approval_mode, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, 0, 'idle', ?, ?, ?)`,
		id, title, cwd, projectID, modelID, provider, approvalMode, now, now,
	); err != nil {
		return types.SessionHeader{}, err
	}

	conn, err := s.manager.Session(id, derefString(projectID))
	if err != nil {
		return types.SessionHeader{}, err
	}
	if _, err := conn.Exec(`
		INSERT INTO session_meta
			(id, title, cwd, project_id, model_id, provider, approval_mode, created_at, updated_at)
		VALUES (1, ?, ?, ?, ?, ?, ?, ?, ?)`,
		title, cwd, projectID, modelID, provider, approvalMode, now, now,
	); err != nil {
		return types.SessionHeader{}, err
	}

	return types.SessionHeader{
		ID: id, Title: title, Cwd: cwd, ProjectID: projectID,
		ModelID: modelID, Provider: provider, ApprovalMode: approvalMode,
		CreatedAt: now, UpdatedAt: now, MessageCount: 0, Status: "idle",
	}, nil
}

// ListFilter mirrors the TS listSessions options.
type ListFilter struct {
	Cwd         string
	ProjectID   string
	OnlyDeleted bool
}

// ListFiltered mirrors the TS listSessions: limit 100, newest first, cwd
// takes precedence over projectId, deleted filter flips the condition.
func (s *Service) ListFiltered(f ListFilter) ([]types.SessionHeader, error) {
	const limit = 100
	deletedCondition := "deleted_at IS NULL"
	if f.OnlyDeleted {
		deletedCondition = "deleted_at IS NOT NULL"
	}
	var rows *sql.Rows
	var err error
	switch {
	case f.Cwd != "":
		rows, err = s.manager.Global().Query(`
			SELECT id, title, cwd, project_id, model_id, provider, approval_mode,
				created_at, updated_at, message_count, status, deleted_at
			FROM sessions WHERE cwd = ? AND `+deletedCondition+` ORDER BY updated_at DESC LIMIT ?`, f.Cwd, limit)
	case f.ProjectID != "":
		rows, err = s.manager.Global().Query(`
			SELECT id, title, cwd, project_id, model_id, provider, approval_mode,
				created_at, updated_at, message_count, status, deleted_at
			FROM sessions WHERE project_id = ? AND `+deletedCondition+` ORDER BY updated_at DESC LIMIT ?`, f.ProjectID, limit)
	default:
		rows, err = s.manager.Global().Query(`
			SELECT id, title, cwd, project_id, model_id, provider, approval_mode,
				created_at, updated_at, message_count, status, deleted_at
			FROM sessions WHERE `+deletedCondition+` ORDER BY updated_at DESC LIMIT ?`, limit)
	}
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
// per-session DB. Pagination mirrors the TS loadSession: rowid cursor,
// newest page first, limit <= 0 means all messages. Messages are the stored
// content JSON with createdAt injected, exactly as the desktop parses them.
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

	query := `SELECT content, created_at, rowid FROM messages`
	var args []any
	if before > 0 {
		query += ` WHERE rowid < ?`
		args = append(args, before)
	}
	query += ` ORDER BY rowid DESC`
	if limit > 0 {
		query += ` LIMIT ?`
		args = append(args, limit+1)
	}
	msgRows, err := conn.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer msgRows.Close()

	type row struct {
		content   string
		createdAt int64
		rowid     int64
	}
	fetched := make([]row, 0)
	for msgRows.Next() {
		var r row
		if err := msgRows.Scan(&r.content, &r.createdAt, &r.rowid); err != nil {
			return nil, err
		}
		fetched = append(fetched, r)
	}
	if err := msgRows.Err(); err != nil {
		return nil, err
	}

	hasMore := limit > 0 && int64(len(fetched)) > limit
	if hasMore {
		fetched = fetched[:limit]
	}
	// Oldest first for display.
	messages := make([]json.RawMessage, 0, len(fetched))
	for i := len(fetched) - 1; i >= 0; i-- {
		messages = append(messages, withCreatedAt(fetched[i].content, fetched[i].createdAt))
	}
	result := &types.LoadedSession{Header: headers[0], Messages: messages, HasMore: hasMore}
	if hasMore && len(fetched) > 0 {
		cursor := fetched[len(fetched)-1].rowid
		result.NextCursor = &cursor
	}
	return result, nil
}

// withCreatedAt injects createdAt into a stored message object, mirroring
// the TS loadSession (msg.createdAt = r.created_at). Non-object payloads
// pass through untouched.
func withCreatedAt(content string, createdAt int64) json.RawMessage {
	var obj map[string]any
	if err := json.Unmarshal([]byte(content), &obj); err != nil {
		return json.RawMessage(content)
	}
	obj["createdAt"] = createdAt
	out, err := json.Marshal(obj)
	if err != nil {
		return json.RawMessage(content)
	}
	return out
}

// SoftDelete marks a session deleted; retention purge is 7 days. Mirrors
// the TS deleteSession: unconditional, idempotent success for known ids.
func (s *Service) SoftDelete(sessionID string) (bool, error) {
	now := utils.NowMillis()
	res, err := s.manager.Global().Exec(
		`UPDATE sessions SET deleted_at = ?, updated_at = ? WHERE id = ?`,
		now, now, sessionID)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// UpdateTitle sets the session title in the global index and the
// per-session meta row.
func (s *Service) UpdateTitle(sessionID, title string) error {
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
	now := utils.NowMillis()
	if _, err := conn.Exec(`UPDATE session_meta SET title = ?, updated_at = ? WHERE id = 1`, title, now); err != nil {
		return err
	}
	if _, err := s.manager.Global().Exec(`UPDATE sessions SET title = ?, updated_at = ? WHERE id = ?`, title, now, sessionID); err != nil {
		return err
	}
	return nil
}
