// Fork operations: duplicate a session's history up to a message point.
//
// A fork is a pure logical copy: new id, same cwd/project/model, same
// worktree path (shared, never provisioned), messages through messageID
// inclusive, file changes for those turns, and the current todo list.
// Subagents, queued prompts, and run state are never copied.
package session

import (
	"database/sql"
	"errors"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/types"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/utils"
)

// ErrForkSessionNotFound is returned when the source session is unknown
// or soft-deleted.
var ErrForkSessionNotFound = errors.New("session not found")

// ErrForkMessageNotFound is returned when messageID is empty or not part
// of the source session's history.
var ErrForkMessageNotFound = errors.New("message not found")

type forkMessage struct {
	id        string
	role      string
	content   string
	createdAt int64
}

// Fork duplicates the source session through messageID (inclusive) into a
// new idle session and returns its header.
func (s *Service) Fork(sourceID, messageID string) (types.SessionHeader, error) {
	if messageID == "" {
		return types.SessionHeader{}, ErrForkMessageNotFound
	}
	src, err := s.Header(sourceID)
	if err != nil {
		return types.SessionHeader{}, err
	}
	if src == nil {
		return types.SessionHeader{}, ErrForkSessionNotFound
	}
	srcProject := projectIDFrom(src.ProjectID)
	srcConn, err := s.manager.Session(sourceID, srcProject)
	if err != nil {
		return types.SessionHeader{}, err
	}

	// History cutoff: every message up to messageID in rowid order.
	rows, err := srcConn.Query(
		`SELECT id, role, content, created_at FROM messages ORDER BY rowid ASC`)
	if err != nil {
		return types.SessionHeader{}, err
	}
	copied := make([]forkMessage, 0)
	userIDs := make(map[string]struct{})
	userCount := 0
	found := false
	for rows.Next() {
		var m forkMessage
		if err := rows.Scan(&m.id, &m.role, &m.content, &m.createdAt); err != nil {
			rows.Close()
			return types.SessionHeader{}, err
		}
		copied = append(copied, m)
		if m.role == "user" {
			userIDs[m.id] = struct{}{}
			userCount++
		}
		if m.id == messageID {
			found = true
			break
		}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return types.SessionHeader{}, err
	}
	rows.Close()
	if !found {
		return types.SessionHeader{}, ErrForkMessageNotFound
	}

	// Current todos snapshot (no history table, so the whole list carries).
	todoRows, err := srcConn.Query(
		`SELECT id, content, status FROM session_todos ORDER BY id ASC`)
	if err != nil {
		return types.SessionHeader{}, err
	}
	type todoRow struct {
		id      int
		content string
		status  string
	}
	todos := make([]todoRow, 0)
	for todoRows.Next() {
		var tr todoRow
		if err := todoRows.Scan(&tr.id, &tr.content, &tr.status); err != nil {
			todoRows.Close()
			return types.SessionHeader{}, err
		}
		todos = append(todos, tr)
	}
	if err := todoRows.Err(); err != nil {
		todoRows.Close()
		return types.SessionHeader{}, err
	}
	todoRows.Close()

	// File changes for the copied turns: modern rows join on the copied
	// user message ids (one turn per user message, changes recorded under
	// that turn's user message id); legacy rows with no user message id
	// fall back to the turn-index cutoff.
	changeRows, err := srcConn.Query(
		`SELECT path, turn_index, user_message_id, status, additions, deletions, diff_text, reviewed, updated_at FROM session_file_changes`)
	if err != nil {
		return types.SessionHeader{}, err
	}
	type changeRow struct {
		change   types.SessionFileChange
		reviewed int
	}
	changes := make([]changeRow, 0)
	for changeRows.Next() {
		var c types.SessionFileChange
		var diffText sql.NullString
		var reviewed int
		if err := changeRows.Scan(&c.Path, &c.TurnIndex, &c.UserMessageID,
			&c.Status, &c.Additions, &c.Deletions, &diffText, &reviewed,
			&c.UpdatedAt); err != nil {
			changeRows.Close()
			return types.SessionHeader{}, err
		}
		if diffText.Valid {
			v := diffText.String
			c.DiffText = &v
		}
		keep := false
		if c.UserMessageID != "" {
			_, keep = userIDs[c.UserMessageID]
		} else if c.TurnIndex < userCount {
			keep = true
		}
		if keep {
			changes = append(changes, changeRow{change: c, reviewed: reviewed})
		}
	}
	if err := changeRows.Err(); err != nil {
		changeRows.Close()
		return types.SessionHeader{}, err
	}
	changeRows.Close()

	newID := utils.RandomID()
	now := utils.NowMillis()
	title := utils.TitleFirst("Fork of " + src.Title)

	if _, err := s.manager.Global().Exec(`
		INSERT INTO sessions
			(id, title, cwd, project_id, model_id, provider, message_count, status, approval_mode, thinking_level, created_at, updated_at,
				worktree_path, worktree_branch, worktree_repo)
		VALUES (?, ?, ?, ?, ?, ?, ?, 'idle', ?, ?, ?, ?,
			?, ?, ?)`,
		newID, title, src.Cwd, src.ProjectID, src.ModelID, src.Provider,
		len(copied), src.ApprovalMode, src.ThinkingLevel, now, now,
		worktreeCol(src.Worktree, func(w *types.SessionWorktree) string { return w.Path }),
		worktreeCol(src.Worktree, func(w *types.SessionWorktree) string { return w.Branch }),
		worktreeCol(src.Worktree, func(w *types.SessionWorktree) string { return w.Repo }),
	); err != nil {
		return types.SessionHeader{}, err
	}
	cleanupGlobal := func() {
		_, _ = s.manager.Global().Exec(`DELETE FROM sessions WHERE id = ?`, newID)
		s.manager.CloseSession(newID)
	}

	destConn, err := s.manager.Session(newID, srcProject)
	if err != nil {
		cleanupGlobal()
		return types.SessionHeader{}, err
	}
	if _, err := destConn.Exec(`
		INSERT INTO session_meta
			(id, title, cwd, project_id, model_id, provider, approval_mode, thinking_level, created_at, updated_at)
		VALUES (1, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		title, src.Cwd, src.ProjectID, src.ModelID, src.Provider,
		src.ApprovalMode, src.ThinkingLevel, now, now,
	); err != nil {
		cleanupGlobal()
		return types.SessionHeader{}, err
	}

	tx, err := destConn.Begin()
	if err != nil {
		cleanupGlobal()
		return types.SessionHeader{}, err
	}
	for _, m := range copied {
		if _, err := tx.Exec(
			`INSERT INTO messages (id, role, content, created_at) VALUES (?, ?, ?, ?)`,
			m.id, m.role, m.content, m.createdAt); err != nil {
			tx.Rollback()
			cleanupGlobal()
			return types.SessionHeader{}, err
		}
	}
	for _, tr := range todos {
		if _, err := tx.Exec(
			`INSERT INTO session_todos (id, content, status, updated_at) VALUES (?, ?, ?, ?)`,
			tr.id, tr.content, tr.status, now); err != nil {
			tx.Rollback()
			cleanupGlobal()
			return types.SessionHeader{}, err
		}
	}
	for _, cr := range changes {
		if _, err := tx.Exec(
			`INSERT INTO session_file_changes (path, turn_index, user_message_id, status, additions, deletions, diff_text, reviewed, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			cr.change.Path, cr.change.TurnIndex, cr.change.UserMessageID,
			cr.change.Status, cr.change.Additions, cr.change.Deletions,
			cr.change.DiffText, cr.reviewed, cr.change.UpdatedAt); err != nil {
			tx.Rollback()
			cleanupGlobal()
			return types.SessionHeader{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		cleanupGlobal()
		return types.SessionHeader{}, err
	}

	return types.SessionHeader{
		ID: newID, Title: title, Cwd: src.Cwd, ProjectID: src.ProjectID,
		ModelID: src.ModelID, Provider: src.Provider,
		ApprovalMode: src.ApprovalMode, ThinkingLevel: src.ThinkingLevel,
		CreatedAt: now, UpdatedAt: now, MessageCount: len(copied),
		Status:   "idle",
		Worktree: src.Worktree,
	}, nil
}

// CountWorktreeOwners reports how many session rows (any deleted state)
// claim worktreePath, excluding excludeID. Forks share one worktree path,
// so deletes must not pull the directory while another owner remains.
func (s *Service) CountWorktreeOwners(worktreePath, excludeID string) (int, error) {
	var n int
	err := s.manager.Global().QueryRow(
		`SELECT COUNT(*) FROM sessions WHERE worktree_path = ? AND id != ?`,
		worktreePath, excludeID).Scan(&n)
	return n, err
}
