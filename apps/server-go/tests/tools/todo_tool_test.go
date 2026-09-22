// Coverage for the todo tool: unbound in-memory behavior and real
// persistence through SessionService's session_todos table.
package tests

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/tools"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/db"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/services"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/types"
	"github.com/Adelodunpeter25/console/apps/server-go/tests/helpers"
)

func TestTodoUnboundLifecycle(t *testing.T) {
	todo := tools.NewTodoTool("", nil)

	initArgs, _ := json.Marshal(map[string]any{"op": "init", "tasks": []string{"first", "second"}})
	out, err := todo.Execute(context.Background(), initArgs)
	if err != nil {
		t.Fatalf("init: %v", err)
	}
	s := resultText(t, out)
	if !strings.Contains(s, "#1: first (pending)") || !strings.Contains(s, "#2: second (pending)") {
		t.Fatalf("init output: %v", out)
	}

	startArgs, _ := json.Marshal(map[string]any{"op": "start", "index": 1})
	out, err = todo.Execute(context.Background(), startArgs)
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	s = resultText(t, out)
	if !strings.Contains(s, "#1: first (in_progress)") {
		t.Fatalf("start output: %v", out)
	}

	doneArgs, _ := json.Marshal(map[string]any{"op": "done", "index": 1})
	if _, err := todo.Execute(context.Background(), doneArgs); err != nil {
		t.Fatalf("done: %v", err)
	}

	appendArgs, _ := json.Marshal(map[string]any{"op": "append", "tasks": []string{"third"}})
	out, err = todo.Execute(context.Background(), appendArgs)
	if err != nil {
		t.Fatalf("append: %v", err)
	}
	s = resultText(t, out)
	if !strings.Contains(s, "#3: third (pending)") {
		t.Fatalf("append output: %v", out)
	}

	viewArgs, _ := json.Marshal(map[string]any{"op": "view"})
	out, err = todo.Execute(context.Background(), viewArgs)
	if err != nil {
		t.Fatalf("view: %v", err)
	}
	s = resultText(t, out)
	if !strings.Contains(s, "#1: first (completed)") {
		t.Fatalf("view output: %v", out)
	}

	// Missing index / tasks -> tool errors.
	if _, err := todo.Execute(context.Background(), helpers.MustJSONRaw(t, map[string]any{"op": "start"})); err == nil {
		t.Fatal("expected error for missing index")
	}
	if _, err := todo.Execute(context.Background(), helpers.MustJSONRaw(t, map[string]any{"op": "init"})); err == nil {
		t.Fatal("expected error for missing tasks")
	}
	if _, err := todo.Execute(context.Background(), helpers.MustJSONRaw(t, map[string]any{"op": "done", "index": 999})); err == nil {
		t.Fatal("expected error for unknown index")
	}
}

