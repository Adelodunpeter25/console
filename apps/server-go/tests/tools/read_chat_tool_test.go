// read_chat tool over a fake ChatReader: argument handling and error
// surfacing. Storage-backed resolution is covered in tests/sessions.
package tests

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/tools"
)

type fakeChatReader struct {
	title, id, transcript string
	err                   error
	gotCurrent, gotRef    string
	gotLimit              int
}

func (f *fakeChatReader) ReadReferencedChat(current, ref string, limit int) (string, string, string, error) {
	f.gotCurrent, f.gotRef, f.gotLimit = current, ref, limit
	return f.title, f.id, f.transcript, f.err
}

func TestReadChatToolStripsDollarAndPassesLimit(t *testing.T) {
	r := &fakeChatReader{title: "Fix Login Bug", id: "abc", transcript: "User: hi"}
	tool := tools.NewReadChatTool("current-1", r)
	args, _ := json.Marshal(map[string]any{"ref": "$fix-login-bug", "limit": 10})
	out, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatal(err)
	}
	if r.gotCurrent != "current-1" || r.gotRef != "fix-login-bug" || r.gotLimit != 10 {
		t.Fatalf("reader got current=%q ref=%q limit=%d", r.gotCurrent, r.gotRef, r.gotLimit)
	}
	if text := resultText(t, out); !strings.Contains(text, "Fix Login Bug") || !strings.Contains(text, "User: hi") {
		t.Fatalf("output = %q", text)
	}
}

func TestReadChatToolEmptyTranscript(t *testing.T) {
	r := &fakeChatReader{title: "Empty Chat", id: "abc"}
	tool := tools.NewReadChatTool("current-1", r)
	out, err := tool.Execute(context.Background(), []byte(`{"ref":"empty-chat"}`))
	if err != nil {
		t.Fatal(err)
	}
	if text := resultText(t, out); !strings.Contains(text, "No messages yet") {
		t.Fatalf("output = %q", text)
	}
}

func TestReadChatToolFailsLoud(t *testing.T) {
	tool := tools.NewReadChatTool("current-1", nil)
	if _, err := tool.Execute(context.Background(), []byte(`{"ref":"x"}`)); err == nil {
		t.Fatal("want error for nil reader")
	}
	tool = tools.NewReadChatTool("current-1", &fakeChatReader{})
	if _, err := tool.Execute(context.Background(), []byte(`{"ref":"  "}`)); err == nil {
		t.Fatal("want error for empty ref")
	}
	r := &fakeChatReader{err: errFake}
	tool = tools.NewReadChatTool("current-1", r)
	if _, err := tool.Execute(context.Background(), []byte(`{"ref":"nope"}`)); err == nil {
		t.Fatal("want error when resolution fails")
	}
}

var errFake = &fakeError{"no chat matching \"nope\""}

type fakeError struct{ msg string }

func (e *fakeError) Error() string { return e.msg }
