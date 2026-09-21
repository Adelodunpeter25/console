// Session mutations beyond create/list/load: restore, permanent delete,
// field updates, status, subagents. Ports of the TS session-ops helpers.
package session

import (
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/types"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/utils"
)

// ExpiredDeletedSessions lists soft-deleted session ids whose deleted_at is
// at or before the cutoff (unix millis), oldest first.
func (s *Service) ExpiredDeletedSessions(cutoffMillis int64) ([]string, error) {
	rows, err := s.manager.Global().Query(
		`SELECT id FROM sessions WHERE deleted_at IS NOT NULL AND deleted_at <= ? ORDER BY deleted_at ASC`,
		cutoffMillis)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	ids := make([]string, 0)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// Restore clears the soft-delete mark. Mirrors the TS restoreSession:
// success for any known id, even one that was not deleted.
func (s *Service) Restore(sessionID string) (bool, error) {
	res, err := s.manager.Global().Exec(
		`UPDATE sessions SET deleted_at = NULL WHERE id = ?`, sessionID)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// PermanentDelete irreversibly removes an already soft-deleted session:
// per-session DB file, scratch working dir, then the index row. Mirrors the
// TS permanentlyDeleteSession (false unless the session is soft-deleted).
func (s *Service) PermanentDelete(sessionID string) (bool, error) {
	var projectID sql.NullString
	var deletedAt sql.NullInt64
	err := s.manager.Global().QueryRow(
		`SELECT project_id, deleted_at FROM sessions WHERE id = ?`, sessionID,
	).Scan(&projectID, &deletedAt)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if !deletedAt.Valid {
		return false, nil
	}

	s.manager.CloseSession(sessionID)

	storageDir := s.manager.StorageDir()
	if storageDir != ":memory:" {
		project := ""
		if projectID.Valid && projectID.String != "" && projectID.String != "scratch" {
			project = projectID.String
		}
		if project != "" {
			removeDbFile(utils.SessionDBPath(storageDir, project, sessionID))
		} else if found := findSessionDbFile(storageDir, sessionID); found != "" {
			removeDbFile(found)
		}
		// Cleanup scratch working directory for scratchpad sessions.
		// Safety: only delete under the scratch root.
		if project == "" {
			scratchRoot := filepath.Join(storageDir, "scratch")
			sessionDir := filepath.Join(scratchRoot, sessionID)
			if sessionDir != scratchRoot {
				if rel, err := filepath.Rel(scratchRoot, sessionDir); err == nil &&
					rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
					if st, err := os.Stat(sessionDir); err == nil && st.IsDir() {
						_ = os.RemoveAll(sessionDir)
					}
				}
			}
		}
	}

	res, err := s.manager.Global().Exec(`DELETE FROM sessions WHERE id = ?`, sessionID)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// removeDbFile mirrors the TS helper: main file plus WAL sidecars.
func removeDbFile(dbPath string) {
	_ = os.Remove(dbPath)
	_ = os.Remove(dbPath + "-wal")
	_ = os.Remove(dbPath + "-shm")
}

// findSessionDbFile mirrors findSessionDbPath: scan project session dirs,
// then scratch, for an orphaned session DB file.
func findSessionDbFile(storageDir, sessionID string) string {
	projectsRoot := filepath.Join(storageDir, "projects")
	entries, err := os.ReadDir(projectsRoot)
	if err == nil {
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			candidate := utils.SessionDBPath(storageDir, e.Name(), sessionID)
			if st, err := os.Stat(candidate); err == nil && !st.IsDir() {
				return candidate
			}
		}
	}
	if candidate := utils.ScratchSessionDBPath(storageDir, sessionID); statFile(candidate) {
		return candidate
	}
	return ""
}

func statFile(path string) bool {
	st, err := os.Stat(path)
	return err == nil && !st.IsDir()
}

// Header returns the indexed header, or nil for unknown/deleted sessions.
func (s *Service) Header(sessionID string) (*types.SessionHeader, error) {
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
	return &headers[0], nil
}

// UpdateModel mirrors the TS updateModel: session_meta (when the DB file
// exists, to avoid materializing empty DBs) plus the global index.
func (s *Service) UpdateModel(sessionID, modelID, provider string) error {
	projectID, _, err := s.projectIDBySession(sessionID)
	if err != nil {
		return err
	}
	now := utils.NowMillis()
	if s.sessionDBExists(sessionID, projectID) {
		conn, err := s.manager.Session(sessionID, projectID)
		if err != nil {
			return err
		}
		if _, err := conn.Exec(
			`UPDATE session_meta SET model_id = ?, provider = ?, updated_at = ? WHERE id = 1`,
			modelID, provider, now); err != nil {
			return err
		}
	}
	_, err = s.manager.Global().Exec(
		`UPDATE sessions SET model_id = ?, provider = ?, updated_at = ? WHERE id = ?`,
		modelID, provider, now, sessionID)
	return err
}

// UpdateCwd mirrors the TS updateCwd: meta + index writes, then file
// relocation when project ownership changes (never clobbers).
func (s *Service) UpdateCwd(sessionID, cwd string, newProjectID *string) error {
	oldProjectID, _, err := s.projectIDBySession(sessionID)
	if err != nil {
		return err
	}
	target := oldProjectID
	if newProjectID != nil {
		target = normalizeProjectID(*newProjectID)
	}
	now := utils.NowMillis()
	trimmed := strings.TrimSpace(cwd)
	var targetNull sql.NullString
	if target != "" {
		targetNull = sql.NullString{String: target, Valid: true}
	}

	if s.sessionDBExists(sessionID, oldProjectID) {
		conn, err := s.manager.Session(sessionID, oldProjectID)
		if err != nil {
			return err
		}
		if _, err := conn.Exec(
			`UPDATE session_meta SET cwd = ?, project_id = ?, updated_at = ? WHERE id = 1`,
			trimmed, targetNull, now); err != nil {
			return err
		}
	}
	if _, err := s.manager.Global().Exec(
		`UPDATE sessions SET cwd = ?, project_id = ?, updated_at = ? WHERE id = ?`,
		trimmed, targetNull, now, sessionID); err != nil {
		return err
	}

	if s.manager.StorageDir() != ":memory:" && oldProjectID != target {
		s.manager.CloseSession(sessionID)
		storageDir := s.manager.StorageDir()
		from := scratchOrProjectPath(storageDir, oldProjectID, sessionID)
		to := scratchOrProjectPath(storageDir, target, sessionID)
		relocateSessionDb(from, to)
	}
	return nil
}

// normalizeProjectID mirrors the TS norm(): empty/"scratch" means no project.
func normalizeProjectID(p string) string {
	if p == "" || p == "scratch" {
		return ""
	}
	return p
}

func scratchOrProjectPath(storageDir, projectID, sessionID string) string {
	if projectID == "" {
		return utils.ScratchSessionDBPath(storageDir, sessionID)
	}
	return utils.SessionDBPath(storageDir, projectID, sessionID)
}

// sessionDBExists reports whether the per-session DB file is already
// materialized (TS checks the open handle or the file for the same reason).
func (s *Service) sessionDBExists(sessionID, projectID string) bool {
	if s.manager.StorageDir() == ":memory:" {
		return false
	}
	return statFile(scratchOrProjectPath(s.manager.StorageDir(), projectID, sessionID))
}

// relocateSessionDb moves a session DB file (plus WAL sidecars), never
// clobbering an existing destination. Mirrors the TS helper.
func relocateSessionDb(fromPath, toPath string) bool {
	if fromPath == toPath || !statFile(fromPath) || statFile(toPath) {
		return false
	}
	moved := false
	for _, suffix := range []string{"", "-wal", "-shm"} {
		src := fromPath + suffix
		if !statFile(src) {
			continue
		}
		dst := toPath + suffix
		if statFile(dst) {
			continue
		}
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			continue
		}
		if err := os.Rename(src, dst); err == nil {
			moved = true
		}
	}
	return moved
}

