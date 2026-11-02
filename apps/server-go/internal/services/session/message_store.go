// Writing a message: the original JSON stays in `content` (authoritative during
// the transition) and is also split into a skeleton, searchable text and raw
// part bodies. Every path that stores a message goes through insertMessage, so
// the two copies cannot drift apart.
package session

import (
	"database/sql"
	"encoding/json"
	"log/slog"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/msgparts"
)

const (
	splitPending      = 0
	splitDone         = 1
	splitUnsplittable = 2
)

// insertMessage stores one message. With ignore it behaves like INSERT OR
// IGNORE and reports whether a row was actually added. A message that cannot be
// split is still stored (its content is complete); it is marked so reads use the
// original JSON, and the failure is logged rather than failing the write.
func insertMessage(tx *sql.Tx, ignore bool, id, role, content string, createdAt int64) (bool, error) {
	var (
		skeleton any
		text     any
		state    = splitDone
		parts    []msgparts.Part
	)
	res, err := msgparts.Split([]byte(content))
	if err != nil {
		slog.Warn("message could not be split; stored as-is", "message", id, "error", err)
		state = splitUnsplittable
	} else {
		skeleton, text, parts = string(res.Skeleton), res.Text, res.Parts
	}

	verb := "INSERT"
	if ignore {
		verb = "INSERT OR IGNORE"
	}
	r, err := tx.Exec(
		verb+` INTO messages (id, role, content, created_at, skeleton, text, split_state) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		id, role, content, createdAt, skeleton, text, state)
	if err != nil {
		return false, err
	}
	if n, _ := r.RowsAffected(); n == 0 {
		return false, nil
	}
	if err := insertParts(tx, id, parts); err != nil {
		return false, err
	}
	return true, nil
}

// insertParts stores a message's extracted bodies.
func insertParts(tx *sql.Tx, id string, parts []msgparts.Part) error {
	for _, p := range parts {
		meta, err := json.Marshal(p.Meta)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(
			`INSERT INTO message_parts (message_id, slot, kind, meta, body) VALUES (?, ?, ?, ?, ?)`,
			id, p.Slot, p.Kind, string(meta), p.Body); err != nil {
			return err
		}
	}
	return nil
}
