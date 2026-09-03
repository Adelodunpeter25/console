# Papercut Fixes Spec

Two unrelated papercut bugs to fix in a single PR (or two separate commits).

---

## Fix 1 — Chat Tab Title Does Not Update in Real Time After First-Prompt Rename

### Symptom
When a new chat is created and the user sends their first prompt, the backend auto-generates
a title and sends it back in the SSE stream. The sidebar list and the window title bar both
update immediately. But the **chat tab** in the tab bar continues to show "New Chat" (or
whatever the original placeholder was) until the tab is closed and re-opened.

### Root Cause

`apply_session_header_for_pane` in
`apps/desktop/src/state/sessions.rs`
correctly calls `rename_tabs` on `self.workspace_root` when it detects a title change
(lines 258–268). However, the function **never calls `cx.notify()`** after making that
mutation.

The sidebar and title bar update because they are driven by `self.sessions` (which is an
`Rc`-wrapped vec). Mutating it via `Rc::make_mut` marks the data dirty and any code that
reads `sessions` in the render path sees the fresh clone. The GPUI entity is still notified
elsewhere in the same run-loop tick (e.g. `set_pane_model` calls something that triggers
`cx.notify`, or the outer SSE loop does). That is enough to re-render the sidebar.

But `workspace_root` is **plain owned data** — it is not behind any reactive wrapper.
`rename_tabs` mutates it in place. GPUI only re-renders `WorkspaceTabBar` when the entity
re-renders and clones a fresh `workspace_root` snapshot. If the notify that happens to fire
in the same tick is for a *different* mutation path that doesn't go through the tab-bar
render, the tab bar gets a stale clone.

In practice the notify *does* eventually fire (because SSE drives many updates), but the
tab bar's `leaf.clone()` may have been snapshotted before `rename_tabs` ran, leaving the
tab showing the old title until the next full re-render from something else (e.g. closing
and re-opening the tab forces a fresh `workspace_root.clone()`).

### Fix

Add `cx.notify()` at the end of the `if !header.title.trim().is_empty() && ...` block in
`apply_session_header_for_pane`, immediately after `rename_tabs` returns:

```rust
// sessions.rs — apply_session_header_for_pane
if !header.title.trim().is_empty() && session.title != header.title {
    session.title = header.title.clone();
    console_ui::workspace::ops::rename_tabs(
        &mut self.workspace_root,
        |tab| {
            matches!(tab, console_core::WorkspaceTabConfig::Chat { session_id, .. }
                if session_id == &header.id)
        },
        header.title.clone(),
    );
    cx.notify(); // ← ADD THIS
}
```

**File to edit:**
`apps/desktop/src/state/sessions.rs`
around line 268 (inside `apply_session_header_for_pane`).

**Why this is safe:** `cx.notify()` in a `&mut self` method is the standard GPUI pattern
to schedule a re-render. It is idempotent — multiple calls in the same tick coalesce.

---

## Fix 2 — AI SDK Warning: Non-OpenAI Reasoning Parts Skipped for Muse Models

### Symptom
```
Warning: AI SDK Warning (openai.responses / muse-spark-1.3-contributor-free):
Non-OpenAI reasoning parts are not supported.
Skipping reasoning part: {"type":"reasoning","text":" "}.
```
Appears in the server log on every turn after the first when using a Muse or other
Responses API model. Responses still arrive but the model loses its own prior chain-of-thought.

### Root Cause

Muse models return **reasoning tokens** in the Responses API stream. The AI SDK stores
these as `reasoning`-type content parts inside the assistant message that goes into our
session message history.

