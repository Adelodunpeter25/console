// Cross-chat $references: parsing, project-scoped fuzzy resolution
// (never the current chat), transcript rendering and the read_chat tool.
package tests

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/tools"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/services"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/services/session"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/types"
)

// resultText extracts the first MCP content block from a tool result.
func resultText(t *testing.T, out any) string {
	t.Helper()
	blocks, ok := out.([]map[string]any)
	if !ok || len(blocks) == 0 {
		t.Fatalf("expected MCP content array, got %#v", out)
	}
	text, _ := blocks[0]["text"].(string)
	return text
}

func refSession(t *testing.T, s *services.SessionService, title, project string) types.SessionHeader {
	t.Helper()
	pid := project
	h, err := s.Create(types.CreateSessionOptions{Title: title, Cwd: t.TempDir(), ProjectID: &pid, ModelID: "m", Provider: "p"})
	if err != nil {
		t.Fatal(err)
	}
	return h
}

func TestParseDollarRefs(t *testing.T) {
	got := session.ParseDollarRefs("look at $fix-login-bug and $abc-def, again $fix-login-bug")
	if len(got) != 2 || got[0] != "fix-login-bug" || got[1] != "abc-def" {
		t.Fatalf("refs = %v", got)
	}
	// Prices, e-mails and bare dollars are not refs.
	if got := session.ParseDollarRefs("costs $5, mail a$b, $$ and $ alone"); len(got) != 0 {
		t.Fatalf("refs = %v, want none", got)
	}
	// A digit-led id still counts.
	if got := session.ParseDollarRefs("see $123e4567-89ab"); len(got) != 1 {
		t.Fatalf("refs = %v, want the id", got)
	}
}

func TestResolveReferenceByIDTitleAndSlug(t *testing.T) {
	s, _ := partsStore(t)
	current := refSession(t, s, "Current Chat", "p1")
	target := refSession(t, s, "Fix Login Bug", "p1")
	refSession(t, s, "Fix Login Bug", "p2") // same title, other project

	for _, ref := range []string{target.ID, "Fix Login Bug", "fix-login-bug", "FIX-LOGIN-BUG"} {
		h, err := s.ResolveReference(current.ID, ref)
		if err != nil || h.ID != target.ID {
			t.Fatalf("ref %q: h=%v err=%v, want %s", ref, h, err, target.ID)
		}
	}
}

func TestResolveReferenceFuzzyPrefixAndSubstring(t *testing.T) {
	s, _ := partsStore(t)
	current := refSession(t, s, "Current Chat", "p1")
	target := refSession(t, s, "Fix Login Bug", "p1")
	refSession(t, s, "Fix Signup Page", "p1")

	for _, ref := range []string{"fix-login", "login-bug", "login"} {
		h, err := s.ResolveReference(current.ID, ref)
		if err != nil || h.ID != target.ID {
			t.Fatalf("ref %q: h=%v err=%v, want %s", ref, h, err, target.ID)
		}
	}
}

func TestResolveReferenceAmbiguousListsCandidates(t *testing.T) {
	s, _ := partsStore(t)
	current := refSession(t, s, "Current Chat", "p1")
	refSession(t, s, "Alpha One", "p1")
	refSession(t, s, "Alpha Two", "p1")

	_, err := s.ResolveReference(current.ID, "alpha")
	if err == nil || !strings.Contains(err.Error(), "Alpha One") || !strings.Contains(err.Error(), "Alpha Two") {
		t.Fatalf("err = %v, want ambiguity listing both candidates", err)
	}
}

func TestResolveReferenceNeverCurrentChat(t *testing.T) {
	s, _ := partsStore(t)
	current := refSession(t, s, "Lonely Chat", "p1")

	if _, err := s.ResolveReference(current.ID, "lonely-chat"); err == nil {
		t.Fatal("want error: the only match is the current chat itself")
	}
	if _, err := s.ResolveReference(current.ID, current.ID); err == nil {
		t.Fatal("want error: referencing by own id is also excluded")
	}
}

