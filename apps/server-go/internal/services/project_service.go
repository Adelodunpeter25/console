// Project CRUD against the global DB.
package services

import (
	"os"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/db"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/types"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/utils"
)

type ProjectService struct {
	manager *db.DB
}

func NewProjectService(manager *db.DB) *ProjectService {
	return &ProjectService{manager: manager}
}

type CreateProjectOptions struct {
	ID   string
	Name string
	Dir  string
}

func (s *ProjectService) Create(opts CreateProjectOptions) (types.ProjectInfo, error) {
	id := opts.ID
	if id == "" {
		id = utils.RandomID()
	}
	now := utils.NowMillis()
	_, err := s.manager.Global().Exec(`
		INSERT INTO projects (id, name, dir, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(dir) DO UPDATE SET
			name = excluded.name,
			updated_at = excluded.updated_at
	`, id, opts.Name, opts.Dir, now, now)
	if err != nil {
		return types.ProjectInfo{}, err
	}
	return s.GetByDir(opts.Dir)
}

func scanProject(row interface{ Scan(...any) error }) (types.ProjectInfo, error) {
	var p types.ProjectInfo
	err := row.Scan(&p.ID, &p.Name, &p.Path, &p.CreatedAt, &p.UpdatedAt)
	return p, err
}

func (s *ProjectService) Get(projectID string) (types.ProjectInfo, error) {
	return scanProject(s.manager.Global().QueryRow(
		`SELECT id, name, dir, created_at, updated_at FROM projects WHERE id = ?`, projectID))
}

func (s *ProjectService) GetByDir(dir string) (types.ProjectInfo, error) {
	return scanProject(s.manager.Global().QueryRow(
		`SELECT id, name, dir, created_at, updated_at FROM projects WHERE dir = ?`, dir))
}

func (s *ProjectService) List() ([]types.ProjectInfo, error) {
	rows, err := s.manager.Global().Query(
		`SELECT id, name, dir, created_at, updated_at FROM projects ORDER BY updated_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]types.ProjectInfo, 0)
	for rows.Next() {
		p, err := scanProject(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// Delete closes cached session connections, removes the project's storage
// tree, then clears the global index rows.
func (s *ProjectService) Delete(projectID string) (bool, error) {
	rows, err := s.manager.Global().Query(`SELECT id FROM sessions WHERE project_id = ?`, projectID)
	if err != nil {
		return false, err
	}
	var sessionIDs []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return false, err
		}
		sessionIDs = append(sessionIDs, id)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return false, err
	}
	for _, id := range sessionIDs {
		s.manager.CloseSession(id)
	}

	os.RemoveAll(utils.ProjectStorageDir(s.manager.StorageDir(), projectID))

	if _, err := s.manager.Global().Exec(`DELETE FROM sessions WHERE project_id = ?`, projectID); err != nil {
		return false, err
	}
	res, err := s.manager.Global().Exec(`DELETE FROM projects WHERE id = ?`, projectID)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}