On the **next turn**, `convertOpencodeMessages` converts the full message history
(including those stored `reasoning` parts) back into the wire format. The `@ai-sdk/openai`
Responses driver receives those `reasoning` parts and rejects them with the warning above,
because it only accepts **encrypted** OpenAI reasoning tokens (i.e. the opaque
`encrypted_content` blobs that OpenAI's own models return) — not plain-text thinking blocks
from third-party models.

### Fix

In `apps/server/providers/src/opencode/convert-messages.ts`,
when building the assistant message content array, **strip any `reasoning`-type content
parts** before returning the messages for a Responses API model.

There are two equivalent approaches:

#### Option A — Strip in `convertOpencodeMessages` unconditionally (Recommended)
Remove `reasoning` parts from all outgoing assistant messages regardless of model.
This is safe because no OpenAI-compatible `/chat/completions` model uses reasoning parts
in history either.

```typescript
// convert-messages.ts — inside the assistant message branch
const contentParts = msg.content.filter(
  (part) => part.type !== "reasoning"   // strip thinking blocks — Responses API
                                         // only accepts encrypted reasoning tokens
);
```

#### Option B — Pass `isResponsesModel` flag and strip conditionally
Only strip when `isOpencodeResponsesModel(modelId)` is true. Slightly more surgical but
requires threading the model ID through `convertOpencodeMessages`.

**Recommended: Option A.** Plain-text reasoning parts in outgoing history are never useful
for any provider we support, and the unconditional strip keeps the function signature simple.

**File to edit:**
`apps/server/providers/src/opencode/convert-messages.ts`

After the fix, the warnings disappear and multi-turn Muse conversations work cleanly.
The model simply won't have its previous inner thoughts in context (which is expected —
OpenAI's own encrypted tokens are how *their* models replay reasoning, and Muse doesn't
provide an equivalent).

### Test
Run `cd apps/server && bun tests/opencode.test.ts` — no new test required for the strip
(it is a passthrough filter), but optionally add an assertion in the
`convertOpencodeMessages` test block that an input message with a `reasoning` part
produces output with no `reasoning` part.

---

## Fix 3 — After Second Prompt Completes, UI Shows First Prompt's State (Stale Transcript)

### Symptom
1. User opens a chat, sends Prompt A — agent completes, transcript shows result A.
2. User sends Prompt B in the same tab — agent completes, transcript flashes back to showing
   only result A (the first-prompt state). UI is stuck showing the wrong state until the
   tab is closed and re-selected (which triggers a fresh `load_session_messages_for_pane`).

### Root Cause

Each `submit_prompt` spawns an async task that does this **after** the SSE stream ends:

```rust
// run.rs ~line 278
match client.sessions.wait_until_settled(&session_id).await {
    Ok(detail) => {
        // ...
        if this.active_session_for_pane(&run_pane_id).as_deref() == Some(session_id.as_str()) {
            this.transcript_for_pane(&run_pane_id).update(cx, |t, cx| {
                t.set_messages(detail.messages, cx);  // ← OVERWRITES transcript
            });
        }
    }
}
```

`wait_until_settled` polls the server until the backend settles the run. This is async and
detached. Now consider the two-prompt sequence:

1. **Run A** is spawned. Its SSE stream finishes. It calls `wait_until_settled` and awaits.
2. **Run B** is submitted before Run A's `wait_until_settled` resolves. Run B's SSE stream
   starts and the transcript accumulates Prompt A + Result A + Prompt B + Result B live.
3. **Run A's `wait_until_settled`** resolves and returns `{ messages: [Prompt A, Result A] }`.
   The guard `active_session_for_pane == session_id` is **still true** (it's the same session,
   same pane), so it calls `t.set_messages([Prompt A, Result A])`, **erasing Prompt B and
   Result B** from the transcript.
4. The UI now shows only the first-prompt state.
5. Eventually **Run B's `wait_until_settled`** resolves with the full `[A, B]` and
   overwrites again with the correct state — but there's a visible window where the UI is wrong.

The guard on line 284 checks `active_session_for_pane == session_id` which is always true
for sequential prompts in the same chat. What it needs to additionally check is: **is this
run still the latest run for that session?** If a newer run has started since, this
`wait_until_settled` result is stale and must not overwrite the transcript.

### Fix

Track the **run sequence number** (or a "latest run started_at") per session. Before calling
`t.set_messages`, guard on whether the current `wait_until_settled` completion belongs to the
most recently started run for this session.

**Simplest approach — guard on `is_session_running`:**

The second prompt sets the session back to running immediately via `set_session_running`.
Run A's `wait_until_settled` resolves *while the session is still running* for Run B.
So the guard becomes:

