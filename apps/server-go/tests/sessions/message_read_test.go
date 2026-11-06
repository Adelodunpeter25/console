// Load rebuilds split messages from their parts. While the stage-6 migration
// has not dropped `content`, a rebuild is proven against it before serving;
// after the drop, the rebuild is the source of truth and a damaged message is
// left out (loudly logged), never served wrong.
package tests

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/msgparts"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/services"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/types"
)

func loadedJSON(t *testing.T, raw json.RawMessage) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	return m
}

// What a client receives must be the stored message plus id and createdAt,
// exactly as before the split storage existed.
func TestLoadReturnsTheSameMessagesAsBefore(t *testing.T) {
	s, m := partsStore(t)
	h := partsSession(t, s)
	if err := s.AppendMessages(h.ID, toolTurn()); err != nil {
		t.Fatal(err)
	}
	c := sessionConn(t, m, h.ID)
	loaded, err := s.Load(h.ID, 0, 0)
	if err != nil || len(loaded.Messages) != 3 {
		t.Fatalf("load: %v", err)
	}
	for i, want := range toolTurn() {
		got := loadedJSON(t, loaded.Messages[i])
		if got["id"] != want.ID {
			t.Errorf("message %d: id = %v, want %s", i, got["id"], want.ID)
		}
		if _, ok := got["createdAt"]; !ok {
			t.Errorf("message %d: createdAt missing", i)
		}
		delete(got, "id")
		delete(got, "createdAt")
		gotRaw, _ := json.Marshal(got)
		if same, _ := msgparts.Equal(want.Data, gotRaw); !same {
			t.Errorf("message %d changed on load\n want %s\n got  %s", i, want.Data, gotRaw)
		}
	}
	_ = c
}

// Repeated loads give the same messages every time: with `content` gone, every
// read rebuilds from parts, so nothing may depend on a one-time check.
func TestLoadIsTheSameOnEveryRead(t *testing.T) {
	s, _ := partsStore(t)
	h := partsSession(t, s)
	if err := s.AppendMessages(h.ID, toolTurn()); err != nil {
		t.Fatal(err)
	}
	var first json.RawMessage
	for i := 0; i < 3; i++ {
		loaded, err := s.Load(h.ID, 0, 0)
		if err != nil || len(loaded.Messages) != 3 {
			t.Fatalf("load %d: %v", i, err)
		}
		if i == 0 {
			first = loaded.Messages[2]
			continue
		}
		if string(first) != string(loaded.Messages[2]) {
			t.Fatalf("load %d served a different message than load 0:\n %s\n %s", i, first, loaded.Messages[2])
		}
	}
}

// If the parts are damaged, the stage-6 audit refuses to drop `content`, so
// the original stays the source of truth and the load serves it. A damaged
// message must degrade to the original, never to wrong history.
func TestLoadFallsBackToTheOriginalWhenPartsAreDamaged(t *testing.T) {
	s, m := partsStore(t)
	h := partsSession(t, s)
	if err := s.AppendMessages(h.ID, toolTurn()); err != nil {
		t.Fatal(err)
	}
	c := sessionConn(t, m, h.ID)
	damage := map[string]string{
		"wrong body":      `UPDATE message_parts SET body = x'deadbeef' WHERE message_id = 'r1'`,
		"missing part":    `DELETE FROM message_parts WHERE message_id = 'r1'`,
		"truncated body":  `UPDATE message_parts SET body = substr(body, 1, 3) WHERE message_id = 'r1'`,
		"bad meta":        `UPDATE message_parts SET meta = 'not json' WHERE message_id = 'r1'`,
		"broken skeleton": `UPDATE messages SET skeleton = '{"toolResult":{"results":[{"toolCallId":"x"}]}}' WHERE id = 'r1'`,
		"skeleton junk":   `UPDATE messages SET skeleton = 'junk' WHERE id = 'r1'`,
	}
	var original string
	_ = c.QueryRow(`SELECT content FROM messages WHERE id = 'r1'`).Scan(&original)
	for name, stmt := range damage {
		// Start each case from a healthy copy of the message.
		if _, err := c.Exec(`DELETE FROM message_parts WHERE message_id = 'r1'`); err != nil {
			t.Fatal(err)
		}
		res, _ := msgparts.Split([]byte(original))
		if _, err := c.Exec(`UPDATE messages SET skeleton = ?, split_state = 1 WHERE id = 'r1'`, string(res.Skeleton)); err != nil {
			t.Fatal(err)
		}
		for _, p := range res.Parts {
			meta, _ := json.Marshal(p.Meta)
			if _, err := c.Exec(`INSERT INTO message_parts (message_id, slot, kind, meta, body) VALUES ('r1', ?, ?, ?, ?)`, p.Slot, p.Kind, string(meta), p.Body); err != nil {
				t.Fatal(err)
			}
		}
		// A fresh service re-runs the stage-6 audit, so this load really
		// rebuilds and compares instead of trusting an earlier pass. The audit
		// finds the damage, refuses to drop `content`, and the load serves the
		// original.
		s2 := services.NewSessionService(m)
		if _, err := c.Exec(stmt); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		loaded, err := s2.Load(h.ID, 0, 0)
		if err != nil {
			t.Fatalf("%s: a damaged message must not fail the load: %v", name, err)
		}
		got := loadedJSON(t, loaded.Messages[2])
		delete(got, "id")
		delete(got, "createdAt")
		gotRaw, _ := json.Marshal(got)
		if same, _ := msgparts.Equal([]byte(original), gotRaw); !same {
			t.Errorf("%s: served wrong history instead of the original\n got %s", name, gotRaw)
		}
	}
}

