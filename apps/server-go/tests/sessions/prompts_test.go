// The prompt list behind the "jump to prompt" popup: every user prompt of a
// chat, oldest first, with synthetic turns and file-comment markup left out.
package tests

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/services/session"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/types"
)

func sessionFixture(t *testing.T, name string) []byte {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "proto", "testdata", "session", name))
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func TestListPromptsOldestFirstWithoutSyntheticOrMarkup(t *testing.T) {
	s, _ := partsStore(t)
	h := refSession(t, s, "Prompts", "p1")
	comment := "<file_comment path=\"a.go\" lines=\"1–2\">\nrename\n</file_comment>"
	if err := s.AppendMessages(h.ID, []types.AgentMessage{
		say("u1", "user", "\n  first line\nsecond line"),
		say("a1", "assistant", "an answer"),
		{ID: "syn", Role: "user", Data: []byte(`{"user":{"content":"background done","synthetic":true}}`)},
		say("u2", "user", comment+" fix this"),
		say("u3", "user", comment),
	}); err != nil {
		t.Fatal(err)
	}

	got, err := s.ListPrompts(h.ID)
	if err != nil {
		t.Fatal(err)
	}
	// Timestamps are the insert time; pin them so the fixture is stable.
	for i := range got {
		got[i].CreatedAt = 1700000000000 + int64(i+1)*1000
	}
	raw, _ := json.Marshal(got)
	want := strings.TrimSpace(string(sessionFixture(t, "prompts.json")))
	if string(raw) != want {
		t.Fatalf("prompts drifted:\n got %s\nwant %s", raw, want)
	}

	missing, err := s.ListPrompts("no-such-session")
	if err != nil || missing != nil {
		t.Fatalf("unknown session = %v, %v; want nil, nil", missing, err)
	}
}

func TestListPromptsTruncatesLongLines(t *testing.T) {
	s, _ := partsStore(t)
	h := refSession(t, s, "Long", "p1")
	_ = s.AppendMessages(h.ID, []types.AgentMessage{say("u1", "user", strings.Repeat("é", 500))})
	got, err := s.ListPrompts(h.ID)
	if err != nil || len(got) != 1 || len([]rune(got[0].Preview)) != 160 {
		t.Fatalf("got %v, %v", got, err)
	}
}

// Shared with the Rust and Kotlin parsers: the server's rules are the source
// of truth for what a `$ref` is.
func TestParseDollarRefsMatchesSharedFixture(t *testing.T) {
	var cases []struct {
		Text string   `json:"text"`
		Refs []string `json:"refs"`
	}
	if err := json.Unmarshal(sessionFixture(t, "chat_refs.json"), &cases); err != nil {
		t.Fatal(err)
	}
	for _, c := range cases {
		got := session.ParseDollarRefs(c.Text)
		if len(got) != len(c.Refs) {
			t.Errorf("%q: refs = %v, want %v", c.Text, got, c.Refs)
			continue
		}
		for i := range got {
			if got[i] != c.Refs[i] {
				t.Errorf("%q: refs = %v, want %v", c.Text, got, c.Refs)
				break
			}
		}
	}
}