```rust
if this.active_session_for_pane(&run_pane_id).as_deref() == Some(session_id.as_str())
    && !this.is_session_running(&session_id)   // ← ADD THIS
{
    this.apply_session_header_for_pane(&run_pane_id, &detail.header, cx);
    // ...
    this.transcript_for_pane(&run_pane_id).update(cx, |t, cx| {
        t.set_messages(detail.messages, cx);
    });
}
```

This prevents Run A from overwriting the transcript while Run B is actively streaming. Run B's
own `wait_until_settled` fires after B's `set_session_running(..., None)` call, so it sees
`is_session_running == false` and correctly applies the full canonical message set.

**More robust approach — per-session run token:**

Add a `HashMap<String, u64>` called `session_run_counter`. Increment it at
`submit_prompt` time, capture the value into the closure, and check that the counter
still matches when `wait_until_settled` resolves.

```rust
// At submit time:
let run_token = self.next_run_token_for_session(&session_id);

// In wait_until_settled handler:
if this.current_run_token_for_session(&session_id) == run_token
    && this.active_session_for_pane(&run_pane_id).as_deref() == Some(session_id.as_str())
{
    t.set_messages(detail.messages, cx);
}
```

**Recommended: the `is_session_running` guard.** It requires touching one line in run.rs
and is correct in all normal single-user scenarios. The run-token approach is more future-proof
if parallel runs per session are ever supported, but that adds new state to maintain.

**File to edit:**
`apps/desktop/src/state/run.rs`
— the `wait_until_settled` completion block around line 284.

Apply the same guard to the equivalent `wait_until_settled` block in `attach_session_run_for_pane`
(around line 690) if it also calls `t.set_messages`.

---

## Fix 4 — Changes Tab Badge Shows Count But Panel Shows "No working tree changes"

### Symptom
In a project folder with no git repository:
1. Create a new file (or the agent touches a file during the session).
2. The Changes tab shows a **badge of "1"** (or more).
3. Clicking the Changes tab shows **"No working tree changes"** — the list is empty.

Also affects git repos: if the agent touched files during this session, those session-tracked
changes count toward the badge but are invisible in the Changes panel body.

### Root Cause

The badge count and the panel rendering use two different data sources:

**Badge (right_sidebar.rs line 96):**
```rust
let changes_count = self.working_changes.len() + self.session_changes.len();
```
Counts both `working_changes` (git `status --porcelain`) and `session_changes`
(files the agent touched, stored in the session DB via `session_file_changes` table).

**Panel body (changes_list.rs lines 69–157):**
```rust
.children(self.working_changes.iter().map(|entry| { ... }))
// session_changes is received in the struct but never iterated
```
Only `working_changes` is rendered. `session_changes` is accepted as a field but silently
ignored in `RenderOnce::render`.

So the badge inflates by `session_changes.len()` but the user sees nothing for those entries.

### Fix

Render `session_changes` in the `ChangesListView` body after the `working_changes` section.

The `SessionFileChange` type has the same fields needed for display (`path`, `status`,
`additions`, `deletions`). Add a second `.children()` iterator after the existing one, using
the same row layout. Use a visually distinct section header (e.g. "Session Changes") to
separate them from the git working tree entries so the user understands the two sources.

**File to edit:**
`apps/desktop/crates/console-ui/src/inspector/changes_list.rs`

In `RenderOnce::render`, extend the `else` branch that renders the list:
```rust
// After the existing working_changes iterator...
.children(self.session_changes.iter().map(|entry| {
    // same row layout as working_changes
    // entry.path, entry.status, entry.additions, entry.deletions
}))
```

**Optional but recommended:** Add a section label ("Git Changes" / "Session Changes") when
both lists are non-empty, so the user knows what they're looking at. When only one list has
entries, the label isn't needed.

**Also fix the `has_changes` guard** at line 45 — it already checks both, so that is correct.
The only thing missing is the render loop for session changes.

### Test
No new automated test needed. Visual verification: open a non-git folder, run an agent that
touches a file, open the Changes tab — session-changed files should now appear in the list.

---

