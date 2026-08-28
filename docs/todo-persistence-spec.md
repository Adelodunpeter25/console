# Session Todo & Task Persistence Specification

**Status**: Draft  
**Applies to**: Server (`apps/server`), Desktop (`apps/desktop`), Mobile (`apps/mobile`)  
**Target Capabilities**: SQLite Todo Storage, Cross-Turn Continuity, Multi-Device Task Sync

---

## 1. Overview & Objective

During complex tasks, the agent uses the `todo` tool to break down instructions into a list of actionable checklist items (`pending`, `in_progress`, `completed`).

### Problem
- Currently, [`RunService`](file:///Users/adelodunpeter/Developer/Projects/console/apps/server/api/src/services/run.service.ts) stores todo items in an ephemeral in-memory `Map<string, TodoItem[]>()`.
- When an agent turn finishes, when the app reloads, or when the server process restarts, the todo list is completely wiped from memory.
- In-progress and pending tasks are lost, preventing the agent from resuming multi-turn workflows or users from reviewing outstanding tasks.

### Objective
- Persist todos permanently in each session's dedicated SQLite database (`projects/<projectId>/sessions/<sessionId>.db`).
- Seed the agent's `todo` tool with existing persisted items at the start of every turn.
- Provide a REST endpoint (`GET /api/sessions/:id/todos`) so clients can fetch todos immediately upon loading a session.

---

## 2. Database Schema

In the per-session database schema ([`apps/server/agent/src/session/schema.ts`](file:///Users/adelodunpeter/Developer/Projects/console/apps/server/agent/src/session/schema.ts)), add the `session_todos` table:

```sql
CREATE TABLE IF NOT EXISTS session_todos (
  id INTEGER PRIMARY KEY,
  content TEXT NOT NULL,
  status TEXT NOT NULL CHECK (status IN ('pending', 'in_progress', 'completed')),
  updated_at INTEGER NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_session_todos_id ON session_todos(id ASC);
```

---

## 3. Storage Layer Operations

Add dedicated methods to [`SqliteSessionStorage`](file:///Users/adelodunpeter/Developer/Projects/console/apps/server/agent/src/session/storage.ts):

### 3.1 `saveSessionTodos(sessionId: string, items: readonly TodoItem[]): void`
Replaces the session's todo list in a single SQLite transaction:
```typescript
export function saveSessionTodos(
  state: StorageState,
  sessionId: string,
  items: readonly TodoItem[],
): void {
  const db = getSessionDb(state, sessionId);
  db.transaction(() => {
    db.run("DELETE FROM session_todos");
    const insert = db.prepare(
      "INSERT INTO session_todos (id, content, status, updated_at) VALUES (?1, ?2, ?3, ?4)"
    );
    const now = Date.now();
    for (const item of items) {
      insert.run(item.id, item.content, item.status, now);
    }
  })();
}
```

### 3.2 `getSessionTodos(sessionId: string): TodoItem[]`
Loads all todos ordered by `id ASC`:
```typescript
export function getSessionTodos(
  state: StorageState,
  sessionId: string,
): TodoItem[] {
  const db = getSessionDb(state, sessionId);
  const rows = db
    .prepare("SELECT id, content, status FROM session_todos ORDER BY id ASC")
    .all() as { id: number; content: string; status: string }[];

  return rows.map((r) => ({
    id: r.id,
    content: r.content,
    status: r.status as TodoItem["status"],
  }));
}
```

---

## 4. Run & Tool Lifecycle Integration

In [`RunService`](file:///Users/adelodunpeter/Developer/Projects/console/apps/server/api/src/services/run.service.ts):

1. **Initial Seeding**:
   When preparing a new run stream:
   ```typescript
   initialTodos: this.storage.getSessionTodos(sessionId)
   ```
2. **On-Tool Update Persistence**:
   Whenever `createTodoTool` fires the `onUpdate` callback:
   ```typescript
   onUpdate: (items, action) => {
     this.storage.saveSessionTodos(sessionId, items);
     hub.broadcast({ type: "todoUpdate", items, action });
   }
   ```
3. **Session Reset / Deletion**:
   When a session is cleared or deleted, `session_todos` is automatically cleaned up with the per-session SQLite database file.

---

## 5. REST & Real-Time API Specification

### 5.1 Endpoints

#### `GET /api/sessions/:id/todos`
- **Response**: `{ success: true, data: TodoItem[] }`
- **Description**: Returns the active list of tasks for the session.

#### `POST /api/sessions/:id/todos` (Optional Manual Edit)
- **Request Body**: `{ todos: TodoItem[] }`
- **Response**: `{ success: true, data: TodoItem[] }`
- **Description**: Allows manual client-side reordering or checking of items.

### 5.2 Real-Time SSE Events
Event broadcast on `/api/sessions/:id/run/stream`:
```json
{
  "type": "todoUpdate",
  "items": [
    { "id": 1, "content": "Define the main goal", "status": "completed" },
    { "id": 2, "content": "Complete the first action", "status": "in_progress" },
    { "id": 3, "content": "Review the result", "status": "pending" }
  ],
  "action": "updated"
}
```

---

## 6. Client Handling (Desktop & Mobile)

1. **Session Load**:
   - On opening a session tab, the client fetches `GET /api/sessions/:id/todos` and populates `todo_items`.
2. **Active Runs**:
   - Updates stream live over SSE (`todoUpdate`).
3. **UI Rendering**:
   - Rendered using the collapsible [`todo_card`](file:///Users/adelodunpeter/Developer/Projects/console/apps/desktop/crates/console-ui/src/common/todo_card.rs) component directly above the composer.
