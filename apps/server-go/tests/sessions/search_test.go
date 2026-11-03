// Search over what people said: the user's words, the assistant's replies and a
// line per tool call, never tool output, thinking, images or injected messages.
package tests

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/services"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/services/session"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/types"
)

func say(id, role, text string) types.AgentMessage {
	switch role {
	case "user":
		return types.AgentMessage{ID: id, Role: "user", Data: []byte(`{"user":{"content":` + mustJSON(text) + `}}`)}
	default:
		return types.AgentMessage{ID: id, Role: "assistant", Data: []byte(`{"assistant":{"id":"` + id + `","content":[{"text":{"text":` + mustJSON(text) + `}}],"stopReason":"stop"}}`)}
	}
}

func searchSvc(t *testing.T) (*services.SessionService, types.SessionHeader) {
	t.Helper()
	s, _ := partsStore(t)
	return s, partsSession(t, s)
}

func hitIDs(r *session.SearchResult) []string {
	var out []string
	for _, h := range r.Hits {
		out = append(out, h.MessageID)
	}
	return out
}

func TestSearchFindsWhatPeopleSaid(t *testing.T) {
	s, h := searchSvc(t)
	_ = s.AppendMessages(h.ID, []types.AgentMessage{
		say("u1", "user", "please fix the login bug"),
		say("a1", "assistant", "I found the Login handler in auth.go"),
		say("u2", "user", "thanks, now update the readme"),
	})
	r, err := s.Search(session.SearchScope{SessionID: h.ID}, "login", session.SearchOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if got := hitIDs(r); len(got) != 2 {
		t.Fatalf("hits = %v, want both messages that say login (case-insensitive)", got)
	}
	// Newest first.
	if r.Hits[0].MessageID != "a1" || r.Hits[1].MessageID != "u1" {
		t.Fatalf("order = %v, want newest first", hitIDs(r))
	}
	if r.Hits[0].Role != "assistant" || r.Hits[0].SessionTitle == "" {
		t.Fatalf("hit = %+v", r.Hits[0])
	}
}

func TestSearchRequiresEveryWordInTheSameMessage(t *testing.T) {
	s, h := searchSvc(t)
	_ = s.AppendMessages(h.ID, []types.AgentMessage{
		say("u1", "user", "the login page is slow"),
		say("u2", "user", "the signup page is fast"),
		say("u3", "user", "login is broken and the page crashes"),
	})
	r, _ := s.Search(session.SearchScope{SessionID: h.ID}, "login page", session.SearchOptions{})
	got := hitIDs(r)
	if len(got) != 2 || got[0] != "u3" || got[1] != "u1" {
		t.Fatalf("hits = %v, want u3 and u1 (u2 lacks 'login')", got)
	}
}

// Not words anyone wrote: none of these may ever surface.
func TestSearchNeverReturnsToolOutputThinkingImagesOrInjectedMessages(t *testing.T) {
	s, h := searchSvc(t)
	_ = s.AppendMessages(h.ID, []types.AgentMessage{
		{ID: "r1", Role: "toolResult", Data: []byte(`{"toolResult":{"results":[{"toolCallId":"c","content":"` + enc("SECRETWORD in tool output") + `"}]}}`)},
		{ID: "a1", Role: "assistant", Data: []byte(`{"assistant":{"id":"a1","content":[{"thinking":{"text":"SECRETWORD in thinking"}},{"text":{"text":"nothing here"}}],"stopReason":"stop"}}`)},
		{ID: "i1", Role: "user", Data: []byte(`{"user":{"content":"look","attachments":[{"data":"` + enc("SECRETWORD pixels") + `","mimeType":"image/png"}]}}`)},
		{ID: "s1", Role: "user", Data: []byte(`{"user":{"content":"SECRETWORD from a finished job","synthetic":true}}`)},
	})
	r, err := s.Search(session.SearchScope{SessionID: h.ID}, "secretword", session.SearchOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Hits) != 0 {
		t.Fatalf("found text nobody wrote to be read: %v", hitIDs(r))
	}
}

func TestSearchFindsAToolCallByItsNameAndMainArgument(t *testing.T) {
	s, h := searchSvc(t)
	_ = s.AppendMessages(h.ID, []types.AgentMessage{{ID: "a1", Role: "assistant", Data: []byte(
		`{"assistant":{"id":"a1","content":[{"toolCall":{"id":"c","name":"read_file","arguments":"` + enc(`{"path":"internal/auth/login.go"}`) + `"}}],"stopReason":"toolUse"}}`)}})
	r, _ := s.Search(session.SearchScope{SessionID: h.ID}, "auth/login.go", session.SearchOptions{})
	if len(r.Hits) != 1 {
		t.Fatalf("a file the agent read should be findable: %v", hitIDs(r))
	}
}

func TestSearchRoleFilter(t *testing.T) {
	s, h := searchSvc(t)
	_ = s.AppendMessages(h.ID, []types.AgentMessage{say("u1", "user", "deploy now"), say("a1", "assistant", "deploy started")})
	users, _ := s.Search(session.SearchScope{SessionID: h.ID}, "deploy", session.SearchOptions{Roles: []string{"user"}})
	if got := hitIDs(users); len(got) != 1 || got[0] != "u1" {
		t.Fatalf("user only = %v", got)
	}
	if _, err := s.Search(session.SearchScope{SessionID: h.ID}, "deploy", session.SearchOptions{Roles: []string{"toolResult"}}); err == nil {
		t.Fatal("a role that cannot be searched must be refused, not silently ignored")
	}
}

// SQLite's lower() only folds ASCII; a search for "école" must still find "ÉCOLE".
func TestSearchFoldsNonASCIICase(t *testing.T) {
	s, h := searchSvc(t)
	_ = s.AppendMessages(h.ID, []types.AgentMessage{say("u1", "user", "visit the ÉCOLE tomorrow"), say("u2", "user", "Straße and ΣΊΣΥΦΟΣ")})
	for _, q := range []string{"école", "ÉCOLE", "straße", "σίσυφοσ"} {
		r, err := s.Search(session.SearchScope{SessionID: h.ID}, q, session.SearchOptions{})
		if err != nil || len(r.Hits) == 0 {
			t.Errorf("%q found nothing (err %v)", q, err)
		}
	}
}

func TestSearchMatchOffsetsAreInCharactersNotBytes(t *testing.T) {
	s, h := searchSvc(t)
	text := "日本語 and emoji 🔥 then the TARGET word"
	_ = s.AppendMessages(h.ID, []types.AgentMessage{say("u1", "user", text)})
	r, _ := s.Search(session.SearchScope{SessionID: h.ID}, "target", session.SearchOptions{})
	if len(r.Hits) != 1 || len(r.Hits[0].Matches) != 1 {
		t.Fatalf("hit = %+v", r.Hits)
	}
	m := r.Hits[0].Matches[0]
	runes := []rune(text)
	if got := string(runes[m.Start : m.Start+m.Length]); strings.ToLower(got) != "target" {
		t.Fatalf("offset %d+%d selects %q, not the match (offsets must count characters, not bytes)", m.Start, m.Length, got)
	}
}

func TestSearchReportsEveryOccurrenceForHighlighting(t *testing.T) {
	s, h := searchSvc(t)
	_ = s.AppendMessages(h.ID, []types.AgentMessage{say("u1", "user", "bug one, bug two, and another bug")})
	r, _ := s.Search(session.SearchScope{SessionID: h.ID}, "bug", session.SearchOptions{})
	if got := len(r.Hits[0].Matches); got != 3 {
		t.Fatalf("matches = %d, want 3 (a find bar steps through each)", got)
	}
}

func TestSearchSnippetIsOneLineWithContextAndEllipses(t *testing.T) {
	s, h := searchSvc(t)
	long := strings.Repeat("before ", 40) + "NEEDLE\n\n   in   the\tmiddle " + strings.Repeat("after ", 40)
	_ = s.AppendMessages(h.ID, []types.AgentMessage{say("u1", "user", long)})
	r, _ := s.Search(session.SearchScope{SessionID: h.ID}, "needle", session.SearchOptions{})
	sn := r.Hits[0].Snippet
	if !strings.Contains(sn, "NEEDLE") || !strings.HasPrefix(sn, "…") || !strings.HasSuffix(sn, "…") {
		t.Fatalf("snippet = %q", sn)
	}
	if strings.ContainsAny(sn, "\n\t") || strings.Contains(sn, "  ") {
		t.Fatalf("a snippet must be one tidy line: %q", sn)
	}
	if len([]rune(sn)) > 200 {
		t.Fatalf("snippet is %d characters; it should be a short window", len([]rune(sn)))
	}
}

func TestSearchLimitsAndSaysWhenItStoppedEarly(t *testing.T) {
	s, h := searchSvc(t)
	var msgs []types.AgentMessage
	for i := 0; i < 30; i++ {
		msgs = append(msgs, say(fmt.Sprintf("u%02d", i), "user", fmt.Sprintf("needle number %d", i)))
	}
	_ = s.AppendMessages(h.ID, msgs)

	r, _ := s.Search(session.SearchScope{SessionID: h.ID}, "needle", session.SearchOptions{Limit: 10, PerSession: 50})
	if len(r.Hits) != 10 || !r.Truncated || r.Note == "" {
		t.Fatalf("hits=%d truncated=%v note=%q; a cut-off result must say so", len(r.Hits), r.Truncated, r.Note)
	}
	// One session returns at most PerSession by default.
	d, _ := s.Search(session.SearchScope{SessionID: h.ID}, "needle", session.SearchOptions{})
	if len(d.Hits) != session.DefaultPerSession {
		t.Fatalf("default per-session cap = %d hits, want %d", len(d.Hits), session.DefaultPerSession)
	}
	// A limit beyond the maximum is capped, not honoured.
	big, _ := s.Search(session.SearchScope{SessionID: h.ID}, "needle", session.SearchOptions{Limit: 100000, PerSession: 100000})
	if len(big.Hits) > session.MaxSearchLimit {
		t.Fatalf("returned %d hits; the ceiling is %d", len(big.Hits), session.MaxSearchLimit)
	}
}

func TestSearchRefusesBadQueries(t *testing.T) {
	s, h := searchSvc(t)
	for name, q := range map[string]string{
		"empty": "", "spaces": "   \t\n", "too many words": "a b c d e f g h i",
	} {
		if _, err := s.Search(session.SearchScope{SessionID: h.ID}, q, session.SearchOptions{}); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
	if _, err := s.Search(session.SearchScope{SessionID: "ghost"}, "x", session.SearchOptions{}); err == nil {
		t.Error("an unknown session must be an error")
	}
}

// % and _ are wildcards in SQL LIKE and quotes end SQL strings: none of them may
// mean anything here, and none may break the query.
func TestSearchTreatsEveryCharacterAsLiteral(t *testing.T) {
	s, h := searchSvc(t)
	_ = s.AppendMessages(h.ID, []types.AgentMessage{
		say("u1", "user", `100% done, file_name and a'quote"d; DROP TABLE messages;--`),
		say("u2", "user", "nothing special here"),
	})
	for _, q := range []string{"100%", "file_name", `a'quote"d;`, "drop table", "%", "_", `\`, "'", `"`, "--", "*", "(", ")"} {
		r, err := s.Search(session.SearchScope{SessionID: h.ID}, q, session.SearchOptions{})
		if err != nil {
			t.Fatalf("%q: %v", q, err)
		}
		for _, hit := range r.Hits {
			if hit.MessageID == "u2" {
				t.Errorf("%q matched a message that does not contain it (treated as a wildcard?)", q)
			}
		}
	}
	// And the table is still there.
	if r, err := s.Search(session.SearchScope{SessionID: h.ID}, "special", session.SearchOptions{}); err != nil || len(r.Hits) != 1 {
		t.Fatalf("the store was damaged by a hostile query: %v", err)
	}
}

func TestSearchAcrossSessionsInAProjectAndExcludingOne(t *testing.T) {
	s, m := partsStore(t)
	proj := t.TempDir()
	make1 := func(title string) types.SessionHeader {
		h, err := s.Create(types.CreateSessionOptions{Title: title, Cwd: proj, ModelID: "m", Provider: "p"})
		if err != nil {
			t.Fatal(err)
		}
		return h
	}
	_ = m
	a, b, c := make1("Alpha"), make1("Bravo"), make1("Charlie")
	for _, h := range []types.SessionHeader{a, b, c} {
		_ = s.AppendMessages(h.ID, []types.AgentMessage{say("m-"+h.Title, "user", "the migration plan for "+h.Title)})
	}
	all, err := s.Search(session.SearchScope{Cwd: proj}, "migration", session.SearchOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(all.Hits) != 3 || all.SessionsSearched != 3 {
		t.Fatalf("hits=%d searched=%d, want 3 and 3", len(all.Hits), all.SessionsSearched)
	}
	// The session the agent is running in is left out.
	ex, _ := s.Search(session.SearchScope{Cwd: proj, Exclude: []string{b.ID}}, "migration", session.SearchOptions{})
	for _, hit := range ex.Hits {
		if hit.SessionID == b.ID {
			t.Fatal("an excluded session was searched")
		}
	}
	if len(ex.Hits) != 2 || ex.SessionsSearched != 2 {
		t.Fatalf("after excluding one: hits=%d searched=%d", len(ex.Hits), ex.SessionsSearched)
	}
	// Soft-deleted sessions are out of scope.
	_, _ = s.SoftDelete(c.ID)
	gone, _ := s.Search(session.SearchScope{Cwd: proj}, "migration", session.SearchOptions{})
	for _, hit := range gone.Hits {
		if hit.SessionID == c.ID {
			t.Fatal("a deleted session was searched")
		}
	}
}

// Old messages have no searchable text until split; search must convert them, not
// skip them and report "nothing found".
func TestSearchFindsMessagesFromBeforeTheSplitStorageExisted(t *testing.T) {
	s, m := partsStore(t)
	h := partsSession(t, s)
	c := sessionConn(t, m, h.ID)
	legacyRows(t, c, []types.AgentMessage{say("old1", "user", "the ancient dragon problem")})
	var pending int
	_ = c.QueryRow(`SELECT COUNT(*) FROM messages WHERE split_state = 0`).Scan(&pending)
	if pending != 1 {
		t.Fatal("setup: expected one unsplit legacy row")
	}
	r, err := s.Search(session.SearchScope{SessionID: h.ID}, "dragon", session.SearchOptions{})
	if err != nil || len(r.Hits) != 1 {
		t.Fatalf("an old message was invisible to search: hits=%v err=%v", r, err)
	}
}

// A synthetic injected message must stay out of the words column itself, not just
// out of search results.
func TestInjectedMessagesAreNotStoredAsSearchableText(t *testing.T) {
	s, m := partsStore(t)
	h := partsSession(t, s)
	_ = s.AppendMessages(h.ID, []types.AgentMessage{
		{ID: "s1", Role: "user", Data: []byte(`{"user":{"content":"background job finished","synthetic":true}}`)},
		{ID: "u1", Role: "user", Data: []byte(`{"user":{"content":"a real message"}}`)},
	})
	c := sessionConn(t, m, h.ID)
	var text *string
	_ = c.QueryRow(`SELECT text FROM messages WHERE id = 's1'`).Scan(&text)
	if text != nil && *text != "" {
		t.Fatalf("an injected message has searchable text: %q", *text)
	}
}

func mustJSON(s string) string { b, _ := json.Marshal(s); return string(b) }
