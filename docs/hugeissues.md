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


## 2. Terminal View: Missing Paste & Caps Lock Support

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

---

## 4. Mobile Stale Database Loading & Failed Real-Time Re-Attach

### Symptoms
- When starting a task on Desktop (or server) and opening the chat on Mobile, Mobile displays only stale SQLite-persisted history instead of joining the live stream.
- The chat on Mobile appears frozen in an idle state or fails to show that an agent run is actively in progress.
- Messages that were streamed live on Desktop do not show up in Mobile until the run has fully concluded.

### Root Cause Analysis

Located in:
- [`apps/mobile/hooks/useChatStream.ts`](../apps/mobile/hooks/useChatStream.ts)
- [`apps/mobile/stores/useProjectStore.ts`](../apps/mobile/stores/useProjectStore.ts)
- [`apps/mobile/hooks/useHomeSessions.ts`](../apps/mobile/hooks/useHomeSessions.ts)
- [`apps/server/api/src/services/session.service.ts`](../apps/server/api/src/services/session.service.ts)

#### A. Disconnected Status Source in `useChatStream.ts`
In `apps/mobile/hooks/useChatStream.ts` (lines 83–87):
```typescript
const serverStatus = sessionStatuses$[selectedSessionId].peek();
const hasLocalStream = getController(selectedSessionId);
if (!isRunning && serverStatus === "working" && !hasLocalStream && selectedSessionId) {
    attachServerRun(selectedSessionId);
}
```
`useChatStream` relies entirely on `sessionStatuses$[selectedSessionId].peek()`.
However:
1. `sessionStatuses$` is **only** populated inside `useProjectStore.ts` (line 64) when listing sessions for an active project view.
2. If a user opens a chat from the Home screen (`useHomeSessions.ts`), Search, Recent Chats, or Scratchpad, `setStatuses` is **never called**.
3. In `useChatStream.ts`, `latestHeader` is fetched directly from `GET /api/sessions/:id` (via `useSession`):
   ```typescript
   const latestHeader = sessionQuery.data?.pages[0]?.header;
   ```
   **`useChatStream` completely ignores `latestHeader.status`!** It never updates `sessionStatuses$`, nor does it check `latestHeader.status === "working"` directly.
4. Because `sessionStatuses$[selectedSessionId]` is `undefined`, `serverStatus === "working"` evaluates to `false`.
5. Mobile falls into `loadSessionMessages(selectedSessionId, allMessages)` (which only contains static completed turns from SQLite) and **never calls `attachServerRun`**.

#### B. Server-Side Session Status Is Not Dynamically Authoritative
In `apps/server/api/src/services/session.service.ts`:
1. `getSession()` checks if a run is *inactive* to reset `"working"` to `"done"`, but it **never forces `status: "working"`** if `RunService.isRunActive(sessionId)` is `true`. If SQLite had an un-flushed or default status, the response header still reports `"idle"` or `null`.
2. `listSessions()` directly returns SQLite rows without enriching with `RunService.isRunActive(session.id)`. Mobile and Desktop session lists therefore fail to show the live "working" badge unless SQLite was written synchronously beforehand.

### Proposed Remediation
1. **Direct Header Check in Mobile `useChatStream.ts`**:
   Update `useChatStream` to inspect `latestHeader?.status === "working"`:
   ```typescript
   const serverStatus = latestHeader?.status ?? sessionStatuses$[selectedSessionId].peek();
   ```
   Also sync `latestHeader.status` into `sessionStatuses$[selectedSessionId].set(latestHeader.status)` whenever `latestHeader` arrives.
2. **Seed Statuses in `useHomeSessions.ts`**:
   Call `setStatuses(sessions)` in `useHomeSessions` so recent and home-screen sessions have accurate cached statuses.
3. **Dynamic Status on Server**:
   In `sessionService.getSession` and `sessionService.listSessions`, dynamically override `session.status = "working"` (or `header.status = "working"`) if `RunService.isRunActive(session.id)` is `true`.

---

## 5. Post-Streaming Transcript Regressing to Old Messages (Replaced by Stale Settled State)

### Symptoms
- An agent finishes streaming a response on Desktop.
- Immediately after streaming completes (or a moment later), the latest response vanishes or snaps back to older messages.
- The user has to close the chat tab and re-open it to see the latest messages.

### Correlation with `docs/papercut-fixes-spec.md` (Fix 3)
This is directly caused by the **`wait_until_settled` post-stream reconciliation race** specified in Fix 3 of [`docs/papercut-fixes-spec.md`](papercut-fixes-spec.md#fix-3--after-second-prompt-completes-ui-shows-first-prompts-state-stale-transcript):

1. **The In-Memory vs Canonical Collision**: While the agent is running, Desktop streams tokens directly into `TranscriptView` via `AgentSessionEvent` deltas (optimistic/live state).
2. **Premature / Out-of-Order Overwrite**: Once the SSE stream terminates, `run.rs` (line 278) awaits `client.sessions.wait_until_settled(&session_id)`. When it returns, line 298 executes:
   ```rust
   this.transcript_for_pane(&run_pane_id).update(cx, |t, cx| {
       t.set_messages(detail.messages, cx); // <--- Overwrites live transcript
   });
   ```
3. **The Race Condition**:
   - For sequential prompts (Prompt A followed by Prompt B): Run A's detached `wait_until_settled` resolves *after* Prompt B has streamed, calling `set_messages([Prompt A, Result A])`, obliterating Prompt B and Result B.
   - For single-prompt boundary races: If `wait_until_settled` polls `GET /api/sessions/:id` before SQLite has committed the final turn row from `RunService`, `detail.messages` returns without the latest turn, clobbering the live stream with the previous turn.
   - When the user closes and re-opens the tab, `load_session_messages_for_pane` fetches the now-settled SQLite database from scratch, which finally shows the latest prompt.

### Remedy
As specified in `papercut-fixes-spec.md`:
1. Use **monotonic run tokens** (`run_token`) per session to discard any `wait_until_settled` payload that belongs to an earlier run.
2. In `wait_until_settled`, ensure the payload contains at least the message count of the current transcript before calling `t.set_messages(...)`, preventing regression to a shorter or older message list.
