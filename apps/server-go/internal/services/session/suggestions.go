// Follow-up suggestions: the latest set for a session, kept until the next
// run starts. One row per session database, so nothing lives in memory.
package session

import (
	"database/sql"
	"encoding/json"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/utils"
)

// SaveSessionSuggestions replaces the session's stored suggestions.
func (s *Service) SaveSessionSuggestions(sessionID string, items []string) error {
	projectID, ok, err := s.projectIDBySession(sessionID)
	if err != nil || !ok {
		return err
	}
	conn, err := s.manager.Session(sessionID, projectID)
	if err != nil {
		return err
	}
	raw, err := json.Marshal(items)
	if err != nil {
		return err
	}
	_, err = conn.Exec(
		`INSERT INTO session_suggestions (id, items, created_at) VALUES (1, ?, ?)
		 ON CONFLICT(id) DO UPDATE SET items = excluded.items, created_at = excluded.created_at`,
		string(raw), utils.NowMillis())
	return err
}

// GetSessionSuggestions returns the stored suggestions (empty slice, not
// nil, when there are none).
func (s *Service) GetSessionSuggestions(sessionID string) ([]string, error) {
	projectID, ok, err := s.projectIDBySession(sessionID)
	if err != nil {
		return nil, err
	}
	if !ok {
		return []string{}, nil
	}
	conn, err := s.manager.Session(sessionID, projectID)
	if err != nil {
		return nil, err
	}
	var raw string
	if err := conn.QueryRow(`SELECT items FROM session_suggestions WHERE id = 1`).Scan(&raw); err != nil {
		if err == sql.ErrNoRows {
			return []string{}, nil
		}
		return nil, err
	}
	items := []string{}
	if err := json.Unmarshal([]byte(raw), &items); err != nil {
		return []string{}, nil
	}
	return items, nil
}

// ClearSessionSuggestions drops the stored suggestions.
func (s *Service) ClearSessionSuggestions(sessionID string) error {
	projectID, ok, err := s.projectIDBySession(sessionID)
	if err != nil || !ok {
		return err
	}
	conn, err := s.manager.Session(sessionID, projectID)
	if err != nil {
		return err
	}
	_, err = conn.Exec(`DELETE FROM session_suggestions`)
	return err
}
