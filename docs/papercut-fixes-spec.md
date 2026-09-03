# Papercut Fixes Spec



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
