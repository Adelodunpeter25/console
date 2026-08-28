# Todo & Task List Integration Guide (Desktop & Mobile)

**Status**: Active Integration Reference  
**Applies to**: Desktop (`apps/desktop`), Mobile (`apps/mobile`), Server (`apps/server`)  
**Purpose**: Complete architectural and implementation guide for client agents building or modifying the interactive Todo checklist experience across Desktop and Mobile.

---

## 1. Overview & Agent Workflow

During complex multi-step tasks (feature development, large refactors, bug investigation), the AI agent uses its built-in `todo` tool to break instructions into actionable checklist items.

### The Agent's Todo Lifecycle
1. **`init`**: Agent declares the task breakdown (e.g., `tasks: ["Task 1", "Task 2", "Task 3"]`). All items start with `status: "pending"`.
2. **`start`**: Agent marks an item as actively being worked on (`status: "in_progress"`).
3. **`done`**: Agent marks the item as finished (`status: "completed"`).
4. **`append`**: Agent dynamically appends new follow-up tasks discovered mid-run.

Todos are **persisted in SQLite** on the server per session. If a turn completes with pending items, subsequent turns automatically inherit the existing todo list.

---

## 2. Server Data Models & Endpoints

### 2.1 Data Model (`@console/types`)

```typescript
export interface TodoItem {
  id: number;
  content: string;
  status: "pending" | "in_progress" | "completed";
}
```

### 2.2 REST API

#### `GET /api/sessions/:id/todos`
Fetches the current persisted task list for the session.

- **Response**:
  ```json
  {
    "success": true,
    "data": [
      { "id": 1, "content": "Define project scaffolding", "status": "completed" },
      { "id": 2, "content": "Implement API routes", "status": "in_progress" },
      { "id": 3, "content": "Write integration tests", "status": "pending" }
    ]
  }
  ```

### 2.3 Real-Time SSE Stream

During an active run (`/api/sessions/:id/run` or `/api/sessions/:id/run/stream`), the server emits `todoUpdate` events whenever the agent updates tasks:

```json
{
  "type": "todoUpdate",
  "items": [
    { "id": 1, "content": "Define project scaffolding", "status": "completed" },
    { "id": 2, "content": "Implement API routes", "status": "in_progress" },
    { "id": 3, "content": "Write integration tests", "status": "pending" }
  ],
  "action": "updated"
}
```

---

## 3. Desktop Client Integration (`apps/desktop`)

### 3.1 State Architecture

In `apps/desktop/src/state/`:
- `app.rs`:
  - `pub todo_items: HashMap<String, Vec<TodoItem>>` (keyed by `session_id`).
  - `pub todos_collapsed: HashMap<String, bool>` (keyed by `session_id`).
- `workspace_panes.rs`:
  - `todo_items_for_pane(&self, pane_id: &str) -> Vec<TodoItem>`
  - `is_todos_collapsed_for_pane(&self, pane_id: &str) -> bool`
  - `toggle_todos_collapsed_for_pane(&mut self, pane_id: &str)`
  - `set_todo_items_for_session(&mut self, session_id: &str, items: Vec<TodoItem>)`

### 3.2 SSE Event Hook

In `apps/desktop/src/state/run.rs`:
```rust
AgentSessionEvent::TodoUpdate { items, .. } => {
    self.set_todo_items_for_session(run_session_id, items);
    cx.notify();
}
```

### 3.3 UI Component (`crates/console-ui/src/common/todo_card.rs`)

The component is rendered directly above the composer in [`workspace_content.rs`](file:///Users/adelodunpeter/Developer/Projects/console/apps/desktop/src/view/workspace_content.rs):

```rust
todo_card(
    todo_items.clone(),
    collapsed,
    Some(Rc::new(move |_window, cx| {
        if let Some(app) = entity.upgrade() {
            app.update(cx, |this, cx| {
                this.toggle_todos_collapsed_for_pane(&pane_id);
                cx.notify();
            });
        }
    })),
    theme,
)
```

#### Visual Invariants for Desktop:
- **Background**: `theme.composer` (pure dark surface).
- **Surrounding Canvas**: `theme.chat_canvas` (`#000000`).
- **Header**:
  - `TASKS` label + active counter badge (e.g. `1/3`).
  - **Collapsed Chevron**: `ChevronUp` (`▲`) to indicate expanding upward.
  - **Expanded Chevron**: `ChevronDown` (`▼`) to indicate collapsing downward.
- **Collapsed Preview**: Single-line truncated text showing the **next active/pending item** after completed ones.
- **Checkboxes**:
  - `completed`: Green checkmark icon with subtle green tint and strikethrough text.
  - `in_progress`: Accent-colored active glowing dot.
  - `pending`: Crisp empty dark checkbox with subtle border.

---

## 4. Mobile Client Integration (`apps/mobile`)

### 4.1 State & Hook Setup

In `apps/mobile/hooks/use-chat-session.ts` (or relevant Zustand chat store):
1. **Hydration on Session Mount**:
   ```typescript
   useEffect(() => {
     if (!sessionId) return;
     api.get(`/api/sessions/${sessionId}/todos`).then((res) => {
       if (res.data?.success) {
         setTodos(res.data.data);
       }
     });
   }, [sessionId]);
   ```
2. **Real-Time Stream Subscription**:
   In the SSE event parser:
   ```typescript
   if (event.type === "todoUpdate") {
     setTodos(event.items);
   }
   ```

### 4.2 UI Component Guidelines for Mobile (`apps/mobile/components/chat/todo-banner.tsx`)

1. **Placement**: Docked floating right above the bottom chat composer pill.
2. **Animation**: Animate height and opacity using `react-native-reanimated` (`FadeInDown`, `FadeOutDown`, `LinearTransition`).
3. **Collapsible Behavior**:
   - Tapping the banner toggles between compact single-line mode (showing next task) and full sheet/card mode.
4. **Theme Alignment**:
   - Dark background matching mobile composer background (`#0A0A0A` / `#121212`).
   - Clean native touch targets (min 44px for chevron button).

---

## 5. Notes & Gotchas for Future Agents

> [!IMPORTANT]
> **1. Session Scoping**:
> Todos are strictly scoped per `sessionId`. Never store todos in a global single variable without indexing by `sessionId`. Panes and tabs must never leak one session's checklist onto another.

> [!NOTE]
> **2. Empty State Handling**:
> When `todo_items` is empty (`items.length === 0`), the UI card must be completely unmounted from the DOM / view tree. Do not render an empty card with "0/0".

> [!TIP]
> **3. Multi-Turn Continuity**:
> When an agent run finishes and a user submits a follow-up prompt, `RunService` automatically queries SQLite `session_todos` and passes them to `createTodoTool({ initialTodos: ... })`. The agent will be aware of remaining pending tasks and continue from where it left off.

> [!WARNING]
> **4. Type Matching**:
> Always use `TodoItem` from `@console/types`. Status values must be strictly `"pending" | "in_progress" | "completed"`.
