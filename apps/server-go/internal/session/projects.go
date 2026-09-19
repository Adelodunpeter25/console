// Project CRUD against the global DB. Port of agent/src/session/projects.ts.
package session

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"os"
)

func randomID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic(err) // crypto/rand never fails on supported platforms
	}
	return fmt.Sprintf("%s-%s-%s-%s-%s",
		hex.EncodeToString(b[0:4]), hex.EncodeToString(b[4:6]),
		hex.EncodeToString(b[6:8]), hex.EncodeToString(b[8:10]),
		hex.EncodeToString(b[10:16]))
}

type createProjectOptions struct {
	ID   string
	Name string
	Dir  string
}

func createProject(db *sql.DB, opts createProjectOptions) (ProjectInfo, error) {
	id := opts.ID
	if id == "" {
		id = randomID()
	}
	now := nowMillis()
	_, err := db.Exec(`
		INSERT INTO projects (id, name, dir, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(dir) DO UPDATE SET
			name = excluded.name,
			updated_at = excluded.updated_at
	`, id, opts.Name, opts.Dir, now, now)
	if err != nil {
		return ProjectInfo{}, err
	}
	return projectByDir(db, opts.Dir)
}

func scanProject(row interface{ Scan(...any) error }) (ProjectInfo, error) {
	var p ProjectInfo
	err := row.Scan(&p.ID, &p.Name, &p.Path, &p.CreatedAt, &p.UpdatedAt)
	return p, err
}

func projectByID(db *sql.DB, id string) (ProjectInfo, error) {
	return scanProject(db.QueryRow(
		`SELECT id, name, dir, created_at, updated_at FROM projects WHERE id = ?`, id))
}

func projectByDir(db *sql.DB, dir string) (ProjectInfo, error) {
	return scanProject(db.QueryRow(
		`SELECT id, name, dir, created_at, updated_at FROM projects WHERE dir = ?`, dir))
}

func listProjects(db *sql.DB) ([]ProjectInfo, error) {
	rows, err := db.Query(
		`SELECT id, name, dir, created_at, updated_at FROM projects ORDER BY updated_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ProjectInfo
	for rows.Next() {
		p, err := scanProject(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// deleteProject closes cached session connections, removes the project's
// storage tree, then clears the global index rows.
func (s *Storage) deleteProject(projectID string) (bool, error) {
	s.mu.Lock()
	rows, err := s.globalDB.Query(`SELECT id FROM sessions WHERE project_id = ?`, projectID)
	if err != nil {
		s.mu.Unlock()
		return false, err
	}
	var sessionIDs []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			s.mu.Unlock()
			return false, err
		}
		sessionIDs = append(sessionIDs, id)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		s.mu.Unlock()
		return false, err
	}
	for _, id := range sessionIDs {
		if db, ok := s.sessionDBs[id]; ok {
			_ = db.Close()
			delete(s.sessionDBs, id)
		}
	}
	s.mu.Unlock()

	_ = os.RemoveAll(ProjectStorageDir(s.storageDir, projectID))

	if _, err := s.globalDB.Exec(`DELETE FROM sessions WHERE project_id = ?`, projectID); err != nil {
		return false, err
	}
	res, err := s.globalDB.Exec(`DELETE FROM projects WHERE id = ?`, projectID)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}
