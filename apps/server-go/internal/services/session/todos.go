// Todo operations: save, get, clear.
package session

import (
	"github.com/Adelodunpeter25/console/apps/server-go/internal/types"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/utils"
)

// SaveSessionTodos replaces the persisted todo list for a session
// (delete-then-insert, matching the TS session-todos.ts semantics).
func (s *Service) SaveSessionTodos(sessionID string, items []types.TodoItem) error {
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
func (s *Service) GetSessionTodos(sessionID string) ([]types.TodoItem, error) {
	projectID, ok, err := s.projectIDBySession(sessionID)
	if err != nil {
		return nil, err
	}
	if !ok {
		return []types.TodoItem{}, nil
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
func (s *Service) ClearSessionTodos(sessionID string) error {
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
	_, err = conn.Exec(`DELETE FROM session_todos`)
	return err
}

// ClearCompletedTodos wipes the session's todo list once every item is
// "completed" — matching RunService's end-of-run cleanup in the TS server
// (finally block of runAgentStream): a finished list is cleared so the next
// run starts fresh, but a partially-done list is left alone. Call this
// after an agent run settles, not from the todo tool itself.
func (s *Service) ClearCompletedTodos(sessionID string) error {
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