// Many messages with many parts: the rebuild must not deadlock on the session
// database's single connection, and must stay correct across query chunks.
func TestLoadRebuildsAHugeSessionWithoutDeadlock(t *testing.T) {
	s, _ := partsStore(t)
	h := partsSession(t, s)
	var msgs []types.AgentMessage
	for i := 0; i < 900; i++ {
		msgs = append(msgs, types.AgentMessage{
			ID: fmt.Sprintf("r%04d", i), Role: "toolResult",
			Data: []byte(`{"toolResult":{"results":[{"toolCallId":"c","content":"` + enc(fmt.Sprintf("output %d", i)) + `"}]}}`),
		})
	}
	if err := s.AppendMessages(h.ID, msgs); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() {
		loaded, err := s.Load(h.ID, 0, 0)
		if err == nil && len(loaded.Messages) != 900 {
			err = fmt.Errorf("got %d messages", len(loaded.Messages))
		}
		done <- err
	}()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(20 * time.Second):
		t.Fatal("Load deadlocked: it read parts while the message rows were still open on the single connection")
	}
}

func TestLoadPaginationStillWorksWithRebuiltMessages(t *testing.T) {
	s, _ := partsStore(t)
	h := partsSession(t, s)
	var msgs []types.AgentMessage
	for i := 0; i < 10; i++ {
		msgs = append(msgs, types.AgentMessage{ID: fmt.Sprintf("r%02d", i), Role: "toolResult",
			Data: []byte(`{"toolResult":{"results":[{"toolCallId":"c","content":"` + enc(fmt.Sprintf("n%d", i)) + `"}]}}`)})
	}
	if err := s.AppendMessages(h.ID, msgs); err != nil {
		t.Fatal(err)
	}
	page, err := s.Load(h.ID, 4, 0)
	if err != nil || len(page.Messages) != 4 || !page.HasMore || page.NextCursor == nil {
		t.Fatalf("page 1: %v %+v", err, page)
	}
	if loadedJSON(t, page.Messages[3])["id"] != "r09" {
		t.Fatalf("newest page must end at r09")
	}
	next, err := s.Load(h.ID, 4, *page.NextCursor)
	if err != nil || len(next.Messages) != 4 || loadedJSON(t, next.Messages[3])["id"] != "r05" {
		t.Fatalf("page 2: %v", err)
	}
}

// Rewriting a session's messages and then loading them must serve the new
// history, built the same way as any other message.
func TestReplaceThenLoadServesTheNewHistory(t *testing.T) {
	s, _ := partsStore(t)
	h := partsSession(t, s)
	if err := s.AppendMessages(h.ID, toolTurn()); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Load(h.ID, 0, 0); err != nil {
		t.Fatal(err)
	}
	if err := s.ReplaceMessages(h.ID, toolTurn()[:1]); err != nil {
		t.Fatal(err)
	}
	loaded, err := s.Load(h.ID, 0, 0)
	if err != nil || len(loaded.Messages) != 1 {
		t.Fatalf("load: %v", err)
	}
	got := loadedJSON(t, loaded.Messages[0])
	if got["id"] != "u1" {
		t.Fatalf("served %v, want the replacement message u1", got["id"])
	}
}
