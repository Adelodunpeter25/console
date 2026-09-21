// Internal helper methods for session service.
package session

import (
	"database/sql"
	"encoding/json"
)

// projectIDBySession returns the project id for a session and whether the
// session row exists at all. A session with no project (scratch) still
// "exists" (ok=true, id=""); ok=false means the id has no row in the
// global sessions table (deleted or never created). Mirrors
// getProjectIdBySessionId, whose callers all no-op on a missing session
// rather than treating it as an error.
func (s *Service) projectIDBySession(sessionID string) (id string, ok bool, err error) {
	var projectID sql.NullString
	err = s.manager.Global().QueryRow(
		`SELECT project_id FROM sessions WHERE id = ?`, sessionID).Scan(&projectID)
	if err == sql.ErrNoRows {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	if projectID.Valid && projectID.String != "" && projectID.String != "scratch" {
		return projectID.String, true, nil
	}
	return "", true, nil
}

func (s *Service) bumpSessionUpdated(sessionID string, now int64, delta int) {
	_, _ = s.manager.Global().Exec(
		`UPDATE sessions SET updated_at = ?, message_count = message_count + ? WHERE id = ?`,
		now, delta, sessionID)
}

// ProjectByDir is the exported wrapper for create/update project inference.
func (s *Service) ProjectByDir(dir string) (string, error) {
	return s.projectByDir(dir)
}

// projectByDir mirrors Projects.getProjectByDir: exact dir match, empty
// string when no project owns the directory.
func (s *Service) projectByDir(dir string) (string, error) {
	var id string
	err := s.manager.Global().QueryRow(
		`SELECT id FROM projects WHERE dir = ?`, dir).Scan(&id)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return id, nil
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
