// Stage 3: messages written before the split storage existed are converted the
// first time their session is read. The conversion must never lose or alter a
// message, must survive being interrupted, and must be safe to run again.
package tests

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/db"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/services"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/services/session"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/types"
)

// legacyRows writes messages the way the old code did: content only, no split
// columns populated, so they are exactly what an existing database holds.
func legacyRows(t *testing.T, c *sql.DB, msgs []types.AgentMessage) {
	t.Helper()
	for i, m := range msgs {
		if _, err := c.Exec(
			`INSERT INTO messages (id, role, content, created_at) VALUES (?, ?, ?, ?)`,
			m.ID, m.Role, string(m.Data), int64(1000+i)); err != nil {
			t.Fatal(err)
		}
	}
}

func contents(t *testing.T, c *sql.DB) map[string]string {
	t.Helper()
	out := map[string]string{}
	rows, err := c.Query(`SELECT id, content FROM messages`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var id, content string
		_ = rows.Scan(&id, &content)
		out[id] = content
	}
	return out
}

func TestBackfillConvertsOldMessagesOnFirstLoadAndNeverChangesContent(t *testing.T) {
	s, m := partsStore(t)
	h := partsSession(t, s)
	c := sessionConn(t, m, h.ID)
	legacyRows(t, c, toolTurn())
	before := contents(t, c)

	var pending int
	_ = c.QueryRow(`SELECT COUNT(*) FROM messages WHERE split_state = 0`).Scan(&pending)
	if pending != 3 {
		t.Fatalf("setup: %d pending, want 3", pending)
	}

	loaded, err := s.Load(h.ID, 0, 0)
	if err != nil || loaded == nil || len(loaded.Messages) != 3 {
		t.Fatalf("load: %v %v", loaded, err)
	}

	_ = c.QueryRow(`SELECT COUNT(*) FROM messages WHERE split_state = 0`).Scan(&pending)
	if pending != 0 {
		t.Fatalf("%d messages still pending after the first load", pending)
	}
	for id, want := range before {
		if got := contents(t, c)[id]; got != want {
			t.Fatalf("%s: content was modified by the backfill:\n before %q\n after  %q", id, want, got)
		}
		verifySplit(t, c, id)
	}
	var parts int
	_ = c.QueryRow(`SELECT COUNT(*) FROM message_parts`).Scan(&parts)
	if parts != 2 {
		t.Fatalf("parts = %d, want 2", parts)
	}
}

func TestBackfillIsRepeatableAndDoesNotDuplicate(t *testing.T) {
	s, m := partsStore(t)
	h := partsSession(t, s)
	c := sessionConn(t, m, h.ID)
	legacyRows(t, c, toolTurn())
	for i := 0; i < 4; i++ {
		if _, err := s.Load(h.ID, 0, 0); err != nil {
			t.Fatal(err)
		}
	}
	var parts, msgs int
	_ = c.QueryRow(`SELECT COUNT(*) FROM message_parts`).Scan(&parts)
	_ = c.QueryRow(`SELECT COUNT(*) FROM messages`).Scan(&msgs)
	if parts != 2 || msgs != 3 {
		t.Fatalf("parts=%d messages=%d after repeated loads, want 2 and 3", parts, msgs)
	}
}

func TestBackfillLeavesUnsplittableRowsAloneAndDoesNotRetryThem(t *testing.T) {
	s, m := partsStore(t)
	h := partsSession(t, s)
	c := sessionConn(t, m, h.ID)
	legacyRows(t, c, []types.AgentMessage{
		{ID: "good", Role: "user", Data: []byte(`{"user":{"content":"fine"}}`)},
		{ID: "junk", Role: "user", Data: []byte(`this is not json`)},
		{ID: "arr", Role: "user", Data: []byte(`[1,2,3]`)},
	})
	loaded, err := s.Load(h.ID, 0, 0)
	if err != nil {
		t.Fatalf("a bad row must not break the load: %v", err)
	}
	if len(loaded.Messages) != 3 {
		t.Fatalf("every message must still be returned, got %d", len(loaded.Messages))
	}
	states := map[string]int{}
	rows, _ := c.Query(`SELECT id, split_state FROM messages`)
	for rows.Next() {
		var id string
		var st int
		_ = rows.Scan(&id, &st)
		states[id] = st
	}
	rows.Close()
	if states["good"] != 1 || states["junk"] != 2 || states["arr"] != 2 {
		t.Fatalf("states = %v, want good=1 junk=2 arr=2", states)
	}
	if got := contents(t, c)["junk"]; got != "this is not json" {
		t.Fatalf("an unsplittable row's content changed: %q", got)
	}
}

