// Stage 6 of the split-message storage: dropping the original `content`
// column. This is the only hard-to-undo step, so it runs as a deliberate,
// audited migration, one session database at a time, the first time that
// session is opened:
//
//  1. any messages still waiting to be split are backfilled (`content` is
//     still there and is still the source of truth);
//  2. unsplittable rows are folded: their whole JSON becomes their skeleton,
//     with no parts to refill;
//  3. every message is rebuilt from its skeleton and parts and compared with
//     its original, the same equality the transition served reads under;
//  4. only if every message rebuilt identically is the column dropped.
//
// If the audit finds anything, the column stays and the session keeps working
// the old way (reads and writes on `content`), retrying on the next open.
// After the drop, older builds of the server cannot read the session database;
// that downgrade is a restore, not a downgrade.
package session

import (
	"database/sql"
	"log/slog"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/msgparts"
)

// auditBatch bounds one page of the audit, so a huge history never holds the
// single database connection for long.
const auditBatch = 500

// ensureContentDropped migrates one session database to post-`content`
// storage. It runs once per session per process; a failure is logged and not
// fatal, and the next open retries.
func (s *Service) ensureContentDropped(sessionID, projectID string, conn *sql.DB) {
	if _, done := s.migrated.Load(sessionID); done {
		return
	}
	if !hasContentColumn(conn) {
		s.migrated.Store(sessionID, struct{}{})
		return
	}
	images := s.imagesFor(sessionID, projectID)
	stats, err := backfillSession(conn, images)
	if err != nil {
		slog.Warn("content drop: backfill failed, will retry on the next open",
			"session", sessionID, "error", err)
		return
	}
	// Unsplittable rows kept their whole JSON in `content`; from now on that
	// JSON is their skeleton, with no parts to refill.
	if _, err := conn.Exec(
		`UPDATE messages SET skeleton = content, split_state = ? WHERE split_state = ?`,
		splitDone, splitUnsplittable); err != nil {
		slog.Warn("content drop: folding unsplittable rows failed, will retry on the next open",
			"session", sessionID, "error", err)
		return
	}
	checked, mismatched, err := auditContentParity(conn, images)
	if err != nil {
		slog.Warn("content drop: audit failed, will retry on the next open",
			"session", sessionID, "error", err)
		return
	}
	if mismatched > 0 {
		slog.Error("content drop: messages do not rebuild from their parts; keeping the content column and retrying on the next open",
			"session", sessionID, "checked", checked, "mismatched", mismatched)
		return
	}
	if _, err := conn.Exec(`DROP INDEX IF EXISTS idx_messages_unsplit`); err != nil {
		slog.Error("content drop: dropping the backlog index failed", "session", sessionID, "error", err)
		return
	}
	if _, err := conn.Exec(`ALTER TABLE messages DROP COLUMN content`); err != nil {
		slog.Error("content drop: dropping the column failed", "session", sessionID, "error", err)
		return
	}
	slog.Info("content column dropped",
		"session", sessionID, "backfilled", stats.Split, "audited", checked)
	s.migrated.Store(sessionID, struct{}{})
}

// auditContentParity rebuilds every message from its skeleton and parts and
// compares it with the original `content`. A row whose skeleton is exactly its
// content and that has no parts (folded unsplittable rows) passes by direct
// comparison: there is nothing to rebuild.
func auditContentParity(conn *sql.DB, images *imageStore) (checked, mismatched int, err error) {
	var cursor int64
	for {
		rows, err := conn.Query(
			`SELECT rowid, id, content, COALESCE(skeleton, ''), split_state FROM messages
			 WHERE rowid > ? ORDER BY rowid LIMIT ?`, cursor, auditBatch)
		if err != nil {
			return checked, mismatched, err
		}
		type auditRow struct {
			rowid    int64
			id       string
			content  string
			skeleton string
			state    int
		}
		var batch []auditRow
		for rows.Next() {
			var r auditRow
			if err := rows.Scan(&r.rowid, &r.id, &r.content, &r.skeleton, &r.state); err != nil {
				rows.Close()
				return checked, mismatched, err
			}
			batch = append(batch, r)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return checked, mismatched, err
		}
		if len(batch) == 0 {
			return checked, mismatched, nil
		}
		cursor = batch[len(batch)-1].rowid

		var need []string
		for _, r := range batch {
			if r.state == splitDone && r.skeleton != r.content {
				need = append(need, r.id)
			}
		}
		parts, err := loadParts(conn, need)
		if err != nil {
			return checked, mismatched, err
		}
		for _, r := range batch {
			checked++
			if r.state != splitDone {
				// The backfill ran to completion above, so nothing should be
				// left pending; refuse the drop rather than trust it.
				mismatched++
				continue
			}
			if r.skeleton == r.content && len(parts[r.id]) == 0 {
				continue
			}
			if err := images.hydrate(parts[r.id]); err != nil {
				mismatched++
				continue
			}
			back, aerr := msgparts.Assemble([]byte(r.skeleton), parts[r.id])
			if aerr == nil {
				var same bool
				if same, aerr = msgparts.Equal([]byte(r.content), back); aerr == nil && same {
					continue
				}
			}
			mismatched++
		}
	}
}