// UpdateApprovalMode mirrors the TS updateApprovalMode.
func (s *Service) UpdateApprovalMode(sessionID, approvalMode string) error {
	projectID, _, err := s.projectIDBySession(sessionID)
	if err != nil {
		return err
	}
	now := utils.NowMillis()
	if s.sessionDBExists(sessionID, projectID) {
		conn, err := s.manager.Session(sessionID, projectID)
		if err != nil {
			return err
		}
		if _, err := conn.Exec(
			`UPDATE session_meta SET approval_mode = ?, updated_at = ? WHERE id = 1`,
			approvalMode, now); err != nil {
			return err
		}
	}
	_, err = s.manager.Global().Exec(
		`UPDATE sessions SET approval_mode = ?, updated_at = ? WHERE id = ?`,
		approvalMode, now, sessionID)
	return err
}

// UpdateStatus sets the indexed status (used for the working→done settle
// fixup on read, mirroring SessionService.getSession).
func (s *Service) UpdateStatus(sessionID, status string) error {
	_, err := s.manager.Global().Exec(
		`UPDATE sessions SET status = ?, updated_at = ? WHERE id = ?`,
		status, utils.NowMillis(), sessionID)
	return err
}

// GetSubagents mirrors the TS getSessionSubagents: live (running) subagents
// only, empty for unknown sessions.
func (s *Service) GetSubagents(sessionID string) ([]types.SubagentInfo, error) {
	projectID, ok, err := s.projectIDBySession(sessionID)
	if err != nil {
		return nil, err
	}
	if !ok {
		return []types.SubagentInfo{}, nil
	}
	conn, err := s.manager.Session(sessionID, projectID)
	if err != nil {
		return nil, err
	}
	rows, err := conn.Query(`
		SELECT id, parent_tool_call_id, name, role, prompt, current_turn,
			status, summary, error, activities, created_at, updated_at
		FROM session_subagents WHERE status = 'running' ORDER BY created_at ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]types.SubagentInfo, 0)
	for rows.Next() {
		var info types.SubagentInfo
		var summary, errText sql.NullString
		var activities string
		if err := rows.Scan(&info.SubagentID, &info.ParentToolCallID, &info.Name,
			&info.Role, &info.Prompt, &info.CurrentTurn, &info.Status,
			&summary, &errText, &activities, &info.CreatedAt, &info.UpdatedAt); err != nil {
			return nil, err
		}
		if summary.Valid {
			info.Summary = &summary.String
		}
		if errText.Valid {
			info.Error = &errText.String
		}
		info.Activities = []types.SubagentActivityItem{}
		if activities != "" {
			var parsed []types.SubagentActivityItem
			if err := json.Unmarshal([]byte(activities), &parsed); err == nil {
				info.Activities = parsed
			}
		}
		out = append(out, info)
	}
	return out, rows.Err()
}
