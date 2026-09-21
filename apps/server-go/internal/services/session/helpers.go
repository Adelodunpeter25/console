// Internal helper methods for session service.
package session

import (
	"database/sql"
	"encoding/json"
)

func (s *Service) projectIDBySession(sessionID string) (string, error) {
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

func (s *Service) bumpSessionUpdated(sessionID string, now int64, delta int) {
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

func jsonEncodeMessage(i int, data json.RawMessage) string {
	return jsonEncode(map[string]any{"index": i, "data": data})
}

func jsonEncode(v any) string {
	raw, _ := json.Marshal(v)
	return string(raw)
}
