# Chat-Contained Workspace with Subtabs

## Motivation
Previously, Console used a global, pane-centric workspace model where chat tabs and artifact tabs (`File`, `Diff`, `Terminal`, `Browser`) were arbitrary siblings across panes. When commenting on a file in the editor gutter or annotating an element in a browser tab, the system had to perform heuristic lookups (`pane_hosting_file`, `pane_hosting_browser`, checking active panes) to calculate which composer should receive the staged context. When multiple chats or splits were open, comments and annotations frequently routed to the wrong pane or failed to appear in the active chat composer.

## Architecture

### 1. Inverted Hierarchy: Chat as the Workspace Container
The active Chat is the primary container for the workspace:
- **Left Side**: Pinned Chat (Transcript + Composer).
- **Right Side**: Secondary Subtabs (`File`, `Diff`, `Terminal`, `Browser`, `ChangesReview`) scoped to the active chat.

```text
┌──────────────┬─────────────────────────────────────────────────┬──────────────────┐
│ Left Sidebar │ Center Workspace (Active Chat Container)         │ Right Inspector  │
│              ├──────────────────────┬──────────────────────────┤                  │
│ • Chat A     │ Chat A (Pinned Left) │ Chat A Subtabs (Right)   │ • File Tree      │
│ • Chat B     │                      │ [main.rs][Diff][Terminal]│ • Git Changes    │
│              │ ── Transcript ──     │                          │                  │
│              │                      │ (File viewer / editor)   ├──────────────────┤
│              │                      │                          │ Bottom Bar (Run) │
│              │ ── Composer ──       │                          │ • console.toml   │
│              │ [Draft & context]    │                          │ • Run scripts    │
└──────────────┴──────────────────────┴──────────────────────────┴──────────────────┘
```

### 2. Automatic Split & Tab Placement
- Opening an artifact (`open_file_tab`, `open_diff_tab`, `open_terminal_tab`, `open_browser_tab`, or clicking markdown file links) targets the right-side split (`target_subtab_pane_id`).
- If only the chat pane is currently open, it automatically splits horizontally (chat on left, subtab on right).
- When all subtabs on the right are closed, the split cleanly collapses back to full-width chat.

### 3. Subtab Containment & Session Switching
- Subtabs are saved per chat session in `session_subtabs: HashMap<String, Vec<WorkspaceTabConfig>>`.
- When switching between chats in the sidebar (`select_and_open_session`):
  1. The outgoing chat stashes its open subtabs.
  2. The incoming chat restores its open subtabs on the right. If it has no open subtabs, the right pane collapses.
  3. Terminals remain managed in the background by the server PTY manager.

### 4. Zero-Heuristic Direct Context Attribution
- **Browser Annotations**: `stage_browser_annotation` resolves `active_chat_pane_id()` directly, ensuring annotations stage immediately above the active chat composer.
- **File Line Comments**: Staged file comments across the active workspace are surfaced to the active chat composer via `composer_comments_for_pane` and submitted with the prompt via `prompt_with_staged_comments`.
- Redundant floating composers inside file views are omitted when the chat pane is visible beside the file.

### 5. Non-Goals
- Inspector and the bottom-bar Run panel (`console.toml` script execution) stay as-is.
- Server-side terminal lifecycle (Go PTY manager) remains untouched.
