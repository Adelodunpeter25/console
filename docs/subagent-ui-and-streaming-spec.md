# Subagent Streaming & UI Flow Specification

**Status**: Draft  
**Applies to**: Server (`apps/server`), Desktop (`apps/desktop`), Mobile (`apps/mobile`)  
**Target Capabilities**: Real-Time Subagent Event Streaming, Inline Transcript Capsules, Workspace Tab vs. Inspector Views, Mobile Bottom Sheet Explorer

---

## 1. Overview & Problem Statement

Currently, when the primary agent spawns a subagent via the `subagent` tool:
1. The backend runs a nested `agentLoop`, but does not emit real-time subagent progress events over the SSE stream.
2. The primary client interfaces (Desktop and Mobile) only receive a generic, opaque tool execution block until the entire subagent run finishes and dumps a summary text block.
3. Users lack visibility into what tools the subagent is running, which files it is inspecting, its turn progression, or intermediate findings.

This specification defines the complete end-to-end architecture for real-time subagent streaming on the **Server**, followed by the **Desktop Dashboard** and **Mobile Client** UI flows.

---

## 2. Server Architecture & Streaming Protocol (`apps/server`)

### 2.1 Subagent Event Types (`@console/types` & `apps/server/agent/src/types/`)

Subagents emit discrete lifecycle events through the session's active `RunEventHub` so that any connected client (Desktop, Mobile, CLI) can track their execution in real time:

```typescript
export interface SubagentStartEvent {
  type: "subagentStart";
  subagentId: string;
  parentToolCallId: string;
  name: string;
  role: string;
  prompt: string;
  maxTurns: number;
}

export interface SubagentActivityEvent {
  type: "subagentActivity";
  subagentId: string;
  turnIndex: number;
  toolCallId: string;
  toolName: string;
  args?: Record<string, unknown>;
  status: "running" | "completed" | "error";
  error?: string;
}

export interface SubagentTurnEndEvent {
  type: "subagentTurnEnd";
  subagentId: string;
  turnIndex: number;
  thought?: string;
}

export interface SubagentEndEvent {
  type: "subagentEnd";
  subagentId: string;
  status: "completed" | "aborted" | "error";
  summary?: string;
  error?: string;
  totalTurns: number;
}

export type SubagentEvent =
  | SubagentStartEvent
  | SubagentActivityEvent
  | SubagentTurnEndEvent
  | SubagentEndEvent;
```

### 2.2 Server Tool Execution (`apps/server/agent/src/tools/subagent.ts`)

When `createSubagentTool` is executed within `RunService`:
1. It generates a unique `subagentId` (`subagent-${randomUUID()}`).
2. Broadcasts `subagentStart` over `RunEventHub`.
3. Passes an event-forwarding callback to the child `agentLoop`.
4. As the subagent invokes tools (`glob`, `grep`, `readFile`), it broadcasts `subagentActivity`.
5. On completion or cancellation, broadcasts `subagentEnd` with the final summary.

```typescript
// Example Server Event Flow:
hub.broadcast({
  type: "subagentStart",
  subagentId: "subagent-102",
  parentToolCallId: "call_9481",
  name: "Codebase Inspector",
  role: "Codebase Inspector",
  prompt: "Find all instances of database schemas across apps/server",
  maxTurns: 10,
});
```

---

## 3. Desktop UI Architecture (`apps/desktop`)

Desktop provides a multi-layered viewing experience: a compact inline card in the main chat, with deep-dive inspection options.

### 3.1 Inline Subagent Capsule Card (Chat Transcript)

Rendered inside the conversation transcript in place of a plain tool call box:

```
+-------------------------------------------------------------------------------+
| 🤖 [Codebase Inspector]  "Audit database migrations and schema"               |
|    ● Running (Turn 2/10) • 3 actions executed                         [ ⌄ ] [↗] |
+-------------------------------------------------------------------------------+
|  ▸ Live Activity:                                                             |
|    ✓ glob: "apps/server/agent/src/**/*.ts" (found 14 files)                   |
|    ✓ readFile: "schema.ts" (lines 80-120)                                     |
|    ⚡ grep: "session_todos" (in progress...)                                  |
+-------------------------------------------------------------------------------+
|  📋 Summary (upon completion):                                                |
|     "Found session_todos table defined in schema.ts with matching storage..." |
+-------------------------------------------------------------------------------+
```

#### Visual Elements:
- **Header**:
  - Subagent icon + Role Badge (e.g. `Codebase Inspector`, `Test Runner`).
  - Mission prompt.
  - Pulsing status indicator (`Running Turn 2/10` / `Completed`).
  - `[ ⌄ ]` Accordion toggle button.
  - `[ ↗ ]` Deep-dive action button.
- **Collapsible Activity Feed**: Compact list of tools invoked with execution status (pending, running, done).
- **Summary Box**: Rendered markdown summary once finished.