## Fix 5 — Title Bar Shows Empty or Stale Chat Title When a File or Diff Tab Is Active

### Symptom
When the user opens a File tab or a Diff tab (e.g. by clicking a file in the Inspector or
a change in the Changes panel), the window title bar either:
- Shows the **previous chat's title** ("Chat Name — folder") — because `selected_session_id`
  still points to the last active chat, or
- Shows **nothing** — if no session is selected (e.g. the file was opened in a fresh pane).

Expected: title bar should show **"filename — project cwd"** (e.g. `README.md — my-project`)
whenever the active tab in the focused pane is a File or Diff tab.

### Root Cause

`titlebar_text` in `apps/desktop/src/view/mod.rs` (lines 234–250) is computed solely from
`self.selected_session_id`:

```rust
let titlebar_text = self
    .selected_session_id
    .as_deref()
    .and_then(|sid| self.sessions.iter().find(|s| s.id == sid))
    .map(|session| {
        // always builds "chat title — folder"
        format!("{} — {}", session.title, folder)
    });
```

There is no branch for File or Diff tabs. The active pane's active tab config is available
in `workspace_root` (cloned at line 23), but `titlebar_text` never consults it.

### Fix

Before falling through to the session-based title, inspect the active pane's active tab.
If it is a `WorkspaceTabConfig::File` or `WorkspaceTabConfig::Diff`, build the title from
the tab's own `title` field (which is the filename, already set when the tab is opened) and
the project/cwd for context.

```rust
// mod.rs — replace the titlebar_text block
let titlebar_text: Option<String> = {
    // First: derive from the active tab in the active pane.
    let active_pane_id = active_pane.as_deref().unwrap_or("pane-main");
    let active_tab = workspace_root
        .leaf(active_pane_id)
        .and_then(|leaf| {
            leaf.active_tab_id.as_deref()
                .and_then(|id| leaf.tabs.iter().find(|t| t.id() == id))
        });

    match active_tab {
        Some(console_core::WorkspaceTabConfig::File { title, .. })
        | Some(console_core::WorkspaceTabConfig::Diff { title, .. }) => {
            // Use the file/diff tab's title (filename) plus the project folder
            // for context, mirroring the chat tab format.
            let folder = self
                .selected_session_id
                .as_deref()
                .and_then(|sid| self.sessions.iter().find(|s| s.id == sid))
                .and_then(|session| {
                    self.projects
                        .iter()
                        .find(|p| p.matches_session(session))
                        .map(|p| p.name.clone())
                        .or_else(|| {
                            let cwd = &session.cwd;
                            if cwd.is_empty() { None }
                            else { Some(console_ui::utils::format_folder_display_name(cwd)) }
                        })
                });
            Some(match folder {
                Some(f) if !f.is_empty() => format!("{} — {}", title, f),
                _ => title.clone(),
            })
        }
        // Chat, Terminal, or no tab: fall through to session-based title (existing logic)
        _ => self
            .selected_session_id
            .as_deref()
            .and_then(|sid| self.sessions.iter().find(|s| s.id == sid))
            .map(|session| {
                let folder = self
                    .projects
                    .iter()
                    .find(|project| project.matches_session(session))
                    .map(|project| project.name.clone())
                    .unwrap_or_else(|| {
                        console_ui::utils::format_folder_display_name(&session.cwd)
                    });
                if folder.is_empty() {
                    session.display_title().to_string()
                } else {
                    format!("{} — {}", session.title, folder)
                }
            }),
    }
};
```

**File to edit:**
`apps/desktop/src/view/mod.rs` — lines 234–250.

Note: `workspace_root` is already cloned as `let workspace_root = self.workspace_root.clone()`
at line 23, and `active_pane` as `let active_pane = self.active_pane_id.clone()` at line 24,
so both are in scope without any additional borrowing gymnastics. The `WorkspaceNode` type
needs a `leaf(&str)` accessor — check if `leaf()` exists or use `leaves().find(|l| l.id == id)`.

### Test
Manual verification: open a file from the Inspector or Changes panel, confirm the title bar
updates to `filename — project`. Switch back to a chat tab, confirm it reverts to
`chat title — project`.
