//! App-owned structural types. `WorkspacePaneState` is the per-split bundle
//! of UI entities and session settings; it sits beside `console-core`'s
//! `WorkspaceNode` semantics rather than inline in `state/app.rs`.

use std::rc::Rc;

use console_core::{ApprovalMode, GitBranchInfo, SelectedModel};
use console_ui::{ComposerInput, ContextMenuHandle, PickerTab, TranscriptView};
use gpui::Entity;

/// Everything a single workspace split needs to render its chat: the
/// transcript/composer/question entities plus the picker, approval, project,
/// branch, and model-search controls. Keyed by pane id in
/// [`crate::state::ConsoleDesktopApp::workspace_pane_states`].
pub(crate) struct WorkspacePaneState {
    pub(crate) transcript_view: Entity<TranscriptView>,
    pub(crate) composer_input: Entity<ComposerInput>,
    /// Answer field mounted in this pane's question card. Per-pane so typing
    /// in one split never mutates another split's pending question.
    pub(crate) question_input: Entity<ComposerInput>,
    pub(crate) selected_model: Option<SelectedModel>,
    pub(crate) active_picker_tab: PickerTab,
    pub(crate) approval_mode: ApprovalMode,
    pub(crate) model_menu: ContextMenuHandle,
    pub(crate) approval_menu: ContextMenuHandle,
    pub(crate) selected_project_id: Option<String>,
    pub(crate) branches: Rc<Vec<GitBranchInfo>>,
    pub(crate) branch_loaded: bool,
    pub(crate) branch_is_git_repository: bool,
    pub(crate) branch_pending: bool,
    pub(crate) project_menu: ContextMenuHandle,
    pub(crate) branch_menu: ContextMenuHandle,
    /// One-line search field inside the model picker popover. The query is
    /// read each frame in `view.rs` to filter the model list; edits notify the
    /// app so the dropdown re-renders, and the field is cleared and focused
    /// each time the popover opens.
    pub(crate) model_search: Entity<ComposerInput>,
    /// The session id whose messages are currently loaded in this pane's transcript.
    pub(crate) loaded_session_id: Option<String>,
}