---

### 3.2 Desktop Deep-Dive Options (Under Evaluation)

The deep-dive exploration experience is designed around two options:

```
+-------------------------------------------------------------------------------+
|                          OPTION B: WORKSPACE TAB                              |
+-------------------------------------------------------------------------------+
| [Chat: Refactor DB]  [Subagent: Codebase Inspector]  [Terminal]               |
+------------------------------------+------------------------------------------+
| Main Chat Transcript               | Subagent Real-Time Transcript            |
| ...                                | Turn 1: Thought -> Glob "src/**/*.ts"    |
| primary agent waiting...           | Turn 2: Thought -> Read "schema.ts"      |
|                                    | Turn 3: Assistant Summary Result         |
+------------------------------------+------------------------------------------+
```

#### Option B: Dedicated Workspace Tab (`WorkspaceTabConfig::Subagent`)
- Clicking `[ ↗ ]` on the Subagent Card opens a dedicated tab in the current or split pane.
- Tab shows the subagent's full, independent message stream, tool calls, and thoughts.
- Can be split side-by-side (`Cmd+\`) with the main chat for real-time parallel monitoring.

```
+-------------------------------------------------------------------------------+
|                      OPTION C: RIGHT INSPECTOR DRAWER                         |
+-------------------------------------------------------------------------------+
| Main Chat Transcript               | INSPECTOR (Cmd+I)                        |
| ...                                | [Subagent: Codebase Inspector]           |
| 🤖 [Subagent Capsule Card]         | Status: Completed (3 turns)              |
|                                    |                                          |
|                                    | Messages & Tool Execution Tree:          |
|                                    | 1. user: "Audit schema..."               |
|                                    | 2. call: glob "src/**/*.ts"              |
|                                    | 3. call: readFile "schema.ts"            |
+------------------------------------+------------------------------------------+
```

#### Option C: Right Inspector Drawer Integration (`Cmd+I`)
- Clicking `[ ↗ ]` focuses the Right Inspector Drawer and switches its view to the selected Subagent Run.
- Keeps the main workspace focused on the primary conversation while allowing side-panel inspection.

---

## 4. Mobile Client Architecture (`apps/mobile`)

On mobile devices, vertical screen space is precious. Subagents use a **Compact Inline Card** in chat and expand into a full **Interactive Bottom Sheet**.

### 4.1 Chat Message Capsule
- Rendered in the mobile chat stream as a sleek dark pill/card.
- Shows agent role badge, truncated task prompt, and live status spinner.
- Tap gesture triggers the **Subagent Bottom Sheet**.

### 4.2 Subagent Bottom Sheet (`components/chat/subagent-bottom-sheet.tsx`)

```
+-------------------------------------------------------+
|  ━━━━ (Drag Handle)                                   |
|  🤖 Codebase Inspector                   [✕ Close]    |
|  "Audit database migrations and schema"               |
|  Status: Running (Turn 2/10)                          |
+-------------------------------------------------------+
|  ACTIVITY FEED:                                       |
|                                                       |
|  [✓] glob "apps/server/src/**/*.ts"                   |
|      Found 14 files                                   |
|                                                       |
|  [✓] readFile "schema.ts" (lines 80-120)              |
|                                                       |
|  [⚡] grep "session_todos"                             |
|      Searching codebase...                            |
|                                                       |
|  FINAL RESULT:                                        |
|  (Awaiting completion...)                             |
+-------------------------------------------------------+
```

#### Mobile Bottom Sheet Invariants:
1. **Snap Points**: `['40%', '85%']` using `@gorhom/bottom-sheet` or native bottom sheet modal.
2. **Real-Time Subscription**: Connects to the active chat session's SSE stream and updates the activity list with spring animations (`react-native-reanimated`).
3. **Copy & Share**: Quick button at the bottom to copy the subagent's findings to clipboard or insert into the composer input.

---

## 5. Implementation Roadmap

### Phase 1: Server Event Streaming
- [ ] Define `SubagentEvent` types in `@console/types`.
- [ ] Update `apps/server/agent/src/tools/subagent.ts` to forward `agentLoop` events to `RunEventHub`.
- [ ] Broadcast events over `/api/sessions/:id/run` SSE stream.

### Phase 2: Mobile Bottom Sheet Experience
- [ ] Create `apps/mobile/components/chat/subagent-card.tsx` in chat stream.
- [ ] Implement `apps/mobile/components/chat/subagent-bottom-sheet.tsx` with activity timeline and live turn indicator.

### Phase 3: Desktop UI & Deep-Dive
- [ ] Implement `subagent_card` in `apps/desktop/crates/console-ui/src/common/`.
- [ ] Implement selected Deep-Dive mode (Option B Tab vs. Option C Inspector) based on design selection.
