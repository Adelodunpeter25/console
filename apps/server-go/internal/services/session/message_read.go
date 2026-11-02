// Reading messages back: split rows are rebuilt from their skeleton and parts,
// and every rebuild is checked against the original `content` before it is
// served. If the two ever differ the original is served and the difference is
// logged and counted, so a bug in the rebuild can degrade to "slower than
// planned" but never to "history that is wrong".
//
// While `content` is kept (the transition), a message is verified the first
// time it is read in this process. Once verified, it is identical to `content`
// by definition, so later reads serve `content` without repeating the work.
package session

import (
	"database/sql"
	"encoding/json"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/msgparts"
)

// partsQueryChunk bounds how many message ids go into one IN (...) query.
const partsQueryChunk = 400

type storedRow struct {
	content   string
	createdAt int64
	rowid     int64
	id        string
	skeleton  sql.NullString
	state     int
}

var (
	parityChecked    atomic.Int64
	parityMismatched atomic.Int64
)

// ParityStats reports how many messages have been rebuilt and compared against
// their original, and how many of those differed. The second number should
// stay zero; a nonzero value means the rebuild has a bug worth finding before
// the original column is ever dropped.
func ParityStats() (checked, mismatched int64) {
	return parityChecked.Load(), parityMismatched.Load()
}

// verifiedSet remembers, per session, which messages already passed the check.
type verifiedSet struct {
	mu   sync.Mutex
	sess map[string]map[string]struct{}
}

func (v *verifiedSet) has(session, id string) bool {
	v.mu.Lock()
	defer v.mu.Unlock()
	_, ok := v.sess[session][id]
	return ok
}

func (v *verifiedSet) add(session, id string) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.sess == nil {
		v.sess = map[string]map[string]struct{}{}
	}
	if v.sess[session] == nil {
		v.sess[session] = map[string]struct{}{}
	}
	v.sess[session][id] = struct{}{}
}

// forget drops a session's record, so its messages are verified again. Called
// whenever the session's messages are rewritten.
func (v *verifiedSet) forget(session string) {
	v.mu.Lock()
	defer v.mu.Unlock()
	delete(v.sess, session)
}

// materialize returns the JSON of each row, rebuilt from parts where that is
// possible and proven, and the original content otherwise.
func (s *Service) materialize(sessionID string, conn *sql.DB, rows []storedRow) []string {
	out := make([]string, len(rows))
	var need []string // split rows not yet verified in this process
	for i, r := range rows {
		out[i] = r.content
		if r.state == splitDone && r.skeleton.Valid && !s.verified.has(sessionID, r.id) {
			need = append(need, r.id)
		}
	}
	if len(need) == 0 {
		return out
	}
	parts, err := loadParts(conn, need)
	if err != nil {
		// The original is complete, so serve it and try again next time. This is
		// still a failure to rebuild, so it is counted: a counter that only saw
		// clean mismatches would hide unreadable parts.
		parityChecked.Add(int64(len(need)))
		parityMismatched.Add(int64(len(need)))
		slog.Error("message parts could not be read; serving original content",
			"session", sessionID, "messages", len(need), "error", err)
		return out
	}
	for i, r := range rows {
		if r.state != splitDone || !r.skeleton.Valid || s.verified.has(sessionID, r.id) {
			continue
		}
		parityChecked.Add(1)
		back, err := msgparts.Assemble([]byte(r.skeleton.String), parts[r.id])
		if err == nil {
			var same bool
			if same, err = msgparts.Equal([]byte(r.content), back); err == nil && same {
				out[i] = string(back)
				s.verified.add(sessionID, r.id)
				continue
			}
		}
		parityMismatched.Add(1)
		slog.Error("message rebuilt from parts differs from its original; serving the original",
			"session", sessionID, "message", r.id, "error", err)
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
				strings.TrimSuffix(strings.Repeat("?,", len(chunk)), ",")+`) ORDER BY message_id, slot`, args...)
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
