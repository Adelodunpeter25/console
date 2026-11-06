// Writing a message: the message is split into a skeleton, searchable text and
// raw part bodies. While the database still has a `content` column (before the
// stage-6 drop), the original JSON is written there too; every path that stores
// a message goes through insertMessage, so the copies cannot drift apart.
package session

import (
	"database/sql"
	"encoding/json"
	"log/slog"
	"strings"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/msgparts"
)

const (
	splitPending      = 0
	splitDone         = 1
	splitUnsplittable = 2
)

// insertMessage stores one message. Image bodies go to the session's image
// store (images may be nil, which keeps them in the database). With ignore it
// behaves like INSERT OR IGNORE and reports whether a row was actually added.
// withContent writes the original JSON to the `content` column and is only
// true while that column still exists. A message that cannot be split is
// stored whole: its JSON becomes its own skeleton, with no parts to refill.
func insertMessage(tx *sql.Tx, images *imageStore, ignore, withContent bool, id, role, content string, createdAt int64) (bool, error) {
	var (
		skeleton any
		text     any
		state    = splitDone
		parts    []msgparts.Part
	)
	res, err := msgparts.Split([]byte(content))
	if err != nil {
		slog.Warn("message stored whole: nothing could be split out of it", "message", id, "error", err)
		skeleton, text, parts = content, "", nil
	} else {
		skeleton, text, parts = string(res.Skeleton), res.Text, images.externalize(res.Parts)
	}

	verb := "INSERT"
	if ignore {
		verb = "INSERT OR IGNORE"
	}
	args := []any{id, role, createdAt, skeleton, text, state}
	cols := "(id, role, created_at, skeleton, text, split_state)"
	if withContent {
		args = []any{id, role, content, createdAt, skeleton, text, state}
		cols = "(id, role, content, created_at, skeleton, text, split_state)"
	}
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(args)), ",")
	r, err := tx.Exec(verb+" INTO messages "+cols+" VALUES ("+placeholders+")", args...)
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
