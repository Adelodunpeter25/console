# Huge Issues Audit & Architectural Diagnosis

This document details the root causes and remediation plans for three critical issues across Desktop, Terminal, and the Server Run Hub.

> **IMPORTANT**: DO NOT APPLY FIXES WHILE AN AGENT RUN IS IN PROGRESS. This document serves as the comprehensive architectural specification for implementation.

---

## 1. Transcript Infinite Scroll & Scrollback Bugs During Streaming

### Symptoms
- While scrolling back (scrolling up) to read previous prompts/responses, the transcript suddenly jumps or snaps all the way back to the **very first prompt** of the conversation.
- Severe jitter, jumping, or fighting the scrollbar occurs while the agent is streaming.
- Gradually scrolling upwards gets trapped in a loop or teleports to row 0.

### Root Cause Analysis

Located in:
- [`apps/desktop/crates/console-ui/src/chat/transcript_view.rs`](../apps/desktop/crates/console-ui/src/chat/transcript_view.rs)
- [`apps/desktop/src/state/pagination.rs`](../apps/desktop/src/state/pagination.rs)

#### A. The `stay_at_top` & `refresh_list()` Reset Trap
In `transcript_view.rs` (`prepend_messages`, lines 184–209):
```rust
let prepended = self.messages.len() - old_len;
let stay_at_top = anchor.map(|(r, _, _)| r <= 2).unwrap_or(false);
if stay_at_top {
    // Let refresh_list reset to 0 and keep the viewport at the top
    self.refresh_list();
} else if let Some((row, offset, _at_tail)) = anchor {
    let new_row = row + prepended;
    let entity = cx.entity().downgrade();
    cx.spawn(async move |_, cx| {
        cx.background_executor().timer(Duration::from_millis(16)).await;
        cx.update(|cx| {
            if let Some(e) = entity.upgrade() {
                e.update(cx, |this, _| {
                    this.list_state.scroll_to(ListOffset {
                        item_ix: new_row,
                        offset_in_item: px(offset),
                    });
                });
            }
        });
    }).detach();
    self.refresh_list(); // <--- BUG!
}
```
There are two catastrophic flaws here:
1. **`stay_at_top` Triggers at `r <= 2`**: When a user scrolls up toward older messages, as soon as the top visible item index reaches `2`, `stay_at_top` becomes `true`. It calls `self.refresh_list()`, which executes `self.list_state.reset(row_count)` with no offset, immediately **jumping the viewport to row 0 (the first prompt)**.
2. **Synchronous Reset Before Asynchronous Scroll Restoration**: In the `else` branch, it schedules a 16ms timer to restore the scroll anchor to `new_row`, but **immediately calls `self.refresh_list()` synchronously on line 206**. In GPUI, `list_state.reset(row_count)` wipes the scroll offset back to row 0 on that exact frame. The user experiences an instantaneous jump to the top before the delayed timer tries to pull them back.

#### B. The Auto-Pagination Feedback Loop
In `transcript_view.rs` (`render`, lines 888–902):
```rust
if self.has_more && !self.loading_older && !self.messages.is_empty() {
    let top = self.list_state.logical_scroll_top();
    if top.item_ix <= 5 {
        // Triggers load_older_messages_for_pane
    }
}
```
Whenever `refresh_list()` snaps the list to row 0, `top.item_ix <= 5` is instantly true again on the subsequent render pass. This immediately triggers another page fetch, prepending more messages, resetting to row 0 again, creating an infinite auto-scroll feedback loop.

#### C. Collision with Live Streaming Renders
During agent streaming, `schedule_stream_render` flushes new tokens every 33ms. Each flush updates transcript message contents and calls `refresh_list()`. If the user has scrolled up (`is_following_tail()` is false), calling `list_state.reset(row_count)` whenever rows change or resize during streaming destabilizes GPUI's internal scroll offset.

### Proposed Remediation
1. **Eliminate `stay_at_top` Reset to Row 0**: When older messages are prepended, the scroll anchor must *always* be preserved relative to the message that was previously visible (`new_row = row + prepended`).
2. **Synchronous Scroll Anchoring**: Never delay scroll position restoration behind an arbitrary 16ms timer while resetting the list synchronously. GPUI's `ListState` should be reset and updated with `scroll_to(ListOffset { item_ix: new_row, offset_in_item })` in the exact same transaction before painting.
3. **Threshold Guard**: Only trigger pagination when the user actively scrolls upwards with intent, not when the list is programmatically positioned or settling.

---

## 2. Mobile-to-Desktop Real-Time Re-Attach & Run-Hub Desync

### Symptoms
- When initiating a prompt/task on mobile via `@console/utils` `RunEventHub`, opening that project on Desktop shows only old SQLite-persisted data.
- Desktop does not seamlessly stream the active in-progress run or shows a stale state until the entire run completes.

### Root Cause Analysis

Located in:
- [`packages/utils/src/run-hub.ts`](../packages/utils/src/run-hub.ts)
- [`apps/server/api/src/routes/run.ts`](../apps/server/api/src/routes/run.ts)
- [`apps/server/api/src/services/session.service.ts`](../apps/server/api/src/services/session.service.ts)
- [`apps/desktop/src/state/sessions.rs`](../apps/desktop/src/state/sessions.rs)
- [`apps/desktop/src/state/run.rs`](../apps/desktop/src/state/run.rs)

