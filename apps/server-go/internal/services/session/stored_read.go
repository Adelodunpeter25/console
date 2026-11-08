// Reading messages in split format for fast direct replay in agent runs.
package session

import (
	"database/sql"
	"log/slog"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/msgparts"
)

// StoredSplit holds a stored message's skeleton and raw parts for direct decoding
// without reassembling the monolithic JSON document.
type StoredSplit struct {
	ID        string
	Role      string
	CreatedAt int64
	RowID     int64
	Skeleton  string
	State     int
	Content   string // present only before stage-6 migration drops it
	Parts     []msgparts.Part
}

// LoadSplit reads messages in chronological order (oldest first) with their parts
// hydrated from the database and image store, bypassing JSON reconstruction.
func (s *Service) LoadSplit(sessionID string) ([]StoredSplit, error) {
	rows, err := s.manager.Global().Query(`
		SELECT id, project_id, deleted_at FROM sessions WHERE id = ?`, sessionID)
	if err != nil {
		return nil, err
	}
	var id string
	var projID sql.NullString
	var deletedAt sql.NullInt64
	found := false
	if rows.Next() {
		if err := rows.Scan(&id, &projID, &deletedAt); err != nil {
			rows.Close()
			return nil, err
		}
		found = true
	}
	rows.Close()
	if !found || deletedAt.Valid {
		return nil, nil
	}
	projectID := ""
	if projID.Valid {
		projectID = projID.String
	}

	conn, err := s.manager.Session(sessionID, projectID)
	if err != nil {
		return nil, err
	}
	s.ensureContentDropped(sessionID, projectID, conn)

	withContent := hasContentColumn(conn)
	contentSel := "''"
	if withContent {
		contentSel = "content"
	}

	// Read chronological order (oldest first) for history replay.
	query := `SELECT ` + contentSel + `, created_at, rowid, id, role, COALESCE(skeleton, ''), split_state FROM messages ORDER BY rowid ASC`
	msgRows, err := conn.Query(query)
	if err != nil {
		return nil, err
	}

	type rawRow struct {
		content   sql.NullString
		createdAt int64
		rowid     int64
		id        string
		role      string
		skeleton  string
		state     int
	}

	var fetched []rawRow
	var needParts []string
	for msgRows.Next() {
		var r rawRow
		if err := msgRows.Scan(&r.content, &r.createdAt, &r.rowid, &r.id, &r.role, &r.skeleton, &r.state); err != nil {
			msgRows.Close()
			return nil, err
		}
		fetched = append(fetched, r)
		if !withContent && r.state == splitDone && r.skeleton != "" {
			needParts = append(needParts, r.id)
		}
	}
	err = msgRows.Err()
	// Close explicitly before querying message_parts on the single connection.
	msgRows.Close()
	if err != nil {
		return nil, err
	}

	var partsByID map[string][]msgparts.Part
	if len(needParts) > 0 {
		var err error
		partsByID, err = loadParts(conn, needParts)
		if err != nil {
			slog.Error("load split: message parts could not be read", "session", sessionID, "error", err)
			return nil, err
		}
	}

	images := s.imagesFor(sessionID, projectID)
	out := make([]StoredSplit, len(fetched))
	for i, r := range fetched {
		pList := partsByID[r.id]
		if len(pList) > 0 {
			if err := images.hydrate(pList); err != nil {
				slog.Error("load split: hydrating image files failed", "session", sessionID, "message", r.id, "error", err)
			}
		}
		out[i] = StoredSplit{
			ID:        r.id,
			Role:      r.role,
			CreatedAt: r.createdAt,
			RowID:     r.rowid,
			Skeleton:  r.skeleton,
			State:     r.state,
			Content:   r.content.String,
			Parts:     pList,
		}
	}
	return out, nil
}
