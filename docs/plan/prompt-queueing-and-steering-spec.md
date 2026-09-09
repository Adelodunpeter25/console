# Prompt Queueing & Steering Specification

**Status**: Draft (reconciled against current codebase)
**Applies to**: Desktop (`apps/desktop`), Mobile (`apps/mobile`), Server API (`apps/server`)
**Target Capabilities**: Real-Time Agent Collaboration, Turn Orchestration, Mid-Flight Steering

> This revision replaces the original draft's server/event-model sections with designs that match the
> real `RunService`, `AgentSessionEvent`, and `ComposerRunState` implementations in this repo, rather than
> inventing parallel types and endpoints. UX intent (sections 1–2) is unchanged.

---

## 1. Overview & Problem Statement

When an AI agent is executing a turn (researching, editing files, running commands), users frequently think of follow-up requests, refinements, or course corrections.

Currently:
1. When an agent is running, the composer action button shows **Stop** (`■`).
2. If the user types into the composer during a run, they cannot easily queue the message to run automatically once the agent finishes.
3. If the user realizes the agent took the wrong approach, they must manually click Stop, wait for termination, and then send a new prompt.

### Goals
- **Seamless Prompt Queueing**: Allow users to type and submit a follow-up prompt while a turn is active. The prompt is staged in a dedicated **Queued Prompt Card** directly above the composer.
- **Auto-Execution**: Once the active turn finishes successfully, the queued prompt is automatically dispatched as the next turn without requiring user intervention.
- **Three Core Queued Actions**:
  1. **Edit**: Pulls the queued prompt text and attachments back down into the composer input for editing (removing it from the queue).
  2. **Delete**: Discards the queued prompt completely.
  3. **Steer / Send Now**: Immediately halts the current turn and dispatches the queued prompt right away, steering the agent with the new instructions.
- **Multi-Device Synchronization**: Queued state is persisted on the backend and broadcast via SSE so Desktop and Mobile clients stay synchronized in real time.

---

## 2. User Experience & Visual Design

```
+-----------------------------------------------------------------------+
|  Agent Transcript / Tool Runs (Active Turn)                           |
|  ...                                                                  |
|  [Tool: editFile src/app.tsx] -> Success                              |
+-----------------------------------------------------------------------+
|                                                                       |
|  +-----------------------------------------------------------------+  |
|  | ⏳ "Also make sure to update the unit tests in app.test.tx..."  |  |
|  |                                              [✏️] [🗑️] [⚡ Steer] |  |
|  +-----------------------------------------------------------------+  |
|                                                                       |
|  +-----------------------------------------------------------------+  |
|  | (+) Message...                                [Claude 3.7 ⌵] [↑] |  |
|  +-----------------------------------------------------------------+  |
|  ⑂ main   📁 project-name ⌵                          🔒 Always Ask ⌵ |
+-----------------------------------------------------------------------+
```

### 2.1 Composer Button Dynamic States

| State | Composer Content | Agent Status | Action Button Rendered | Action on Click | Keyboard Shortcut |
|---|---|---|---|---|---|
| **Idle Ready** | Empty | Idle | Arrow Up (dimmed) | Disabled | - |
| **Idle Ready** | Non-empty | Idle | Arrow Up (active) | Run new turn | `Enter` |
| **Running** | Empty | Running | Stop Button (`■`) | Abort active turn | `Esc` / `⌘.` |
| **Running** | Non-empty | Running | **Queue Button (`↑`)** | **Queue prompt** | `Enter` |
| **Queued + Running** | Non-empty | Running | Disabled / Replace Queue | Update queue | `Enter` |

> [!NOTE]
> When the composer is empty during a run, the primary button is **Stop** (`■`). As soon as the user begins typing a follow-up, the button transitions into a **Queue** button (`↑`). If the user deletes their text, it smoothly reverts to the **Stop** button.
>
> This state is orthogonal to whether a prompt is already queued (see §5.2): "has queued prompt" and
> "run is active" are tracked independently, and the button/table above only concerns the latter plus
> current composer content.

