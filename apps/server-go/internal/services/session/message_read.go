// Reading messages back. Once a database has completed the stage-6 drop (its
// `content` column is gone), every message is rebuilt from its skeleton and
// parts: the rebuild is the source of truth. A row that cannot be rebuilt has
// no original to fall back on, so it is logged loudly and left out of the
// result rather than served wrong.
//
// While `content` is still present — the migration has not run yet, or its
// audit refused the drop — reads serve the original directly, which is the
// pre-split behavior.
package session

import (
	"database/sql"
	"encoding/json"
	"log/slog"
	"strings"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/msgparts"
)

// partsQueryChunk bounds how many message ids go into one IN (...) query.
const partsQueryChunk = 400

type storedRow struct {
	content   sql.NullString
	createdAt int64
	rowid     int64
	id        string
	skeleton  sql.NullString
	state     int
}

// hasContentColumn reports whether the messages table still has a `content`
// column, i.e. this database has not yet completed the drop of stage 6.
func hasContentColumn(conn *sql.DB) bool {
	var n int
	if err := conn.QueryRow(
		`SELECT COUNT(*) FROM pragma_table_info('messages') WHERE name = 'content'`,
	).Scan(&n); err != nil {
		// An unreadable pragma leaves the conservative answer: keep reading
		// and writing the original column if it is there.
		return true
	}
	return n > 0
}

// materialize returns the JSON of each row. With `content` still present it is
// served as stored; after the drop every row is rebuilt from its skeleton and
// parts. A row that cannot be rebuilt is returned empty, so callers leave that
// message out; each such row is logged.
func (s *Service) materialize(sessionID, projectID string, conn *sql.DB, rows []storedRow) []string {
	out := make([]string, len(rows))
	if hasContentColumn(conn) {
		for i, r := range rows {
			out[i] = r.content.String
		}
		return out
	}
	var need []string
	for _, r := range rows {
		if r.state == splitDone && r.skeleton.Valid {
			need = append(need, r.id)
		}
	}
	if len(need) == 0 {
		return out
	}
	parts, err := loadParts(conn, need)
	if err != nil {
		// Nothing can be rebuilt, so every message of this page is skipped.
		slog.Error("message parts could not be read; no message can be rebuilt",
			"session", sessionID, "messages", len(need), "error", err)
		return out
	}
	images := s.imagesFor(sessionID, projectID)
	for i, r := range rows {
		if r.state != splitDone || !r.skeleton.Valid {
			slog.Error("message has no split copy to rebuild; skipping it",
				"session", sessionID, "message", r.id, "state", r.state)
			continue
		}
		// Image bodies live in files; fetch them before rebuilding. A missing
		// or damaged file fails the rebuild below, which logs it.
		if err := images.hydrate(parts[r.id]); err != nil {
			slog.Error("message's image files could not be read; skipping it",
				"session", sessionID, "message", r.id, "error", err)
			continue
		}
		back, err := msgparts.Assemble([]byte(r.skeleton.String), parts[r.id])
		if err != nil {
			slog.Error("message could not be rebuilt from its parts; skipping it",
				"session", sessionID, "message", r.id, "error", err)
			continue
		}
		out[i] = string(back)
	}
	return out
}

// loadParts fetches the parts of the given messages, grouped by message and
// ordered by slot.
func loadParts(conn *sql.DB, ids []string) (map[string][]msgparts.Part, error) {
	out := make(map[string][]msgparts.Part, len(ids))
	for start := 0; start < len(ids); start += partsQueryChunk {
		end := start + partsQueryChunk
		if end > len(ids) {
			end = len(ids)
		}
		chunk := ids[start:end]
		args := make([]any, len(chunk))
		for i, id := range chunk {
			args[i] = id
		}
		rows, err := conn.Query(
			`SELECT message_id, slot, kind, meta, body FROM message_parts WHERE message_id IN (`+
				strings.TrimSuffix(strings.Repeat("?,", len(chunk)), ",")+") ORDER BY message_id, slot", args...)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var id, meta string
			var p msgparts.Part
			if err := rows.Scan(&id, &p.Slot, &p.Kind, &meta, &p.Body); err != nil {
				rows.Close()
				return nil, err
			}
			if err := json.Unmarshal([]byte(meta), &p.Meta); err != nil {
				rows.Close()
				return nil, err
			}
			out[id] = append(out[id], p)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return nil, err
		}
	}
	return out, nil
}
