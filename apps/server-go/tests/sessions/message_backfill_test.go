// Stage 3: messages written before the split storage existed are converted the
// first time their session is read. The conversion must never lose or alter a
// message, must survive being interrupted, and must be safe to run again.
package tests

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/msgparts"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/services"
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

func TestBackfillConvertsOldMessagesOnFirstLoad(t *testing.T) {
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
	// Every message must come back exactly as it was stored, and while the
	// `content` column is still there, it must be untouched.
	for _, raw := range loaded.Messages {
		var m map[string]any
		if err := json.Unmarshal(raw, &m); err != nil {
			t.Fatal(err)
		}
		id, _ := m["id"].(string)
		if id == "" {
			t.Fatal("a loaded message has no id")
		}
		delete(m, "id")
		delete(m, "createdAt")
		served, _ := json.Marshal(m)
		if same, _ := msgparts.Equal([]byte(before[id]), served); !same {
			t.Fatalf("%s: served a different message than was stored:\n before %q\n after  %q", id, before[id], served)
		}
	}
	if contentKept(t, c) {
		for id, want := range before {
			if got := contents(t, c)[id]; got != want {
				t.Fatalf("%s: content was modified by the backfill:\n before %q\n after  %q", id, want, got)
			}
		}
	}
	for id := range before {
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

// Rows nothing can be split out of (not the expected JSON, say) are stored
// whole: their JSON becomes their own skeleton. They must not break the load;
// a row that cannot even be re-issued as JSON is skipped on read and logged.
func TestUnsplittableRowsDoNotBreakTheLoad(t *testing.T) {
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
	ids := map[string]bool{}
	servedArr := false
	for _, raw := range loaded.Messages {
		var m map[string]any
		_ = json.Unmarshal(raw, &m)
		if id, _ := m["id"].(string); id != "" {
			ids[id] = true
		}
		if strings.Trim(string(raw), " ") == "[1,2,3]" {
			servedArr = true
		}
	}
	// "junk" is not JSON at all, so it cannot be rebuilt and is left out.
	if len(loaded.Messages) != 2 || !ids["good"] || !servedArr || ids["junk"] {
		t.Fatalf("want good and arr served and junk skipped, got %v (arr=%v)", ids, servedArr)
	}
	for _, id := range []string{"good", "arr", "junk"} {
		var skeleton string
		var state int
		if err := c.QueryRow(`SELECT COALESCE(skeleton,''), split_state FROM messages WHERE id = ?`, id).Scan(&skeleton, &state); err != nil {
			t.Fatal(err)
		}
		if state != 1 {
			t.Fatalf("%s: split_state = %d, want 1", id, state)
		}
	}
	var junkSkeleton string
	_ = c.QueryRow(`SELECT COALESCE(skeleton,'') FROM messages WHERE id = 'junk'`).Scan(&junkSkeleton)
	if junkSkeleton != "this is not json" {
		t.Fatalf("an unsplittable row must keep its whole JSON as the skeleton: %q", junkSkeleton)
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

func copyFile(src, dst string) error {
	b, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, b, 0o644)
}

var _ = services.NewSessionService