### 2.2 The Queued Prompt Card (Single Compact Row)

The Queued Prompt Card is an ultra-compact, single-line strip (`h(32px)`, `rounded(8px)`, `bg(theme.composer)` / `border_strong`) docked directly above the composer input:

- **Left Content (Single Truncated Line)**:
  - Subtle `⏳` icon or `Queued:` label.
  - The prompt text truncated with ellipsis (`truncate()`) before reaching the action buttons on the right.
- **Right Action Buttons (3 Compact Controls)**:
  1. **Edit (`✏️`)**: Clears the queue from the server and restores the prompt text & attachments into the active composer input.
  2. **Delete (`🗑️` / `✕`)**: Discards the queued prompt completely.
  3. **Steer / Send Now (`⚡` / `↑`)**: Halts the active agent turn immediately and starts this queued prompt as the new turn.

---

## 3. Architecture & Data Flow

Both "auto-pop on completion" and "steer now" are the **same underlying mechanism**: a single
per-session `pendingNextTurn` slot that `RunService` drains from one place — the `finally` block of the
run it's currently executing. Steering just aborts the current run early so that drain point is reached
sooner. There is no separate "start turn 2" call racing the first run's teardown.

```mermaid
sequenceDiagram
    autonumber
    actor User
    participant Client as Desktop / Mobile Client
    participant API as Server Run & Queue API
    participant Run as RunService (per-session)

    User->>Client: Types follow-up while Turn 1 is running
    Client->>Client: Composer shows Queue [↑] button
    User->>Client: Clicks Queue / presses Enter
    Client->>API: POST /api/sessions/:id/queue { prompt, attachments }
    API->>Run: setPendingNextTurn(sessionId, dto)  (persisted + in-memory)
    API-->>Client: SSE: queueUpdated { queuedPrompt }
    Client->>Client: Renders Queued Prompt Card above composer

    alt Normal Completion (Auto-Drain)
        Run->>Run: Turn 1 settles (existing finally block in runAgentStreamInternal)
        Run->>Run: pendingNextTurn present -> clear slot, broadcast queueUpdated(null)
        Run->>Run: fire-and-forget runAgentStream(sessionId, pendingNextTurn) for Turn 2
        Run-->>Client: SSE: turnStart { prompt } for Turn 2 (existing event, no new type)
        Client->>Client: Dismiss Queued Prompt Card & stream Turn 2
    else User Steers Mid-Flight (Send Now)
        User->>Client: Clicks "Steer Now"
        Client->>API: POST /api/sessions/:id/steer { queueId }
        API->>Run: setPendingNextTurn(sessionId, dto); abortRun(sessionId)
        Run->>Run: Turn 1's existing abort handling persists partial tools/messages
        Run->>Run: finally block sees pendingNextTurn -> drains it exactly as above
        Run-->>Client: SSE: queueUpdated { queuedPrompt: null }, turnStart for Turn 2
    else User Edits Queued Prompt
        User->>Client: Clicks "Edit"
        Client->>API: DELETE /api/sessions/:id/queue
        API->>Run: clearPendingNextTurn(sessionId)
        API-->>Client: SSE: queueUpdated { queuedPrompt: null }
        Client->>Client: Loads prompt text & attachments into Composer Input
    end
```

---

## 4. Backend Server Specification (`apps/server`)

### 4.1 Data Model

```typescript
export interface QueuedPrompt {
  id: string;
  sessionId: string;
  prompt: string;
  attachments?: ImageAttachment[];
  modelId?: string;
  provider?: string;
  approvalMode?: string;
  createdAt: string; // ISO timestamp
}
```

Persisted the same way `session-todos.ts` persists todos: one row per session via `sessionStorage`
(SQLite), mirrored into an in-memory `Map<sessionId, QueuedPrompt>` on `RunService` for fast access
during a run. This satisfies both same-server SSE sync and reconnect-after-restart (§7.3) without a
separate "Session Queue Store" abstraction.

