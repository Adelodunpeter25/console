# Subagent Streaming & Multi-Platform UI Specification

**Status**: Finalized Design  
**Applies to**: Server (`apps/server`), Desktop (`apps/desktop`), Mobile (`apps/mobile`)  
**Target Capabilities**: Real-Time Subagent Streaming, Right Panel Subagents Tab with Accordion & Badges, Mobile Bottom Sheet

---

## 1. Overview & Problem Statement

When the primary agent spawns child subagents (e.g. for codebase inspection, test verification, or deep research), users need real-time visibility into what the subagents are doing without:
1. Cluttering the main chat stream with hundreds of nested tool lines.
2. Opening dozens of separate workspace tabs that must be manually closed.
3. Feeling blind to what tools, files, and queries the subagents are executing.

This specification defines the complete end-to-end architecture across the **Server**, **Desktop Dashboard**, and **Mobile App**.

---

## 2. Server Architecture & Streaming Protocol (`apps/server`)

### 2.1 Subagent Event Types (`@console/types` & `apps/server/agent/src/types/`)

Subagents emit discrete real-time events over the session's active `RunEventHub`:

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
  | SubagentEndEvent;
```

### 2.2 Execution in `apps/server/agent/src/tools/subagent.ts`

1. Creates a unique `subagentId` (`subagent-${randomUUID()}`).
2. Emits `subagentStart` immediately.
3. Attaches an event listener to the child `agentLoop` to broadcast `subagentActivity` as the child calls `glob`, `grep`, `readFile`, etc.
4. On loop settlement, broadcasts `subagentEnd` with the final markdown summary.

---

## 3. Desktop UI Architecture (`apps/desktop`)

Desktop uses a unified **Right Panel `Subagents` Tab** paired with an **In-Chat Capsule**.

### 3.1 Right Panel `Subagents` Tab (`RightPanelTab::Subagents`)

The Right Panel header adds a third tab alongside `All` and `Changes`, featuring a **dynamic badge counter**:

```
+-------------------------------------------------------------------------------+
| RIGHT PANEL (Cmd+I)                                                           |
| [ All ]        [ Changes (4) ]        [ Subagents (2) ]                       |
+-------------------------------------------------------------------------------+
| 🤖 Codebase Inspector                                               ● Running |
|    "Audit database migrations and schema"                         (Turn 2/10) |
|    ▾ Details                                                                  |
|      • Task Prompt: "Audit database migrations and schema..."                 |
|      • Activity Timeline:                                                     |
|        ✓ glob: "apps/server/agent/src/**/*.ts" (14 files found)               |
|        ✓ readFile: "schema.ts" (lines 80-120)                                 |
|        ⚡ grep: "session_todos" (in progress...)                              |
|      • Summary: (Awaiting completion...)                                      |
+-------------------------------------------------------------------------------+
| ✓ Test Runner                                                          • Done |
|    "Verify bun tests pass across server"                                      |
|    ▸ Details                                                                  |
+-------------------------------------------------------------------------------+
```

#### Key Features:
1. **Dynamic Tab Badge**:
   - Displays total subagent count for the active session (e.g. `Subagents (2)`).
   - If any subagents are actively running, the badge pulses with an active green accent indicator.
2. **Accordion-Style List Items**:
   - **Collapsed State**: Displays subagent icon, role badge, prompt snippet, status pill (`● Running (Turn 2/10)`, `✓ Done`, `✗ Failed`), and chevron toggle (`▸`).
   - **Expanded State (`▾ Details`)**:
     - Full Mission Prompt.
     - Step-by-step Activity Timeline (tools executed with file paths and match counts).
     - Rendered Markdown Summary once completed.
3. **Empty State**:
   - When no subagents have run in the current session, displays a clean placeholder:
     ```
     +-------------------------------------------------------+
     |                    🤖                                 |
     |           No Subagents Spawned                        |
     |   Subagents spawned by the assistant will appear here |
     +-------------------------------------------------------+
     ```

---

### 3.2 In-Transcript Subagent Capsule & Linkage

In the main chat transcript, the subagent tool call is displayed as a sleek capsule with a **`[ View in Panel → ]`** action button:

```
+-------------------------------------------------------------------------------+
| 🤖 [Codebase Inspector]  "Audit database migrations and schema"               |
|    ● Running (Turn 2/10) • 3 actions executed              [ View in Panel → ] |
+-------------------------------------------------------------------------------+
```

- Clicking `[ View in Panel → ]`:
  1. Opens the Right Panel (if closed).
  2. Switches the active tab to `Subagents`.
  3. Automatically scrolls to and expands that specific subagent's accordion.

---

## 4. Mobile Client Architecture (`apps/mobile`)

On mobile, subagents are viewed via an **Interactive Bottom Sheet**.

### 4.1 Chat Message Capsule
- Rendered in the chat stream as an interactive dark pill/card showing the subagent role badge, prompt snippet, and live status spinner.
- Tapping the capsule triggers the **Subagent Bottom Sheet**.

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
|  FINAL SUMMARY:                                       |
|  (Awaiting completion...)                             |
|                                                       |
|  [ 📋 Copy Summary ]                                  |
+-------------------------------------------------------+
```

#### Mobile Invariants:
1. **Snap Points**: `['45%', '85%']` via bottom sheet modal.
2. **Reanimated Transitions**: Live tool steps animate in smoothly as SSE events arrive.
3. **Copy Action**: Button to copy the subagent's final output to clipboard or paste into the composer.

---

## 5. Implementation Checklist

- [ ] **Phase 1: Server Event Protocol (`apps/server`)**:
  - Define `SubagentEvent` interfaces in `@console/types`.
  - Update `apps/server/agent/src/tools/subagent.ts` to emit real-time lifecycle events (`subagentStart`, `subagentActivity`, `subagentEnd`) to `RunEventHub`.
- [ ] **Phase 2: Desktop Right Panel `Subagents` Tab (`apps/desktop`)**:
  - Add `RightPanelTab::Subagents` in `apps/desktop/src/state/`.
  - Implement badge counter and `subagent_list_view` in `crates/console-ui/src/right_sidebar/`.
  - Add accordion toggle state and empty state view.
  - Wire `[ View in Panel → ]` click handler in transcript capsule.
- [ ] **Phase 3: Mobile Bottom Sheet (`apps/mobile`)**:
  - Implement `subagent-card.tsx` for chat messages.
  - Implement `subagent-bottom-sheet.tsx` with activity timeline and copy action.
