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
)

func TestTodoUnboundLifecycle(t *testing.T) {
	todo := tools.NewTodoTool("", nil)

	initArgs, _ := json.Marshal(map[string]any{"op": "init", "tasks": []string{"first", "second"}})
	out, err := todo.Execute(context.Background(), initArgs)
	if err != nil {
		t.Fatalf("init: %v", err)
	}
	s, _ := out.(string)
	if !strings.Contains(s, "#1: first (pending)") || !strings.Contains(s, "#2: second (pending)") {
		t.Fatalf("init output: %v", out)
	}

	startArgs, _ := json.Marshal(map[string]any{"op": "start", "index": 1})
	out, err = todo.Execute(context.Background(), startArgs)
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	s, _ = out.(string)
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
	s, _ = out.(string)
	if !strings.Contains(s, "#3: third (pending)") {
		t.Fatalf("append output: %v", out)
	}

	viewArgs, _ := json.Marshal(map[string]any{"op": "view"})
	out, err = todo.Execute(context.Background(), viewArgs)
	if err != nil {
		t.Fatalf("view: %v", err)
	}
	s, _ = out.(string)
	if !strings.Contains(s, "#1: first (completed)") {
		t.Fatalf("view output: %v", out)
	}

	// Missing index / tasks -> tool errors.
	if _, err := todo.Execute(context.Background(), mustJSONRaw(t, map[string]any{"op": "start"})); err == nil {
		t.Fatal("expected error for missing index")
	}
	if _, err := todo.Execute(context.Background(), mustJSONRaw(t, map[string]any{"op": "init"})); err == nil {
		t.Fatal("expected error for missing tasks")
	}
	if _, err := todo.Execute(context.Background(), mustJSONRaw(t, map[string]any{"op": "done", "index": 999})); err == nil {
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
	s, _ := out.(string)
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
