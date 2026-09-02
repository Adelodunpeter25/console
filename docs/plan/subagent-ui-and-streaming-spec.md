# Subagent Streaming & Multi-Platform UI Specification

**Status**: Finalized Design  
**Applies to**: Server (`apps/server`), Desktop (`apps/desktop`), Mobile (`apps/mobile`)  
**Target Capabilities**: Real-Time Subagent Streaming, Right Panel Subagents Tab with Accordion & Badges, Mobile Floating Banner & Dedicated Subagent Screens

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

On Mobile, subagents follow the same high-polish floating banner pattern as the Todo Banner, navigating into dedicated screen views for list browsing and deep-dive inspection.

### 4.1 Floating Subagent Banner (`components/chat/subagent-banner.tsx`)

Docked floating directly above the Composer / Interaction Panel in `ChatScreen` (beside or stacked with `TodoBanner`):

```
+-------------------------------------------------------------------------------+
| 🤖 SUBAGENTS [ 2 ] • Codebase Inspector (Running)                         [→] |
+-------------------------------------------------------------------------------+
```

- **Visuals**: Dark `#121214` surface, `#27272a` border, rounded-xl styling.
- **Left Icon & Title**: Bot icon with `SUBAGENTS` header and active counter badge (`[ 2 ]`).
- **Preview Snippet**: Name and live status of the latest running or completed subagent.
- **Action**: Tapping the banner navigates to the **Subagents List Screen**.

---

### 4.2 Subagents List Screen (`screens/subagents/subagents-screen.tsx`)

A dedicated screen listing all subagents created during the active chat session:

```
+-------------------------------------------------------+
|  [← Back]               Subagents (2)                 |
+-------------------------------------------------------+
|                                                       |
|  +-------------------------------------------------+  |
|  | 🤖 Codebase Inspector                 ● Running |  |
|  |    "Audit database migrations and schema"       |  |
|  |    Turn 2/10 • 3 tools executed              [→]|  |
|  +-------------------------------------------------+  |
|                                                       |
|  +-------------------------------------------------+  |
|  | ✓ Test Runner                            • Done |  |
|  |    "Verify bun tests across apps/server"        |  |
|  |    Completed in 3 turns                      [→]|  |
|  +-------------------------------------------------+  |
|                                                       |
+-------------------------------------------------------+
```

#### Features:
1. **ScreenHeader**: Standard navigation header with back button (`[←]`) returning to `ChatScreen`.
2. **List Cards**:
   - Subagent Name & Role badge.
   - Truncated prompt description.
   - Status pill (`● Running (Turn 2/10)` with pulsing blue/cyan dot, `✓ Done` with green pill, `✗ Failed` with red pill).
   - Chevron / Arrow (`[→]`) indicating navigation.
3. **Empty State**:
   - Displays a clean placeholder if no subagents have been spawned in the session.
4. **Navigation Action**: Tapping any card navigates to the **Subagent Details Screen**.

---

### 4.3 Subagent Details Screen (`screens/subagents/subagent-details-screen.tsx`)

The deep-dive inspector screen for a specific subagent run:

```
+-------------------------------------------------------+
|  [← Back]            Codebase Inspector               |
+-------------------------------------------------------+
|                                                       |
|  MISSION PROMPT                                       |
|  "Audit database migrations and schema across         |
|   apps/server to verify session_todos table"          |
|                                                       |
|  STATUS & TURNS                                       |
|  ● Running • Turn 2 of 10                             |
|                                                       |
|  ACTIVITY TIMELINE                                    |
|  [✓] glob "apps/server/src/**/*.ts"                   |
|      Found 14 matching files                          |
|                                                       |
|  [✓] readFile "schema.ts" (lines 80-120)              |
|                                                       |
|  [⚡] grep "session_todos"                             |
|      Searching codebase in progress...                |
|                                                       |
|  FINAL SUMMARY                                        |
|  (Awaiting completion...)                             |
|                                                       |
|  [ 📋 Copy Summary ]                                  |
+-------------------------------------------------------+
```

#### Features:
1. **ScreenHeader**: Back button returning to the Subagents List screen.
2. **Mission Prompt Section**: Full original instruction provided by the primary agent.
3. **Live Activity Timeline**: Real-time list of tool executions (`glob`, `readFile`, `grep`) with status indicators, argument summaries, and file match counts.
4. **Markdown Summary**: Full rendered GitHub-flavored Markdown summary returned upon subagent completion.
5. **Action Buttons**: One-tap button to copy summary to clipboard or share.

---

## 5. Implementation Roadmap

### Phase 1: Server Event Protocol (`apps/server`)
- [ ] Define `SubagentEvent` interfaces in `@console/types`.
- [ ] Update `apps/server/agent/src/tools/subagent.ts` to emit real-time lifecycle events (`subagentStart`, `subagentActivity`, `subagentEnd`) to `RunEventHub`.
- [ ] Forward subagent events over the active SSE stream.

### Phase 2: Desktop Right Panel `Subagents` Tab (`apps/desktop`)
- [ ] Add `RightPanelTab::Subagents` in `apps/desktop/src/state/`.
- [ ] Implement badge counter and `subagent_list_view` in `crates/console-ui/src/right_sidebar/`.
- [ ] Add accordion toggle state and empty state view.
- [ ] Wire `[ View in Panel → ]` click handler in transcript capsule.

### Phase 3: Mobile Banner & Subagent Screens (`apps/mobile`)
- [ ] Create `useSessionSubagents` hook to track subagent state and live SSE events.
- [ ] Create `components/chat/subagent-banner.tsx` floating in `ChatScreen`.
- [ ] Implement `screens/subagents/subagents-screen.tsx` (Subagents List).
- [ ] Implement `screens/subagents/subagent-details-screen.tsx` (Subagent Details & Activity Timeline).