// More rows than one batch, so the loop must run several transactions.
func TestBackfillHandlesManyRowsAcrossBatches(t *testing.T) {
	s, m := partsStore(t)
	h := partsSession(t, s)
	c := sessionConn(t, m, h.ID)
	var msgs []types.AgentMessage
	for i := 0; i < 650; i++ {
		msgs = append(msgs, types.AgentMessage{
			ID: fmt.Sprintf("r%04d", i), Role: "toolResult",
			Data: []byte(`{"toolResult":{"results":[{"toolCallId":"c","content":"` + enc(fmt.Sprintf("output number %d", i)) + `"}]}}`),
		})
	}
	legacyRows(t, c, msgs)
	if _, err := s.Load(h.ID, 0, 0); err != nil {
		t.Fatal(err)
	}
	var pending, parts int
	_ = c.QueryRow(`SELECT COUNT(*) FROM messages WHERE split_state = 0`).Scan(&pending)
	_ = c.QueryRow(`SELECT COUNT(*) FROM message_parts`).Scan(&parts)
	if pending != 0 || parts != 650 {
		t.Fatalf("pending=%d parts=%d, want 0 and 650", pending, parts)
	}
	verifySplit(t, c, "r0000")
	verifySplit(t, c, "r0649")
}

// A half-finished backfill (some rows done, some not) just continues.
func TestBackfillResumesAfterAnInterruption(t *testing.T) {
	s, m := partsStore(t)
	h := partsSession(t, s)
	c := sessionConn(t, m, h.ID)
	legacyRows(t, c, toolTurn())
	// Simulate an earlier run that converted only the first message.
	first := toolTurn()[0]
	if _, err := c.Exec(`UPDATE messages SET skeleton = ?, text = ?, split_state = 1 WHERE id = ?`,
		string(first.Data), "fix the login bug", first.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Load(h.ID, 0, 0); err != nil {
		t.Fatal(err)
	}
	var pending int
	_ = c.QueryRow(`SELECT COUNT(*) FROM messages WHERE split_state = 0`).Scan(&pending)
	if pending != 0 {
		t.Fatalf("%d rows still pending after resuming", pending)
	}
	for _, id := range []string{"u1", "a1", "r1"} {
		verifySplit(t, c, id)
	}
}

// Messages added after a backfill, and messages added during one, all end up split.
func TestNewMessagesAfterBackfillAreSplit(t *testing.T) {
	s, m := partsStore(t)
	h := partsSession(t, s)
	c := sessionConn(t, m, h.ID)
	legacyRows(t, c, toolTurn()[:1])
	if _, err := s.Load(h.ID, 0, 0); err != nil {
		t.Fatal(err)
	}
	if err := s.AppendMessages(h.ID, toolTurn()[1:]); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"u1", "a1", "r1"} {
		verifySplit(t, c, id)
	}
}

// The real test: copy the developer's actual databases, backfill the copies, and
// require every message's content to be byte for byte unchanged and every split
// to rebuild. Originals are never opened for writing.
func TestBackfillOverCopiesOfRealDatabases(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("no home directory")
	}
	store := filepath.Join(home, ".console")
	var srcs []string
	_ = filepath.Walk(store, func(p string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() && strings.HasSuffix(p, ".db") && strings.Contains(p, string(filepath.Separator)+"sessions"+string(filepath.Separator)) {
			srcs = append(srcs, p)
		}
		return nil
	})
	if len(srcs) == 0 {
		t.Skip("no real session databases")
	}
	checked, rowsTotal, left := 0, 0, 0
	for _, src := range srcs {
		probe, err := sql.Open("sqlite", "file:"+src+"?mode=ro&immutable=1")
		if err != nil {
			continue
		}
		var n int
		if probe.QueryRow(`SELECT COUNT(*) FROM messages`).Scan(&n) != nil {
			probe.Close()
			continue // not a session database, or unreadable
		}
		probe.Close()

		copyPath := filepath.Join(t.TempDir(), "copy.db")
		if err := copyFile(src, copyPath); err != nil {
			continue
		}
		c, err := sql.Open("sqlite", "file:"+copyPath)
		if err != nil {
			continue
		}
		if err := db.InitSessionDB(c, copyPath); err != nil {
			c.Close()
			t.Errorf("%s: upgrade failed: %v", filepath.Base(src), err)
			continue
		}
		before := contents(t, c)
		stats, err := session.BackfillAll(c)
		if err != nil {
			t.Errorf("%s: backfill: %v", filepath.Base(src), err)
			c.Close()
			continue
		}
		after := contents(t, c)
		for id, want := range before {
			if after[id] != want {
				t.Errorf("%s: message %s content changed", filepath.Base(src), id)
			}
		}
		rows, _ := c.Query(`SELECT id FROM messages WHERE split_state = 1`)
		for rows.Next() {
			var id string
			_ = rows.Scan(&id)
			verifySplit(t, c, id)
		}
		rows.Close()
		checked++
		rowsTotal += len(before)
		left += stats
		c.Close()
	}
	if checked == 0 {
		t.Skip("no readable databases")
	}
	t.Logf("backfilled copies of %d real databases: %d messages, %d left as-is", checked, rowsTotal, left)
}

func copyFile(src, dst string) error {
	b, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, b, 0o644)
}

var _ = services.NewSessionService