### 4.2 REST Endpoints

#### `POST /api/sessions/:id/queue`
Adds or replaces the queued prompt for the session.
- **Request Body**: `RunPromptDto`
- **Response**: `{ success: true, data: QueuedPrompt }`
- **Behavior**: persists the prompt, sets `RunService`'s in-memory `pendingNextTurn` slot for the
  session, and broadcasts `queueUpdated` on that session's `RunEventHub` (a no-op if no run/hub is
  active — the client still sees the value via `GET`).

#### `GET /api/sessions/:id/queue`
Fetches the current queued prompt for the session (if any).
- **Response**: `{ success: true, data: QueuedPrompt | null }`

#### `DELETE /api/sessions/:id/queue`
Deletes the queued prompt for the session (used by both the "Delete" and "Edit" card actions; "Edit"
additionally reads the value client-side via `GET` before deleting).
- **Response**: `{ success: true, data: { deleted: true } }`
- **Emits Event**: `queueUpdated` with `queuedPrompt: null`.

#### `POST /api/sessions/:id/steer`
Halts the active run and arranges for the given prompt to start as the next turn as soon as the current
run settles.
- **Request Body**: `RunPromptDto` (same shape as `/queue`; a client typically sends the queued
  prompt's own fields, but steer does not require a prompt to have been queued first)
- **Response**: `{ success: true, data: { steered: true } }`
- **Behavior**:
  1. Sets `pendingNextTurn` for the session to the request body (persisted + in-memory), overwriting any
     existing queued prompt.
  2. Calls the existing `runService.abortRun(sessionId)`. Returns `404` if there is no active run — the
     caller should use `POST /sessions/:id/run` directly in that case.
  3. Returns immediately; it does **not** itself call `runAgentStream`. Turn 2 starts once the aborted
     run's own `finally` block (see §4.4) drains `pendingNextTurn`, matching the auto-pop path exactly.

### 4.3 SSE Event Stream Extensions

The server's real-time event stream (`/api/sessions/:id/run/stream`) gains exactly one new frame type,
added to the existing `AgentSessionEvent` union in `packages/types/src/events.ts` (and mirrored in
`apps/desktop/crates/console-core/src/types/events.rs` in the same change — these two are hand-written,
not generated from each other):

```typescript
export type AgentSessionEvent =
  | // ...all existing variants unchanged (sessionStart, turnStart, turnEnd,
    // modelStreamStart/Part/End, toolExecutionStart/Result/End, permissionRequest,
    // askQuestion, todoUpdate, compaction, sessionEnd, error, subagent*, done, aborted, streamReset)
  | { type: "queueUpdated"; queuedPrompt: QueuedPrompt | null };
```

No `runStarted` or `turnFinished` events are introduced. Turn 2 starting is communicated with the
existing `turnStart { prompt }` event; turn settlement (success, error, or abort) is communicated with
the existing `done` / `aborted` / `error` events. This keeps every current exhaustive `match` over
`AgentSessionEvent` — in `apps/desktop/src/state/run.rs`, `apps/mobile/utils/chat-events.ts`, and the Rust
enum — limited to one new arm instead of several.

### 4.4 Drain Point: `RunService` Internals

- Add `private pendingNextTurn = new Map<string, RunPromptDto>()` to `RunService`, parallel to the
  existing `private static activeRuns = new Map<string, AbortController>()`.
- `queuePrompt(sessionId, dto)`: persists + sets `pendingNextTurn`, broadcasts `queueUpdated` on the
  session's hub if one exists.
- `clearQueuedPrompt(sessionId)`: persists + clears `pendingNextTurn`, broadcasts `queueUpdated(null)`.
- `steer(sessionId, dto)`: sets `pendingNextTurn` to `dto`, then calls the existing `abortRun(sessionId)`.
- In `runAgentStreamInternal`'s existing `finally` block, immediately **before** the line that deletes
  `RunService.activeRuns.get(sessionId)` (in the outer `runAgentStream` method, not
  `runAgentStreamInternal`, which is where that deletion actually happens today), check
  `pendingNextTurn.get(sessionId)`. If present: delete it, broadcast `queueUpdated(null)`, and
  fire-and-forget a new `this.runAgentStream(sessionId, nextDto, onEvent)` call reusing the same
  `onEvent` callback chain (so SSE subscribers keep receiving frames for Turn 2 without reconnecting).
  This preserves the documented invariant that `activeRuns` cleanup and the next run's start happen from
  the same synchronous section, so a client's steer request arriving at the exact moment a run settles
  cannot start two runs — the Map itself is the lock, no separate mutex primitive is needed.

---

## 5. Desktop Implementation (`apps/desktop`)

### 5.1 Component Structure

1. **`QueuedPromptCard`** (`crates/console-ui/src/common/queued_prompt_card.rs`):
   - Standalone GPUI component rendered above `ComposerView`.
   - Props:
     - `queued_prompt: Option<QueuedPrompt>`
     - `on_edit: Rc<dyn Fn(&mut Window, &mut App)>`
     - `on_delete: Rc<dyn Fn(&mut Window, &mut App)>`
     - `on_steer: Rc<dyn Fn(&mut Window, &mut App)>`
2. **`ComposerView` Updates** (`crates/console-ui/src/common/composer_view.rs`):
   - `ComposerRunState` stays `Ready | Preparing | Running` — "queued" is not a run state (a queue can
     exist independently of what's currently typed). Add a separate `has_queued_prompt: bool` prop to
     `ComposerView` instead of a new enum variant.
   - Button variant is derived from the triple `(run_state, composer_input.is_empty(), has_queued_prompt)`:
     - `run_state.is_running()` and input non-empty → `↑ Queue` (`btn-queue-follow-up`), regardless of
       `has_queued_prompt` (submitting again replaces the existing queued prompt — see §2.1's "Queued +
       Running" row).
     - `run_state.is_running()` and input empty → `■ Stop` (`btn-abort-prompt`).
     - Otherwise (idle) → existing Arrow Up behavior, unaffected by queue state.
3. **State Management** (`apps/desktop/src/state/`):
   - Store `queued_prompts: HashMap<String, Option<QueuedPrompt>>` keyed by `session_id`, updated from
     the `queueUpdated` SSE frame (§4.3) the same way other session-scoped event state is updated in
     `apps/desktop/src/state/run.rs`.
   - On `ComposerEvent::Submit` when `run_state.is_running()`: call `POST /sessions/:id/queue` (replacing
     any existing queued prompt) instead of `POST /sessions/:id/run`.

### 5.2 Queue vs. Run State Independence

A session can be in any of these combinations, all of which the UI must render correctly:
- Idle, no queue (default).
- Running, no queue (today's behavior).
- Running, queue present (new: card shown above composer, Stop/Queue button per §5.1).
- Idle, queue present (transient — only possible for the instant between a run settling and the
  auto-drained Turn 2's `turnStart` arriving; the client should treat this like "Running" for button
  purposes until `turnStart` or `done`/`aborted` without a follow-up event clears it).

---

## 6. Mobile Implementation (`apps/mobile`)

### 6.1 UI & UX Flow

1. **Floating Capsule with Queued Banner**:
   - `QueuedPromptBanner` renders directly above the bottom chat input bar when `session.queuedPrompt` is present.
   - Smooth animated slide-up entry and exit using `react-native-reanimated`.
2. **Mobile Composer Actions**:
   - When turn is running and input field is focused with text: Send icon changes to a blue Queue badge (`↑`).
   - Tapping Queue calls `POST /sessions/:id/queue` and clears the input without interrupting the run.
3. **Card Actions**:
   - **Edit**: Tapping the card or edit icon calls `DELETE /sessions/:id/queue`, then restores the text to the `TextInput` and focuses keyboard.
   - **Trash**: Calls `DELETE /sessions/:id/queue`.
   - **Steer / Send Now**: Calls `POST /sessions/:id/steer` with a confirmation haptic to interrupt and re-route the agent.
4. **Event handling**: `apps/mobile/utils/chat-events.ts` and `apps/mobile/stores/useChatStore.ts` gain one
   new case for `queueUpdated`, mirroring how `todoUpdate` is already handled — no other event handling
   changes are required (see §4.3).

---

## 7. Edge Cases & Safety Invariants

1. **Run Errors / Tool Failures**:
   - If Turn 1 encounters an error, or a required permission is rejected, the queue is **held, not
     discarded** — the `finally` drain in §4.4 still runs, but the resulting Turn 2 error path (see item 5
     below) is what actually decides whether the queue survives. The user can also click Steer to
     proceed immediately or Edit to adjust before Turn 1 even settles.
2. **Session Switching**:
   - Queued state is strictly scoped per `sessionId` (`pendingNextTurn` is keyed by session, like
     `activeRuns`). Switching between tabs or sessions shows the corresponding session's queue.
3. **Client Disconnection / Reconnection**:
   - Because the queue is persisted via `sessionStorage` (§4.1), restarting the desktop app or reloading
     mobile — even across a server restart — recovers the queued prompt via
     `GET /api/sessions/:id/queue`, not just from in-memory server state.
4. **Race Conditions on Auto-Drain vs. Steer**:
   - No separate mutex is introduced. `RunService.activeRuns` (existing) already serializes run
     lifecycle per session; the `finally`-block drain in §4.4 is the single place `pendingNextTurn` is
     read and cleared, so a steer request arriving at the exact moment a turn settles cannot cause two
     Turn-2 starts — either the steer's `abortRun` call lands before the natural settle (aborting sooner)
     or after (a no-op `abortRun` since the run already finished), but the drain itself always happens
     exactly once, from one call site.
5. **Auto-Drained or Steered Turn Fails to Start**:
   - If the drained `runAgentStream` call itself throws before producing any events — e.g. an unknown
     provider, or `catalogModel?.supportsImages === false` for an attachment carried over from the queue
     (per the existing check in `run.service.ts`) — surface it as a normal turn error (`error` event +
     `needs_attention` session status, matching existing error handling in `runAgentStreamInternal`)
     rather than silently dropping the prompt. `pendingNextTurn` is already cleared at this point, so it
     will not be retried automatically; the user must Edit or resend.
   - Attachments on a queued prompt are validated against the resolved model at drain time, not at queue
     time, since the active model/provider may change between when a prompt is queued and when it
     actually starts.

---

## 8. Summary of Tasks for Implementation

- [ ] **Server (`apps/server`)**:
  - Add `pendingNextTurn` map, `queuePrompt`/`clearQueuedPrompt`/`steer` methods, and the `finally`-block
    drain to `RunService` (§4.4).
  - Persist `QueuedPrompt` via `sessionStorage`, following the `session-todos.ts` pattern (§4.1).
  - Add REST endpoints: `POST`/`GET`/`DELETE /sessions/:id/queue`, `POST /sessions/:id/steer` (§4.2).
  - Add the single `queueUpdated` variant to `AgentSessionEvent` in `packages/types/src/events.ts` and
    mirror it in `apps/desktop/crates/console-core/src/types/events.rs` (§4.3).
- [ ] **Desktop (`apps/desktop`)**:
  - Build `QueuedPromptCard` component in `console-ui`.
  - Add `has_queued_prompt` prop to `ComposerView`; do not add a new `ComposerRunState` variant (§5.1).
  - Wire queue, edit, delete, and steer actions to the new REST endpoints; handle `queueUpdated` in
    `apps/desktop/src/state/run.rs`.
- [ ] **Mobile (`apps/mobile`)**:
  - Build `QueuedPromptBanner` with Reanimated animations.
  - Connect to queue/steer REST endpoints; handle `queueUpdated` in `chat-events.ts` / `useChatStore.ts`.