#### A. Empty Replay Queue When Attaching With No `since` Parameter
In `packages/utils/src/run-hub.ts` (lines 106–117):
```typescript
subscribe(sub: RunStreamSubscriber, since?: number): void {
  const qs: QueuedSubscriber = { sub, queue: [], draining: false, dead: false, drainPromise: Promise.resolve() };

  if (since !== undefined) {
    for (const item of this.buffer) {
      if (item.seq > since) qs.queue.push(item);
    }
  }
  this.subs.set(sub.id, qs);
  void this.drain(qs);
}
```
And in Desktop's `attach_session_run_for_pane` (`apps/desktop/src/state/run.rs`, line 613):
```rust
client.runs.attach_run_stream(&session_id, None).await
```
When Desktop attaches to an in-flight run started on mobile, it passes `since: None`.
Because `since === undefined`, `run-hub.ts` queues **zero** items from `this.buffer` (`MAX_REPLAY_BUFFER = 500`). It only registers for future broadcast events.
However, in SQLite, the active assistant turn is not yet finalized or saved (it only commits on `TurnEnd` / `SessionEnd`).
**Result**: Desktop loads older history from SQLite, skips the replay buffer of the active turn, and misses all tokens/tool calls generated before Desktop connected.

#### B. Dynamic `status` Not Reflected in Session Header
In `apps/server/api/src/services/session.service.ts` (`getSession`, lines 70–78):
```typescript
const session = this.storage.loadSessionPage(sessionId, options);
if (!session) return null;

if (!RunService.isRunActive(sessionId)) {
  if (session.header.status === "working" || session.header.status === "needs_attention") {
    this.storage.updateSessionStatus(sessionId, "done");
    session.header.status = "done";
  }
}
```
If `RunService.isRunActive(sessionId)` is `true`, `session.header.status` is **not dynamically forced to `"working"`**. If SQLite had not yet flushed or if `listSessions` is queried, the desktop client may observe a null or non-`Working` status, causing `detail.header.status == Some(Working)` in `load_session_messages_for_pane` to evaluate to `false` and bypass `attach_session_run_for_pane` altogether.

### Proposed Remediation
1. **Replay Buffer on Initial Re-Attach**: When a client attaches to `/api/sessions/:id/run/stream` without a `since` parameter (or with `since=0`), `RunEventHub` must replay the active turn's buffered frames (coalesced deltas and tool execution starts) so the joining client catches up to the current streaming state.
2. **Authoritative Run Status**: Ensure `sessionService.getSession` and `sessionService.listSessions` inject `status: "working"` whenever `RunService.isRunActive(sessionId)` is true.

---

## 3. Terminal View: Missing Paste & Caps Lock Support

### Symptoms
- Pressing `Cmd+V` (macOS) in the terminal does not paste clipboard contents; instead, it literally sends the character `'v'` to the pty shell.
- Caps Lock has no effect; letters typed while Caps Lock is engaged arrive as lowercase characters.

### Root Cause Analysis

Located in:
- [`apps/desktop/crates/console-ui/src/terminal/terminal_view.rs`](../apps/desktop/crates/console-ui/src/terminal/terminal_view.rs)

#### A. No Clipboard / Paste Handler
In `terminal_view.rs`, `key_to_bytes` (lines 144–226) only inspects `mods.control`, `mods.alt`, and `mods.shift`.
There is no branch checking for `mods.platform` (Cmd on macOS) or platform-standard paste chords (`Cmd+V` / `Ctrl+Shift+V` / `Ctrl+V`):
```rust
match key {
    ...
    _ if key.chars().count() == 1 => Some(key.to_string()),
    _ => None,
}
```
When `Cmd+V` is pressed:
- `mods.platform` is `true`.
- `key` is `"v"`.
- It misses all modifier matches, falls into `key.chars().count() == 1`, and emits `"v"`.
- The terminal receives the literal byte `0x76` (`'v'`) and never reads GPUI's clipboard.

#### B. No Caps Lock Evaluation
In GPUI, `event.keystroke.key` provides the raw key identifier (e.g. `"a"`, `"b"`).
`key_to_bytes` simply returns `key.to_string()` without inspecting whether `event.keystroke.modifiers.caps_lock` is active or using GPUI's text input representation.
Consequently, typing with Caps Lock engaged produces lowercase input in the terminal.

### Proposed Remediation
1. **Implement Paste in `on_key_down`**:
   In `terminal_view.rs` on `KeyDownEvent`:
   ```rust
   let is_paste = (event.keystroke.modifiers.platform && event.keystroke.key == "v")
       || (event.keystroke.modifiers.control && event.keystroke.modifiers.shift && event.keystroke.key == "v");

   if is_paste {
       if let Some(clipboard) = cx.read_from_clipboard() {
           if let Some(text) = clipboard.text() {
               if let Some(h) = &handle_for_key {
                   h.send_input(text);
               }
           }
       }
       return;
   }
   ```
2. **Support Caps Lock in `key_to_bytes`**:
   If `event.keystroke.modifiers.caps_lock` is true, uppercase single alphabetic ascii characters (`'a'..='z'`) before sending.
