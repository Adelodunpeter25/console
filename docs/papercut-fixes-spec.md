# Papercut Fixes Spec


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

Track a **per-session run token**. The `is_session_running` guard alone is
incorrect: `set_session_running(..., None)` only happens *after* the
`wait_until_settled` block (`run.rs:342`), so at resolve time the flag is
`true` for both the stale Run A and the fresh Run B — the guard would block
Run B's own legitimate canonical update too.

Add a `session_run_token: HashMap<String, u64>` (monotonic counter; do not
reuse `run_started_at`, which is second-granularity `timestamp()` and can
collide for rapid sequential prompts). Increment it in `submit_prompt`,
capture the value into the spawned closure, and check equality before
`set_messages`:

```rust
// At submit time (before cx.spawn):
let run_token = self.next_run_token(&run_session_key);

// In wait_until_settled handler (run.rs ~284):
if this.current_run_token(&session_id) == run_token
    && this.active_session_for_pane(&run_pane_id).as_deref() == Some(session_id.as_str())
{
    this.apply_session_header_for_pane(&run_pane_id, &detail.header, cx);
    // ...
    this.transcript_for_pane(&run_pane_id).update(cx, |t, cx| {
        t.set_messages(detail.messages, cx);
    });
}
```

Stale Run A sees a token mismatch and skips; fresh Run B matches and applies.
Single-run case is unaffected (token matches).

**Files to edit:**
- New `apps/desktop/src/state/token.rs` — owns the counter and helpers:
  ```rust
  // token.rs
  use std::collections::HashMap;

  #[derive(Default)]
  pub struct RunTokenCounter {
      tokens: HashMap<String, u64>,
  }

  impl RunTokenCounter {
      pub fn next_run_token(&mut self, session_id: &str) -> u64 {
          let entry = self.tokens.entry(session_id.to_string()).or_insert(0);
          *entry = entry.wrapping_add(1);
          *entry
      }

      pub fn current_run_token(&self, session_id: &str) -> u64 {
          self.tokens.get(session_id).copied().unwrap_or(0)
      }
  }
  ```
  Wire it into `ConsoleDesktopApp` (e.g. `pub(crate) run_tokens: RunTokenCounter`
  in `app.rs` next to `running_sessions`, declared in `state/mod.rs`).
- `apps/desktop/src/state/run.rs` — capture `let run_token =
  self.run_tokens.next_run_token(&key)` in `submit_prompt` (key on session id
  once known, falling back to the pane id for the pre-creation case), move it
  into the spawned closure, and guard the `wait_until_settled` completion
  block around line 284 with `this.run_tokens.current_run_token(&session_id)
  == run_token` alongside the existing pane check.

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