func TestTodoPersistsAcrossToolInstances(t *testing.T) {
	manager, err := db.Open(db.OpenOptions{Path: ":memory:"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(manager.Close)
	sessions := services.NewSessionService(manager)
	header, err := sessions.Create(types.CreateSessionOptions{
		Cwd: "/tmp/todo-test", ModelID: "mock", Provider: "mock",
	})
	if err != nil {
		t.Fatal(err)
	}

	// First tool instance initializes the list.
	first := tools.NewTodoTool(header.ID, sessions)
	initArgs, _ := json.Marshal(map[string]any{"op": "init", "tasks": []string{"write code", "write tests"}})
	if _, err := first.Execute(context.Background(), initArgs); err != nil {
		t.Fatalf("init: %v", err)
	}
	startArgs, _ := json.Marshal(map[string]any{"op": "start", "index": 1})
	if _, err := first.Execute(context.Background(), startArgs); err != nil {
		t.Fatalf("start: %v", err)
	}

	// A fresh tool instance for the same session must load persisted state.
	second := tools.NewTodoTool(header.ID, sessions)
	viewArgs, _ := json.Marshal(map[string]any{"op": "view"})
	out, err := second.Execute(context.Background(), viewArgs)
	if err != nil {
		t.Fatalf("view: %v", err)
	}
	s := resultText(t, out)
	if !strings.Contains(s, "#1: write code (in_progress)") || !strings.Contains(s, "#2: write tests (pending)") {
		t.Fatalf("persisted view: %v", out)
	}

	// Verify it round-trips through SessionService directly too.
	stored, err := sessions.GetSessionTodos(header.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(stored) != 2 || stored[0].Status != "in_progress" {
		t.Fatalf("stored todos: %+v", stored)
	}

	// And the HTTP-facing shape (GetSessionTodos) matches what the second
	// tool instance saw.
	if err := sessions.ClearSessionTodos(header.ID); err != nil {
		t.Fatal(err)
	}
	cleared, err := sessions.GetSessionTodos(header.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(cleared) != 0 {
		t.Fatalf("expected cleared todos, got: %+v", cleared)
	}
}

// TestClearCompletedTodos matches RunService's end-of-run cleanup (TS
// run.service.ts finally block): only a non-empty list where EVERY item is
// completed gets wiped; a partial list is left alone.
func TestClearCompletedTodos(t *testing.T) {
	manager, err := db.Open(db.OpenOptions{Path: ":memory:"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(manager.Close)
	sessions := services.NewSessionService(manager)
	header, err := sessions.Create(types.CreateSessionOptions{
		Cwd: "/tmp/todo-clear-test", ModelID: "mock", Provider: "mock",
	})
	if err != nil {
		t.Fatal(err)
	}

	// Empty list: no-op.
	if err := sessions.ClearCompletedTodos(header.ID); err != nil {
		t.Fatalf("clear on empty: %v", err)
	}

	// Partially done: must survive.
	partial := []types.TodoItem{
		{ID: 1, Content: "a", Status: "completed"},
		{ID: 2, Content: "b", Status: "in_progress"},
	}
	if err := sessions.SaveSessionTodos(header.ID, partial); err != nil {
		t.Fatal(err)
	}
	if err := sessions.ClearCompletedTodos(header.ID); err != nil {
		t.Fatalf("clear on partial: %v", err)
	}
	stillThere, err := sessions.GetSessionTodos(header.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(stillThere) != 2 {
		t.Fatalf("partial list must survive: %+v", stillThere)
	}

	// Fully done: must be wiped.
	done := []types.TodoItem{
		{ID: 1, Content: "a", Status: "completed"},
		{ID: 2, Content: "b", Status: "completed"},
	}
	if err := sessions.SaveSessionTodos(header.ID, done); err != nil {
		t.Fatal(err)
	}
	if err := sessions.ClearCompletedTodos(header.ID); err != nil {
		t.Fatalf("clear on all-done: %v", err)
	}
	gone, err := sessions.GetSessionTodos(header.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(gone) != 0 {
		t.Fatalf("fully-completed list must be wiped: %+v", gone)
	}
}

// TestTodoOnUpdateActions covers the live-update hook the run service uses
// to broadcast todoUpdate: action names ("created"/"updated") match the TS
// tool, and delivered snapshots are copies the tool cannot mutate later.
func TestTodoOnUpdateActions(t *testing.T) {
	type update struct {
		items  []types.TodoItem
		action string
	}
	var got []update
	todo := tools.NewTodoToolWithUpdate("", nil, func(items []types.TodoItem, action string) {
		got = append(got, update{items: items, action: action})
	})
	ctx := context.Background()
	exec := func(op string, extra map[string]any) {
		t.Helper()
		args := map[string]any{"op": op}
		for k, v := range extra {
			args[k] = v
		}
		if _, err := todo.Execute(ctx, helpers.MustJSONRaw(t, args)); err != nil {
			t.Fatalf("%s: %v", op, err)
		}
	}
	exec("init", map[string]any{"tasks": []string{"a"}})
	exec("append", map[string]any{"tasks": []string{"b"}})
	exec("start", map[string]any{"index": 1})
	exec("done", map[string]any{"index": 1})
	exec("view", nil)

	want := []string{"created", "created", "updated", "updated", "updated"}
	if len(got) != len(want) {
		t.Fatalf("expected %d updates, got %d", len(want), len(got))
	}
	for i, w := range want {
		if got[i].action != w {
			t.Fatalf("update %d: expected action %q, got %q", i, w, got[i].action)
		}
	}
	if len(got[0].items) != 1 || got[0].items[0].Content != "a" {
		t.Fatalf("init snapshot: %+v", got[0].items)
	}
	if len(got[4].items) != 2 || got[4].items[0].Status != "completed" {
		t.Fatalf("final snapshot: %+v", got[4].items)
	}
	// The init snapshot must still read pending even though item 1 was
	// completed afterwards — delivered slices are copies.
	if got[0].items[0].Status != "pending" {
		t.Fatalf("snapshot was mutated by later ops: %+v", got[0].items)
	}
}
