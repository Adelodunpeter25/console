// Stage 2 of the split-message storage: every write keeps the original JSON in
// `content` and also stores the skeleton, searchable text and raw bodies, and
// the two always agree.
package tests

import (
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/db"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/msgparts"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/services"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/types"
)

func partsStore(t *testing.T) (*services.SessionService, *db.DB) {
	t.Helper()
	manager, err := db.Open(db.OpenOptions{Path: ":memory:"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(manager.Close)
	return services.NewSessionService(manager), manager
}

func partsSession(t *testing.T, s *services.SessionService) types.SessionHeader {
	t.Helper()
	h, err := s.Create(types.CreateSessionOptions{Title: "t", Cwd: t.TempDir(), ModelID: "m", Provider: "p"})
	if err != nil {
		t.Fatal(err)
	}
	return h
}

func enc(s string) string { return base64.StdEncoding.EncodeToString([]byte(s)) }

func toolTurn() []types.AgentMessage {
	return []types.AgentMessage{
		{ID: "u1", Role: "user", Data: []byte(`{"user":{"content":"fix the login bug"}}`)},
		{ID: "a1", Role: "assistant", Data: []byte(`{"assistant":{"id":"a1","content":[{"text":{"text":"Looking at it."}},{"toolCall":{"id":"c1","name":"read_file","arguments":"` + enc(`{"path":"auth.go"}`) + `"}}],"stopReason":"toolUse"}}`)},
		{ID: "r1", Role: "toolResult", Data: []byte(`{"toolResult":{"results":[{"toolCallId":"c1","toolName":"read_file","content":"` + enc(`[{"type":"text","text":"package auth"}]`) + `"}]}}`)},
	}
}

func sessionConn(t *testing.T, m *db.DB, id string) *sql.DB {
	t.Helper()
	c, err := m.Session(id, "")
	if err != nil {
		t.Fatal(err)
	}
	return c
}

// contentKept reports whether the test database still has a `content` column,
// i.e. the stage-6 drop has not happened for it.
func contentKept(t *testing.T, c *sql.DB) bool {
	t.Helper()
	var n int
	if err := c.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('messages') WHERE name = 'content'`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n > 0
}

// verifySplit checks one stored row end to end: parts present, and the skeleton
// plus parts rebuild exactly. While `content` is still there it must equal the
// rebuild; after the drop there is no original left to compare with.
func verifySplit(t *testing.T, c *sql.DB, id string) {
	t.Helper()
	var skeleton string
	var state int
	if err := c.QueryRow(`SELECT COALESCE(skeleton,''), split_state FROM messages WHERE id = ?`, id).Scan(&skeleton, &state); err != nil {
		t.Fatalf("%s: %v", id, err)
	}
	if state != 1 {
		t.Fatalf("%s: split_state = %d, want 1 (split)", id, state)
	}
	rows, err := c.Query(`SELECT slot, kind, meta, body FROM message_parts WHERE message_id = ? ORDER BY slot`, id)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var parts []msgparts.Part
	for rows.Next() {
		var p msgparts.Part
		var meta string
		if err := rows.Scan(&p.Slot, &p.Kind, &meta, &p.Body); err != nil {
			t.Fatal(err)
		}
		if err := jsonUnmarshal(meta, &p.Meta); err != nil {
			t.Fatal(err)
		}
		parts = append(parts, p)
	}
	back, err := msgparts.Assemble([]byte(skeleton), parts)
	if err != nil {
		t.Fatalf("%s: assemble: %v", id, err)
	}
	if contentKept(t, c) {
		var content string
		if err := c.QueryRow(`SELECT content FROM messages WHERE id = ?`, id).Scan(&content); err != nil {
			t.Fatal(err)
		}
		if same, _ := msgparts.Equal([]byte(content), back); !same {
			t.Fatalf("%s: the split copy no longer matches content\n content:  %.200s\n rebuilt:  %.200s", id, content, back)
		}
	}
}

func TestAppendKeepsContentAndStoresTheSplitCopy(t *testing.T) {
	s, m := partsStore(t)
	h := partsSession(t, s)
	if err := s.AppendMessages(h.ID, toolTurn()); err != nil {
		t.Fatal(err)
	}
	c := sessionConn(t, m, h.ID)
	for _, msg := range toolTurn() {
		var content string
		if err := c.QueryRow(`SELECT content FROM messages WHERE id = ?`, msg.ID).Scan(&content); err != nil || content != string(msg.Data) {
			t.Fatalf("%s: content must be stored byte for byte: %q %v", msg.ID, content, err)
		}
		verifySplit(t, c, msg.ID)
	}
	var n int
	_ = c.QueryRow(`SELECT COUNT(*) FROM message_parts`).Scan(&n)
	if n != 2 { // one tool_args, one tool_result
		t.Fatalf("parts = %d, want 2", n)
	}
	var body []byte
	_ = c.QueryRow(`SELECT body FROM message_parts WHERE kind = 'tool_args'`).Scan(&body)
	if string(body) != `{"path":"auth.go"}` {
		t.Fatalf("tool arguments must be stored raw, not base64: %q", body)
	}
}

func TestSearchableTextIsStoredAndHoldsOnlyWords(t *testing.T) {
	s, m := partsStore(t)
	h := partsSession(t, s)
	if err := s.AppendMessages(h.ID, toolTurn()); err != nil {
		t.Fatal(err)
	}
	c := sessionConn(t, m, h.ID)
	text := func(id string) string {
		var v sql.NullString
		_ = c.QueryRow(`SELECT text FROM messages WHERE id = ?`, id).Scan(&v)
		return v.String
	}
	if text("u1") != "fix the login bug" {
		t.Fatalf("user text = %q", text("u1"))
	}
	if got := text("a1"); !strings.Contains(got, "Looking at it.") || !strings.Contains(got, "read_file auth.go") {
		t.Fatalf("assistant text = %q", got)
	}
	if got := text("r1"); got != "" {
		t.Fatalf("tool output must not be searchable text: %q", got)
	}
}

func TestDuplicateIDsAreIgnoredAndDoNotDuplicateParts(t *testing.T) {
	s, m := partsStore(t)
	h := partsSession(t, s)
	for i := 0; i < 3; i++ {
		if err := s.AppendMessages(h.ID, toolTurn()); err != nil {
			t.Fatal(err)
		}
	}
	c := sessionConn(t, m, h.ID)
	var msgs, parts int
	_ = c.QueryRow(`SELECT COUNT(*) FROM messages`).Scan(&msgs)
	_ = c.QueryRow(`SELECT COUNT(*) FROM message_parts`).Scan(&parts)
	if msgs != 3 || parts != 2 {
		t.Fatalf("messages=%d parts=%d after re-appending the same turn, want 3 and 2", msgs, parts)
	}
}

func TestReplaceRemovesTheOldPartsAndWritesNewOnes(t *testing.T) {
	s, m := partsStore(t)
	h := partsSession(t, s)
	if err := s.AppendMessages(h.ID, toolTurn()); err != nil {
		t.Fatal(err)
	}
	repl := []types.AgentMessage{
		{ID: "n1", Role: "user", Data: []byte(`{"user":{"content":"fresh start"}}`)},
		{ID: "n2", Role: "toolResult", Data: []byte(`{"toolResult":{"results":[{"toolCallId":"z","content":"` + enc("new") + `"}]}}`)},
	}
	if err := s.ReplaceMessages(h.ID, repl); err != nil {
		t.Fatal(err)
	}
	c := sessionConn(t, m, h.ID)
	var parts int
	_ = c.QueryRow(`SELECT COUNT(*) FROM message_parts`).Scan(&parts)
	if parts != 1 {
		t.Fatalf("parts = %d after replace; the old message's parts must be gone (want 1)", parts)
	}
	var orphans int
	_ = c.QueryRow(`SELECT COUNT(*) FROM message_parts WHERE message_id NOT IN (SELECT id FROM messages)`).Scan(&orphans)
	if orphans != 0 {
		t.Fatalf("%d orphaned parts", orphans)
	}
	verifySplit(t, c, "n2")
}

func TestForkCopiesMessagesWithTheirOwnParts(t *testing.T) {
	s, m := partsStore(t)
	h := partsSession(t, s)
	if err := s.AppendMessages(h.ID, toolTurn()); err != nil {
		t.Fatal(err)
	}
	fork, err := s.Fork(h.ID, "r1")
	if err != nil {
		t.Fatal(err)
	}
	fc := sessionConn(t, m, fork.ID)
	for _, id := range []string{"u1", "a1", "r1"} {
		verifySplit(t, fc, id)
	}
	var parts int
	_ = fc.QueryRow(`SELECT COUNT(*) FROM message_parts`).Scan(&parts)
	if parts != 2 {
		t.Fatalf("fork parts = %d, want 2", parts)
	}
	// The source is untouched.
	verifySplit(t, sessionConn(t, m, h.ID), "r1")
}

func TestAMessageThatCannotBeSplitIsStillStored(t *testing.T) {
	s, m := partsStore(t)
	h := partsSession(t, s)
	bad := types.AgentMessage{ID: "bad", Role: "user", Data: []byte(`not json at all`)}
	if err := s.AppendMessages(h.ID, []types.AgentMessage{bad}); err != nil {
		t.Fatalf("an unsplittable message must not fail the write: %v", err)
	}
	c := sessionConn(t, m, h.ID)
	var skeleton string
	var state int
	if err := c.QueryRow(`SELECT COALESCE(skeleton,''), split_state FROM messages WHERE id = 'bad'`).Scan(&skeleton, &state); err != nil {
		t.Fatal(err)
	}
	// The whole JSON becomes the row's own skeleton, with no parts to refill.
	if skeleton != "not json at all" || state != 1 {
		t.Fatalf("skeleton=%q state=%d; want the original kept whole and state 1", skeleton, state)
	}
	if contentKept(t, c) {
		var content string
		if err := c.QueryRow(`SELECT content FROM messages WHERE id = 'bad'`).Scan(&content); err != nil {
			t.Fatal(err)
		}
		if content != "not json at all" {
			t.Fatalf("content=%q; want the original kept", content)
		}
	}
}

func TestSchemaUpgradeIsIdempotent(t *testing.T) {
	s, m := partsStore(t)
	h := partsSession(t, s)
	c := sessionConn(t, m, h.ID)
	for i := 0; i < 3; i++ {
		if err := db.InitSessionDB(c, ":memory:"); err != nil {
			t.Fatalf("re-running the schema upgrade failed: %v", err)
		}
	}
}

func jsonUnmarshal(s string, v any) error { return json.Unmarshal([]byte(s), v) }

// An existing database, created before any of this, must upgrade in place: old
// rows keep their content untouched and are marked as not yet split, and new
// writes alongside them work.
func TestOldDatabaseUpgradesInPlaceWithoutTouchingRows(t *testing.T) {
	path := t.TempDir() + "/old.db"
	old, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	defer old.Close()
	// The schema exactly as it was before the split columns existed.
	if _, err := old.Exec(`
		CREATE TABLE session_meta (id INTEGER PRIMARY KEY CHECK (id = 1), title TEXT NOT NULL, cwd TEXT NOT NULL, project_id TEXT,
			model_id TEXT NOT NULL, provider TEXT NOT NULL, approval_mode TEXT NOT NULL DEFAULT 'always-ask', thinking_level TEXT,
			repaired INTEGER NOT NULL DEFAULT 0, created_at INTEGER NOT NULL, updated_at INTEGER NOT NULL);
		CREATE TABLE messages (id TEXT PRIMARY KEY, role TEXT NOT NULL, content TEXT NOT NULL, created_at INTEGER NOT NULL);
		CREATE INDEX idx_messages_created_at ON messages(created_at);`); err != nil {
		t.Fatal(err)
	}
	const content = `{"toolResult":{"results":[{"toolCallId":"c","content":"` + "YWJj" + `"}]}}`
	if _, err := old.Exec(`INSERT INTO messages (id, role, content, created_at) VALUES ('old1','toolResult',?,1)`, content); err != nil {
		t.Fatal(err)
	}

	if err := db.InitSessionDB(old, path); err != nil {
		t.Fatalf("upgrading an old database failed: %v", err)
	}
	var got string
	var state int
	var skel sql.NullString
	if err := old.QueryRow(`SELECT content, split_state, skeleton FROM messages WHERE id='old1'`).Scan(&got, &state, &skel); err != nil {
		t.Fatal(err)
	}
	if got != content {
		t.Fatalf("an old row's content changed during upgrade: %q", got)
	}
	if state != 0 || skel.Valid {
		t.Fatalf("an old row must be marked not-yet-split and left alone: state=%d skeleton=%v", state, skel)
	}
	var tbl int
	_ = old.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE name='message_parts'`).Scan(&tbl)
	if tbl != 1 {
		t.Fatal("message_parts was not created")
	}
}
