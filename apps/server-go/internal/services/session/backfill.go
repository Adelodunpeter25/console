// Backfill: messages written before the split storage existed have split_state
// 0 and no skeleton or parts. They are converted one session at a time, the
// first time that session is read, in batches of whole transactions.
//
// It is safe to interrupt at any point: a batch either commits entirely or not
// at all, `content` is never modified, and anything left over is simply picked
// up by the next call. A message that cannot be split is marked so it is not
// retried forever, and reads keep using its original content.
package session

import (
	"database/sql"
	"log/slog"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/msgparts"
)

// backfillBatch bounds one transaction, so a huge history never holds the
// single database connection for long.
const backfillBatch = 200

// BackfillStats reports what one backfill pass did.
type BackfillStats struct {
	Split        int // rows converted
	Unsplittable int // rows marked as unsplittable
	Remaining    int // rows still pending after the pass
}

// backfillSession converts every pending message of one session. It is a no-op
// when nothing is pending, which is the common case after the first read.
func backfillSession(conn *sql.DB) (BackfillStats, error) {
	var stats BackfillStats
	for {
		n, bad, err := backfillBatchOnce(conn)
		stats.Split += n
		stats.Unsplittable += bad
		if err != nil {
			return stats, err
		}
		if n+bad == 0 {
			break
		}
	}
	if err := conn.QueryRow(`SELECT COUNT(*) FROM messages WHERE split_state = ?`, splitPending).Scan(&stats.Remaining); err != nil {
		return stats, err
	}
	return stats, nil
}

type pendingRow struct {
	id      string
	content string
}

func backfillBatchOnce(conn *sql.DB) (split, unsplittable int, err error) {
	rows, err := conn.Query(
		`SELECT id, content FROM messages WHERE split_state = ? ORDER BY created_at, rowid LIMIT ?`,
		splitPending, backfillBatch)
	if err != nil {
		return 0, 0, err
	}
	var batch []pendingRow
	for rows.Next() {
		var r pendingRow
		if err := rows.Scan(&r.id, &r.content); err != nil {
			rows.Close()
			return 0, 0, err
		}
		batch = append(batch, r)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return 0, 0, err
	}
	rows.Close()
	if len(batch) == 0 {
		return 0, 0, nil
	}

	tx, err := conn.Begin()
	if err != nil {
		return 0, 0, err
	}
	for _, r := range batch {
		res, serr := msgparts.Split([]byte(r.content))
		if serr == nil {
			// Never trust the split until it provably rebuilds the original.
			back, aerr := msgparts.Assemble(res.Skeleton, res.Parts)
			if aerr != nil {
				serr = aerr
			} else if same, eqErr := msgparts.Equal([]byte(r.content), back); eqErr != nil || !same {
				serr = errRebuildDiffers
			}
		}
		if serr != nil {
			slog.Warn("backfill: message left as-is", "message", r.id, "error", serr)
			if _, err := tx.Exec(`UPDATE messages SET split_state = ? WHERE id = ?`, splitUnsplittable, r.id); err != nil {
				tx.Rollback()
				return 0, 0, err
			}
			unsplittable++
			continue
		}
		if _, err := tx.Exec(`DELETE FROM message_parts WHERE message_id = ?`, r.id); err != nil {
			tx.Rollback()
			return 0, 0, err
		}
		if err := insertParts(tx, r.id, res.Parts); err != nil {
			tx.Rollback()
			return 0, 0, err
		}
		if _, err := tx.Exec(
			`UPDATE messages SET skeleton = ?, text = ?, split_state = ? WHERE id = ?`,
			string(res.Skeleton), res.Text, splitDone, r.id); err != nil {
			tx.Rollback()
			return 0, 0, err
		}
		split++
	}
	if err := tx.Commit(); err != nil {
		return 0, 0, err
	}
	return split, unsplittable, nil
}

type backfillError string

func (e backfillError) Error() string { return string(e) }

const errRebuildDiffers = backfillError("the split did not rebuild to the original message")

// ensureBackfilled converts a session's old messages the first time the session
// is loaded in this process. A failure is logged and not fatal: the original
// content is complete, so the session keeps working and the next load retries.
func (s *Service) ensureBackfilled(sessionID string, conn *sql.DB) {
	if _, done := s.backfilled.Load(sessionID); done {
		return
	}
	stats, err := backfillSession(conn)
	if err != nil {
		slog.Warn("backfill: will retry on the next load", "session", sessionID, "error", err)
		return
	}
	if stats.Remaining == 0 {
		s.backfilled.Store(sessionID, struct{}{})
	}
	if stats.Split+stats.Unsplittable > 0 {
		slog.Info("backfill: converted old messages", "session", sessionID,
			"split", stats.Split, "left_as_is", stats.Unsplittable)
	}
}

// BackfillAll converts every pending message of one session database and returns
// how many were left as-is. Exported for tests that run it on a database copy.
func BackfillAll(conn *sql.DB) (leftAsIs int, err error) {
	stats, err := backfillSession(conn)
	return stats.Unsplittable, err
}
