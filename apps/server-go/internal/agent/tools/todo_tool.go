// todo tool: session-scoped task list, persisted to the session's
// session_todos table (survives restarts and session reload). Port of
// apps/server/agent/src/tools/todo.ts + agent/src/session/session-todos.ts.
package tools

import (
	"context"
	"fmt"
	"sync"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/types"
)

// TodoStore is the persistence seam the tool writes through — satisfied by
// *services.SessionService (kept as an interface here to avoid tools
// depending on the concrete session storage implementation).
type TodoStore interface {
	GetSessionTodos(sessionID string) ([]types.TodoItem, error)
	SaveSessionTodos(sessionID string, items []types.TodoItem) error
}

type todoInput struct {
	Op    string   `json:"op" jsonschema:"required,description=Operation to apply: 'init'\\, 'start'\\, 'done'\\, 'append'\\, or 'view'"`
	Tasks []string `json:"tasks,omitempty" jsonschema:"description=List of task item strings (used with 'init' or 'append')"`
	Index int      `json:"index,omitempty" jsonschema:"description=Task ID/index (1-indexed) to update status for ('start' or 'done')"`
}

func renderTodoList(title string, items []types.TodoItem) []map[string]any {
	if len(items) == 0 {
		return textResult(fmt.Sprintf("%s:\n(No active tasks in TODO list)", title))
	}
	out := title + ":\n"
	for i, item := range items {
		icon := "[ ]"
		switch item.Status {
		case "completed":
			icon = "[x]"
		case "in_progress":
			icon = "[>]"
		}
		if i > 0 {
			out += "\n"
		}
		out += fmt.Sprintf("%s #%d: %s (%s)", icon, item.ID, item.Content, item.Status)
	}
	return textResult(out)
}

// NewTodoTool builds a todo tool bound to one session: reads/writes persist
// through store. A nil store (used by the DefaultTools() singleton, which
// has no session context) falls back to in-memory-only state for the
// process lifetime — mirrors TS's unbound `createTodoTool()` default.
// TodoUpdateHandler mirrors the TS TodoUpdateHandler: fired with a snapshot
// copy after every op ("created" for init/append, "updated" for
// start/done/view). The run service uses it to broadcast todoUpdate.
type TodoUpdateHandler func(items []types.TodoItem, action string)

func NewTodoTool(sessionID string, store TodoStore) Tool {
	return NewTodoToolWithUpdate(sessionID, store, nil)
}

// NewTodoToolWithUpdate builds a session-bound todo tool that also reports
// every mutation through onUpdate (nil disables reporting).
func NewTodoToolWithUpdate(sessionID string, store TodoStore, onUpdate TodoUpdateHandler) Tool {
	var mu sync.Mutex
	var items []types.TodoItem
	loaded := false

	load := func() {
		if loaded || store == nil {
			return
		}
		if existing, err := store.GetSessionTodos(sessionID); err == nil {
			items = existing
		}
		loaded = true
	}
	persist := func() {
		if store == nil {
			return
		}
		snapshot := make([]types.TodoItem, len(items))
		copy(snapshot, items)
		_ = store.SaveSessionTodos(sessionID, snapshot)
	}
	publish := func(action string) {
		if onUpdate == nil {
			return
		}
		snapshot := make([]types.TodoItem, len(items))
		copy(snapshot, items)
		onUpdate(snapshot, action)
	}

	return NewTool("todo", "Manage the session task list for multi-step work. Operations: init, start, done, append, view.", TierRead,
		func(ctx context.Context, in todoInput) (any, error) {
			mu.Lock()
			defer mu.Unlock()
			load()

			switch in.Op {
			case "init":
				if len(in.Tasks) == 0 {
					return nil, NewToolError("'init' operation requires a non-empty 'tasks' array.")
				}
				items = make([]types.TodoItem, len(in.Tasks))
				for i, content := range in.Tasks {
					items[i] = types.TodoItem{ID: i + 1, Content: content, Status: "pending"}
				}
				persist()
				publish("created")
				return renderTodoList("Initialized task list", items), nil

			case "append":
				if len(in.Tasks) == 0 {
					return nil, NewToolError("'append' operation requires a non-empty 'tasks' array.")
				}
				startID := len(items) + 1
				for i, content := range in.Tasks {
					items = append(items, types.TodoItem{ID: startID + i, Content: content, Status: "pending"})
				}
				persist()
				publish("created")
				return renderTodoList("Appended tasks", items), nil

			case "start", "done":
				if in.Index == 0 {
					return nil, NewToolError("'%s' operation requires task 'index'.", in.Op)
				}
				found := false
				for i := range items {
					if items[i].ID == in.Index {
						if in.Op == "done" {
							items[i].Status = "completed"
						} else {
							items[i].Status = "in_progress"
						}
						found = true
						break
					}
				}
				if !found {
					return nil, NewToolError("Task index %d not found.", in.Index)
				}
				persist()
				publish("updated")
				return renderTodoList(fmt.Sprintf("Marked task #%d as %s", in.Index, statusOf(items, in.Index)), items), nil

			default: // "view" or unspecified
				publish("updated")
				return renderTodoList("Current task list status", items), nil
			}
		})
}

func statusOf(items []types.TodoItem, id int) string {
	for _, item := range items {
		if item.ID == id {
			return item.Status
		}
	}
	return ""
}

// Todo is the unbound default instance used by DefaultTools() (in-memory
// only; a real run binds its own via NewTodoTool with the session store).
var Todo = NewTodoTool("", nil)
