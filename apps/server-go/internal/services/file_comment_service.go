// Durable file comments against the global DB.
package services

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/db"
	consolev1 "github.com/Adelodunpeter25/console/apps/server-go/internal/gen/console/v1"
)

var ErrFileCommentNotFound = errors.New("comment not found")

type FileCommentService struct {
	manager *db.DB
}

func NewFileCommentService(manager *db.DB) *FileCommentService {
	return &FileCommentService{manager: manager}
}

const diffCommentColumns = `id, project_id, path, side, start_line, end_line, text, snippet, created_at, updated_at`

type rowScanner interface{ Scan(dest ...any) error }

func scanFileComment(row rowScanner) (*consolev1.FileComment, error) {
	var c consolev1.FileComment
	var side, start, end int64
	if err := row.Scan(&c.Id, &c.ProjectId, &c.Path, &side, &start, &end,
		&c.Text, &c.Snippet, &c.CreatedAt, &c.UpdatedAt); err != nil {
		return nil, err
	}
	c.Side = consolev1.FileCommentSide(side)
	c.StartLine = uint32(start)
	c.EndLine = uint32(end)
	return &c, nil
}

// List returns a project's comments, oldest first. An empty path means every file.
func (s *FileCommentService) List(projectID, path string) ([]*consolev1.FileComment, error) {
	query := `SELECT ` + diffCommentColumns + ` FROM file_comments WHERE project_id = ?`
	args := []any{projectID}
	if path != "" {
		query += ` AND path = ?`
		args = append(args, path)
	}
	rows, err := s.manager.Global().Query(query+` ORDER BY created_at, id`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*consolev1.FileComment{}
	for rows.Next() {
		c, err := scanFileComment(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (s *FileCommentService) Create(projectID string, req *consolev1.CreateFileCommentRequest) (*consolev1.FileComment, error) {
	text := strings.TrimSpace(req.Text)
	switch {
	case projectID == "":
		return nil, fmt.Errorf("projectId is required.")
	case req.Path == "":
		return nil, fmt.Errorf("path is required.")
	case text == "":
		return nil, fmt.Errorf("text is required.")
	case req.StartLine == 0 || req.EndLine < req.StartLine:
		return nil, fmt.Errorf("invalid line range.")
	}
	now := time.Now().UnixMilli()
	c := &consolev1.FileComment{
		Id:        uuid.NewString(),
		ProjectId: projectID,
		Path:      req.Path,
		Side:      req.Side,
		StartLine: req.StartLine,
		EndLine:   req.EndLine,
		Text:      text,
		Snippet:   req.Snippet,
		CreatedAt: now,
		UpdatedAt: now,
	}
	_, err := s.manager.Global().Exec(`INSERT INTO file_comments (`+diffCommentColumns+`)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		c.Id, c.ProjectId, c.Path, int64(c.Side), c.StartLine, c.EndLine,
		c.Text, c.Snippet, c.CreatedAt, c.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return c, nil
}

func (s *FileCommentService) UpdateText(id, text string) (*consolev1.FileComment, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil, fmt.Errorf("text is required.")
	}
	res, err := s.manager.Global().Exec(
		`UPDATE file_comments SET text = ?, updated_at = ? WHERE id = ?`,
		text, time.Now().UnixMilli(), id)
	if err != nil {
		return nil, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return nil, ErrFileCommentNotFound
	}
	c, err := scanFileComment(s.manager.Global().QueryRow(
		`SELECT `+diffCommentColumns+` FROM file_comments WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrFileCommentNotFound
	}
	return c, err
}

func (s *FileCommentService) Delete(id string) error {
	res, err := s.manager.Global().Exec(`DELETE FROM file_comments WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrFileCommentNotFound
	}
	return nil
}
