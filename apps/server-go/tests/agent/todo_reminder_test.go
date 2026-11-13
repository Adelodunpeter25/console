// Todo reminders: a stale task list gets one trailing line in a tool result.
package tests

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/loop"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/permissions"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/tools"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/db"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/services"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/types"
	"github.com/Adelodunpeter25/console/apps/server-go/tests/helpers"
)

// runListDirRounds runs `rounds` tool rounds then finishes, returning every
// tool-result text the loop produced.
func runListDirRounds(t *testing.T, items []types.TodoItem, rounds int, reminders bool) []string {
	t.Helper()
	manager, err := db.Open(db.OpenOptions{Path: ":memory:"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(manager.Close)
	sessions := services.NewSessionService(manager)
	header, err := sessions.Create(types.CreateSessionOptions{Cwd: t.TempDir(), ModelID: "mock", Provider: "mock"})
	if err != nil {
		t.Fatal(err)
	}
	if items != nil {
		if err := sessions.SaveSessionTodos(header.ID, items); err != nil {
			t.Fatal(err)
		}
	}
	dir, _ := json.Marshal(map[string]string{"path": t.TempDir()})
	turns := []func() []loop.Event{}
	for i := 0; i < rounds; i++ {
		id := string(rune('a' + i))
		turns = append(turns, func() []loop.Event {
			return []loop.Event{{Kind: loop.EventToolCall, Call: &tools.ToolCall{ID: id, Name: "list_dir", Arguments: dir}}}
		})
	}
	turns = append(turns, func() []loop.Event { return []loop.Event{{Kind: loop.EventText, Text: "done"}} })

	registry := tools.NewRegistry(tools.DefaultTools()...)
	agent := loop.New(&helpers.MockProvider{Turns: turns}, loop.NewExecutor(registry, permissions.FullAccess), sessions)
	agent.TodoReminders = reminders
	events, err := agent.Run(context.Background(), header.ID, "go", registry.Definitions())
	if err != nil {
		t.Fatal(err)
	}
	var texts []string
	for {
		event, err, ok := events.Next()
		if !ok {
			if err != nil {
				t.Fatal(err)
			}
			break
		}
		if event.Kind == loop.EventToolResult && event.Result != nil {
			raw, _ := json.Marshal(event.Result.Content)
			texts = append(texts, string(raw))
		}
	}
	return texts
}

func countReminders(texts []string) int {
	n := 0
	for _, text := range texts {
		if strings.Contains(text, "Mark any finished with todo.") {
			n++
		}
	}
	return n
}

func TestTodoReminderFiresAfterFourStaleRounds(t *testing.T) {
	items := []types.TodoItem{
		{ID: 1, Content: "a", Status: "in_progress"},
		{ID: 2, Content: "b", Status: "pending"},
		{ID: 3, Content: "c", Status: "pending"},
	}
	texts := runListDirRounds(t, items, 8, true)
	if got := countReminders(texts); got != 2 {
		t.Fatalf("want 2 reminders over 8 rounds, got %d", got)
	}
	if !strings.Contains(texts[3], "Tasks: #1 in progress, 2 pending. Mark any finished with todo.") {
		t.Fatalf("fourth result: %s", texts[3])
	}
	if countReminders(texts[:3]) != 0 {
		t.Fatal("reminder fired before four rounds")
	}
}

func TestTodoReminderQuietWhenNothingOpenOrDisabled(t *testing.T) {
	done := []types.TodoItem{{ID: 1, Content: "a", Status: "completed"}}
	if got := countReminders(runListDirRounds(t, done, 6, true)); got != 0 {
		t.Fatalf("finished list reminded %d times", got)
	}
	if got := countReminders(runListDirRounds(t, nil, 6, true)); got != 0 {
		t.Fatalf("empty list reminded %d times", got)
	}
	open := []types.TodoItem{{ID: 1, Content: "a", Status: "pending"}}
	if got := countReminders(runListDirRounds(t, open, 6, false)); got != 0 {
		t.Fatalf("disabled reminders fired %d times", got)
	}
}