func TestResolveReferenceScopedToProject(t *testing.T) {
	s, _ := partsStore(t)
	current := refSession(t, s, "Current Chat", "p1")
	refSession(t, s, "Other Project Chat", "p2")

	if _, err := s.ResolveReference(current.ID, "other-project-chat"); err == nil {
		t.Fatal("want error: chat belongs to another project")
	}
}

func TestResolveReferenceSkipsDeleted(t *testing.T) {
	s, _ := partsStore(t)
	current := refSession(t, s, "Current Chat", "p1")
	target := refSession(t, s, "Gone Chat", "p1")
	if _, err := s.SoftDelete(target.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ResolveReference(current.ID, "gone-chat"); err == nil {
		t.Fatal("want error: deleted chats are not referenceable")
	}
}

func TestReadReferencedChatRendersWordsOnly(t *testing.T) {
	s, _ := partsStore(t)
	current := refSession(t, s, "Current Chat", "p1")
	target := refSession(t, s, "Fix Login Bug", "p1")
	_ = s.AppendMessages(target.ID, []types.AgentMessage{
		say("u1", "user", "please fix the login bug"),
		say("a1", "assistant", "Found it in auth.go"),
		{ID: "r1", Role: "toolResult", Data: []byte(`{"toolResult":{"results":[{"toolCallId":"c","content":"` + enc("SECRETWORD") + `"}]}}`)},
	})

	title, id, transcript, err := s.ReadReferencedChat(current.ID, "$fix-login-bug", 0)
	if err != nil {
		t.Fatal(err)
	}
	if title != "Fix Login Bug" || id != target.ID {
		t.Fatalf("got %q (%s)", title, id)
	}
	if !strings.Contains(transcript, "User: please fix the login bug") ||
		!strings.Contains(transcript, "Assistant: Found it in auth.go") {
		t.Fatalf("transcript = %q", transcript)
	}
	if strings.Contains(transcript, "SECRETWORD") {
		t.Fatalf("transcript leaks tool output: %q", transcript)
	}
}

func TestReadReferencedChatLimit(t *testing.T) {
	s, _ := partsStore(t)
	current := refSession(t, s, "Current Chat", "p1")
	target := refSession(t, s, "Fix Login Bug", "p1")
	_ = s.AppendMessages(target.ID, []types.AgentMessage{
		say("u1", "user", "first message"),
		say("u2", "user", "second message"),
		say("u3", "user", "third message"),
	})
	_, _, transcript, err := s.ReadReferencedChat(current.ID, target.ID, 2)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(transcript, "first message") || !strings.Contains(transcript, "third message") {
		t.Fatalf("transcript = %q, want only the two most recent", transcript)
	}
}

func TestReadChatToolEndToEnd(t *testing.T) {
	s, _ := partsStore(t)
	current := refSession(t, s, "Current Chat", "p1")
	target := refSession(t, s, "Fix Login Bug", "p1")
	_ = s.AppendMessages(target.ID, []types.AgentMessage{say("u1", "user", "the login bug details")})

	tool := tools.NewReadChatTool(current.ID, s)
	args, _ := json.Marshal(map[string]any{"ref": "$fix-login-bug"})
	out, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatal(err)
	}
	text := resultText(t, out)
	if !strings.Contains(text, "Fix Login Bug") || !strings.Contains(text, "the login bug details") {
		t.Fatalf("tool output = %q", text)
	}
}

func TestReadChatToolErrors(t *testing.T) {
	s, _ := partsStore(t)
	current := refSession(t, s, "Current Chat", "p1")

	if _, err := tools.NewReadChatTool(current.ID, nil).Execute(context.Background(), []byte(`{"ref":"x"}`)); err == nil {
		t.Fatal("want error for nil reader")
	}
	tool := tools.NewReadChatTool(current.ID, s)
	if _, err := tool.Execute(context.Background(), []byte(`{"ref":""}`)); err == nil {
		t.Fatal("want error for empty ref")
	}
	if _, err := tool.Execute(context.Background(), []byte(`{"ref":"nope"}`)); err == nil {
		t.Fatal("want error for unknown ref (fail loud, not empty)")
	}
}
