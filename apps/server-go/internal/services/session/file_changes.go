// File change operations: record, get, clear session file changes.
package session

import (
	"database/sql"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/types"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/utils"
)

// RecordFileChange inserts or updates a file change for a session.
func (s *Service) RecordFileChange(sessionID string, change types.SessionFileChange) error {
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
func (s *Service) GetSessionFileChanges(sessionID string, turnIndex int) ([]types.SessionFileChange, error) {
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
func (s *Service) ClearSessionFileChanges(sessionID string) error {
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
