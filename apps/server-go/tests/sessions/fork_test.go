// Fork coverage: history cutoff, todos/changes copy scope, error cases,
// and shared-worktree headers.
package tests

import (
	"encoding/json"
	"testing"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/types"
)

func appendMsg(t *testing.T, sessions interface {
	AppendMessage(string, types.AgentMessage) error
}, sessionID, id, role string) {
	t.Helper()
	msg := types.AgentMessage{ID: id, Role: role}
	msg.Data, _ = json.Marshal(map[string]any{"role": role, "content": id})
	if err := sessions.AppendMessage(sessionID, msg); err != nil {
		t.Fatalf("append %s: %v", id, err)
	}
}

func TestForkCopiesThroughMessage(t *testing.T) {
	manager, sessions, _, _ := newTestManager(t)

	header, err := sessions.Create(types.CreateSessionOptions{
		Title: "Explore", Cwd: "/tmp/console", ModelID: "m", Provider: "p",
	})
	if err != nil {
		t.Fatal(err)
	}
	appendMsg(t, sessions, header.ID, "u1", "user")
	appendMsg(t, sessions, header.ID, "a1", "assistant")
	appendMsg(t, sessions, header.ID, "u2", "user")
	appendMsg(t, sessions, header.ID, "a2", "assistant")

	if err := sessions.SaveSessionTodos(header.ID, []types.TodoItem{
		{ID: 1, Content: "keep", Status: "in_progress"},
	}); err != nil {
		t.Fatal(err)
	}
	diff := "diff0"
	if err := sessions.RecordFileChange(header.ID, types.SessionFileChange{
		Path: "a.go", TurnIndex: 0, UserMessageID: "u1",
		Status: "modified", Additions: 3, Deletions: 1, DiffText: &diff,
	}); err != nil {
		t.Fatal(err)
	}
	diff1 := "diff1"
	if err := sessions.RecordFileChange(header.ID, types.SessionFileChange{
		Path: "b.go", TurnIndex: 1, UserMessageID: "u2",
		Status: "modified", Additions: 1, Deletions: 0, DiffText: &diff1,
	}); err != nil {
		t.Fatal(err)
	}
	// Legacy row with no user message id: turn 0 is kept, turn 9 is not.
	if err := sessions.RecordFileChange(header.ID, types.SessionFileChange{
		Path: "old.go", TurnIndex: 0, Status: "modified", Additions: 1,
	}); err != nil {
		t.Fatal(err)
	}
	if err := sessions.RecordFileChange(header.ID, types.SessionFileChange{
		Path: "future.go", TurnIndex: 9, Status: "modified", Additions: 1,
	}); err != nil {
		t.Fatal(err)
	}
	// A running subagent on the source must not carry over.
	srcConn, err := manager.Session(header.ID, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := srcConn.Exec(
		`INSERT INTO session_subagents (id, parent_tool_call_id, name, role, prompt, current_turn, status, activities, created_at, updated_at)
		VALUES ('sub1', 'call1', 'n', 'r', 'p', 0, 'running', '[]', 1, 1)`); err != nil {
		t.Fatal(err)
	}

	forked, err := sessions.Fork(header.ID, "a1")
	if err != nil {
		t.Fatalf("fork: %v", err)
	}
	if forked.ID == "" || forked.ID == header.ID {
		t.Fatalf("fork id not fresh: %+v", forked)
	}
	if forked.Title != "Fork of Explore" {
		t.Fatalf("title = %q", forked.Title)
	}
	if forked.Status != "idle" || forked.MessageCount != 2 {
		t.Fatalf("header = %+v", forked)
	}
	if forked.Cwd != header.Cwd || forked.ModelID != header.ModelID || forked.Provider != header.Provider {
		t.Fatalf("fork must share cwd/model: %+v vs %+v", forked, header)
	}

	loaded, err := sessions.Load(forked.ID, 0, 0)
	if err != nil || loaded == nil {
		t.Fatalf("load fork: %v %+v", err, loaded)
	}
	if len(loaded.Messages) != 2 {
		t.Fatalf("messages = %d, want 2", len(loaded.Messages))
	}
	var first map[string]any
	if err := json.Unmarshal(loaded.Messages[0], &first); err != nil || first["id"] != "u1" {
		t.Fatalf("first message: %s %v", loaded.Messages[0], err)
	}

	todos, err := sessions.GetSessionTodos(forked.ID)
	if err != nil || len(todos) != 1 || todos[0].Content != "keep" {
		t.Fatalf("todos: %+v %v", todos, err)
	}
	if subs, err := sessions.GetSubagents(forked.ID); err != nil || len(subs) != 0 {
		t.Fatalf("subagents must not copy: %+v %v", subs, err)
	}
	changes, err := sessions.GetSessionFileChanges(forked.ID, -1)
	if err != nil {
		t.Fatal(err)
	}
	paths := map[string]bool{}
	for _, c := range changes {
		paths[c.Path] = true
	}
	if !paths["a.go"] || !paths["old.go"] || paths["b.go"] || paths["future.go"] {
		t.Fatalf("change cutoff wrong: %v", paths)
	}

	// Source is untouched.
	if got, _ := sessions.Load(header.ID, 0, 0); got == nil || len(got.Messages) != 4 {
		t.Fatalf("source must keep 4 messages: %+v", got)
	}
}

func TestForkErrors(t *testing.T) {
	_, sessions, _, _ := newTestManager(t)

	if _, err := sessions.Fork("does-not-exist", "m1"); err == nil {
		t.Fatal("unknown session must fail")
	}
	header, err := sessions.Create(types.CreateSessionOptions{Cwd: "/tmp/console", ModelID: "m", Provider: "p"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := sessions.Fork(header.ID, ""); err == nil {
		t.Fatal("empty messageId must fail")
	}
	if _, err := sessions.Fork(header.ID, "missing"); err == nil {
		t.Fatal("unknown message must fail")
	}
	if _, err := sessions.SoftDelete(header.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := sessions.Fork(header.ID, "missing"); err == nil {
		t.Fatal("deleted session must fail")
	}
}

func TestForkSharesWorktree(t *testing.T) {
	manager, sessions, _, _ := newTestManager(t)

	header, err := sessions.Create(types.CreateSessionOptions{Cwd: "/tmp/console", ModelID: "m", Provider: "p"})
	if err != nil {
		t.Fatal(err)
	}
	appendMsg(t, sessions, header.ID, "u1", "user")
	if _, err := manager.Global().Exec(
		`UPDATE sessions SET worktree_path = '/tmp/wt', worktree_branch = 'feat', worktree_repo = '/tmp' WHERE id = ?`,
		header.ID); err != nil {
		t.Fatal(err)
	}

	forked, err := sessions.Fork(header.ID, "u1")
	if err != nil {
		t.Fatalf("fork: %v", err)
	}
	if forked.Worktree == nil || forked.Worktree.Path != "/tmp/wt" || forked.Worktree.Branch != "feat" {
		t.Fatalf("worktree not shared: %+v", forked.Worktree)
	}
	var owners int
	if err := manager.Global().QueryRow(
		`SELECT COUNT(*) FROM sessions WHERE worktree_path = '/tmp/wt' AND id != ?`,
		header.ID).Scan(&owners); err != nil || owners != 1 {
		t.Fatalf("owners = %d, %v", owners, err)
	}
}
