# Codebase Refactoring & Modularization Plan

This document outlines the systematic refactoring and modularization roadmap for the desktop client, focusing on [`apps/desktop/crates/console-ui/src/layout/sidebar_view.rs`](file:///Users/adelodunpeter/Developer/Projects/console/apps/desktop/crates/console-ui/src/layout/sidebar_view.rs) and [`apps/desktop/src/state/`](file:///Users/adelodunpeter/Developer/Projects/console/apps/desktop/src/state/).

---

## Overview & Objectives

1. **Eliminate Architectural Bloat**: Decompose oversized files exceeding 400 lines into cohesive, single-responsibility submodules.
2. **Deduplicate Common Logic**: Centralize duplicated project matching, session title fallback, and directory name formatting functions.
3. **Purge Orphaned Dead Code**: Remove ~12,400 lines of uncompiled legacy files in `console-ui` that cause confusion and duplication risks.
4. **Preserve Exact Behavioral Parity**: Maintain all existing GPUI layout, virtualized scrolling, draft interactions, and state synchronization with zero functional regressions.

---

## Phase 1: Zero-Risk Cleanup & Core Utilities Centralization

### Task 1.1: Purge Orphaned Legacy Files (~12,400 Lines)
Delete uncompiled Waku-era files in `apps/desktop/crates/console-ui/` that are not included in any `mod.rs`:
- [x] `apps/desktop/crates/console-ui/src/layout/sidebar.rs` (1,580 lines)
- [x] `apps/desktop/crates/console-ui/src/layout/right_panel.rs` (3,661 lines)
- [x] `apps/desktop/crates/console-ui/src/chat/sessions.rs` (1,584 lines)
- [x] `apps/desktop/crates/console-ui/src/chat/transcript.rs` (1,150 lines)
- [x] `apps/desktop/crates/console-ui/src/chat/streaming.rs` (864 lines)
- [x] `apps/desktop/crates/console-ui/src/common/components.rs` (1,374 lines)
- [x] `apps/desktop/crates/console-ui/src/common/command_palette.rs` (1,184 lines)
- [x] `apps/desktop/crates/console-ui/src/common/file_search.rs` (987 lines)
- [x] `apps/desktop/crates/console-ui/src/common/commit_dialog.rs` (780 lines)
- [x] `apps/desktop/crates/console-ui/src/common/image_preview.rs` (310 lines)

### Task 1.2: Centralize Project Matching Logic
- **Target File**: [`apps/desktop/crates/console-core/src/types/project.rs`](file:///Users/adelodunpeter/Developer/Projects/console/apps/desktop/crates/console-core/src/types/project.rs)
- [x] Implement `ProjectInfo::matches_session(&self, session: &SessionHeader) -> bool`.
- [x] Replace duplicated matching in:
  - [`apps/desktop/crates/console-ui/src/layout/sidebar_view.rs`](file:///Users/adelodunpeter/Developer/Projects/console/apps/desktop/crates/console-ui/src/layout/sidebar_view.rs)
  - [`apps/desktop/src/state/app.rs`](file:///Users/adelodunpeter/Developer/Projects/console/apps/desktop/src/state/app.rs)
  - [`apps/desktop/src/view.rs`](file:///Users/adelodunpeter/Developer/Projects/console/apps/desktop/src/view.rs)

### Task 1.3: Centralize Fallback Session Title Formatting
- **Target File**: [`apps/desktop/crates/console-core/src/types/session.rs`](file:///Users/adelodunpeter/Developer/Projects/console/apps/desktop/crates/console-core/src/types/session.rs)
- [x] Implement `SessionHeader::display_title(&self) -> &str` (defaults trimmed empty string to `"New Chat"`).
- [x] Update call sites in `sidebar_view.rs`, `sessions.rs`, `app.rs`, and `view.rs`.

### Task 1.4: Centralize Directory Name & Sentence-Casing Helper
- **Target File**: [`apps/desktop/crates/console-ui/src/utils/mod.rs`](file:///Users/adelodunpeter/Developer/Projects/console/apps/desktop/crates/console-ui/src/utils/mod.rs)
- [x] Add `pub fn format_folder_display_name(path: &str) -> String`.
- [x] Replace custom split/casing in `sidebar_view.rs`, `file_icons.rs`, and `state/run.rs`.

### Task 1.5: Verification
- [x] Run `cargo check` in `apps/desktop`.
- [x] Commit Phase 1 with a single-line commit message.

---

## Phase 2: Modularize `sidebar_view.rs` (1,236 Lines)

Decompose `sidebar_view.rs` into a dedicated module directory: `apps/desktop/crates/console-ui/src/layout/sidebar/`.

```
apps/desktop/crates/console-ui/src/layout/sidebar/
├── mod.rs             # SidebarView RenderOnce, virtualized list assembly, resize handle, footer
├── session_item.rs    # SidebarSessionItem, rename keybindings & actions, context menu trigger
├── draft_item.rs      # DraftSummary, render_sidebar_draft_item
└── group_header.rs    # group_header, drafts_group_header
```

### Task 2.1: Extract Session Item Component (`session_item.rs`)
- **File**: `apps/desktop/crates/console-ui/src/layout/sidebar/session_item.rs`
- [x] Move `SidebarSessionItem` struct and its `RenderOnce` implementation.
- [x] Move inline rename keybinding actions (`CommitSessionRename`, `CancelSessionRename`).
- [x] Move `render_sidebar_session_item` helper and action buttons.

### Task 2.2: Extract Draft Item Component (`draft_item.rs`)
- **File**: `apps/desktop/crates/console-ui/src/layout/sidebar/draft_item.rs`
- [x] Move `DraftSummary` struct.
- [x] Move `render_sidebar_draft_item` function.

### Task 2.3: Extract Collapsible Group Headers (`group_header.rs`)
- **File**: `apps/desktop/crates/console-ui/src/layout/sidebar/group_header.rs`
- [x] Move `group_header` function (chevron rotation, date label, add-project button).
- [x] Move `drafts_group_header` function.

### Task 2.4: Assemble Main Sidebar View (`mod.rs`)
- **File**: `apps/desktop/crates/console-ui/src/layout/sidebar/mod.rs`
- [x] Retain `SidebarView` struct, constructor `SidebarView::new`, and `SidebarRow` enum.
- [x] Retain list virtualization and bottom environments/settings popup footer.
- [x] Update `apps/desktop/crates/console-ui/src/layout/mod.rs` to re-export `SidebarView` and `DraftSummary`.

### Task 2.5: Verification
- [x] Verify `cargo check` passes with 0 warnings.
- [x] Commit Phase 2 with a single-line commit message.

---

## Phase 3: Modularize Desktop State (`state/app.rs` & `state/run.rs`)

### Task 3.1: Extract Workspace Pane State Accessors
- **Target File**: `apps/desktop/src/state/workspace_panes.rs`
- [ ] Extract pane-scoped entity getters/mutators:
  - `ensure_workspace_pane_state`
  - `transcript_for_pane`, `composer_for_pane`, `active_session_for_pane`
  - `set_pane_model`, `set_pane_approval_mode`
  - `sync_project_from_session_for_pane`

### Task 3.2: Extract Execution & Interaction State
- **Target File**: `apps/desktop/src/state/execution.rs`
- [ ] Extract session execution tracking:
  - `is_session_running`, `set_session_running`, `running_sessions_snapshot`
  - Interactive permissions: `set_pending_permission_for_session`, `confirm_pending_permission_for_pane`
  - Interactive questions: `set_pending_question_for_session`, `submit_question_answer_for_pane`
  - Agent notices: `set_agent_notice_for_session`, `set_error_for_session`

### Task 3.3: Extract Draft State Management
- **Target File**: `apps/desktop/src/state/drafts.rs`
- [ ] Move `draft_summaries(&self) -> Vec<DraftSummary>` computation.
- [ ] Move `get_draft_for_session` and `save_draft_for_session`.

### Task 3.4: Modularize Run Event Dispatcher (`state/run.rs`)
- **Directory**: `apps/desktop/src/state/run/`
- [ ] `submission.rs`: Prompt submission, draft clearing, optimistic transcript push.
- [ ] `event_handler.rs`: `process_agent_event` (14 `AgentSessionEvent` branches).
- [ ] `mod.rs`: `attach_session_run_for_pane` and stream render throttling.

### Task 3.5: Verification
- [ ] Run `cargo check` and verify desktop app state builds cleanly.
- [ ] Commit Phase 3 with a single-line commit message.

---

## Phase 4: Modularize Root Desktop View (`apps/desktop/src/view.rs`)

### Task 4.1: Split `view.rs` (1,208 Lines)
- **Directory**: `apps/desktop/src/view/`
- [ ] `mod.rs`: Root window frame `Render for ConsoleDesktopApp`, title bar, and modal dialog overlays.
- [ ] `chat_view.rs`: Workspace chat tab content (`render_workspace_content` chat branch).
- [ ] `terminal_view.rs`: Terminal tab content and empty state rendering.

### Task 4.2: Full Suite & Desktop Verification
- [ ] Run `cargo check` across all workspace crates.
- [ ] Run relevant server/desktop unit tests per `AGENTS.md`.
- [ ] Commit Phase 4 with a single-line commit message.
