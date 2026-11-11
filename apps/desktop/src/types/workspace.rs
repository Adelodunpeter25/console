//! App-owned structural types. `WorkspacePaneState` is the per-split bundle
//! of UI entities and session settings; it sits beside `console-core`'s
//! `WorkspaceNode` semantics rather than inline in `state/app.rs`.

use std::rc::Rc;

use console_core::{ApprovalMode, GitBranchInfo, SelectedModel, ThinkingLevel};
use console_ui::workspace::TabStripFollow;
use console_ui::{ComposerInput, ContextMenuHandle, PickerTab, TranscriptView};
use gpui::Entity;

/// One staged line comment in a pane. A draft is the card opened by a gutter
/// drag; committing it keeps the same card in the `Added` state and stages it
/// in the pane's composer.
pub(crate) struct FileComment {
    pub(crate) id: u64,
    /// Stable across devices: the key the synced draft identifies it by.
    pub(crate) sync_id: String,
    /// Absolute path; prompts render it project-relative.
    pub(crate) path: String,
    pub(crate) start_line: u32,
    pub(crate) end_line: u32,
    /// Committed body, shown in the `Added` card.
    pub(crate) text: String,
    pub(crate) input: Entity<ComposerInput>,
    pub(crate) state: FileCommentState,
    /// Focus the input on the next frame (new draft or Edit clicked).
    pub(crate) focus_pending: bool,
    /// Owned by the comment so removing it also drops the input subscription
    /// instead of leaking one per comment into the app-wide list.
    pub(crate) _subscription: gpui::Subscription,
}

#[derive(Clone, Copy, Debug, PartialEq, Eq)]
pub(crate) enum FileCommentState {
    Draft,
    Added,
    Editing,
}

impl FileComment {
    /// `address.rs +23-35` (or `+23` for one line), the chip/card label.
    pub(crate) fn label(&self) -> String {
        let name = std::path::Path::new(&self.path)
            .file_name()
            .and_then(|name| name.to_str())
            .unwrap_or(&self.path);
        if self.start_line == self.end_line {
            format!("{name} +{}", self.start_line)
        } else {
            format!("{name} +{}-{}", self.start_line, self.end_line)
        }
    }
}

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
    pub(crate) thinking_level: Option<ThinkingLevel>,
    pub(crate) model_menu: ContextMenuHandle,
    pub(crate) thinking_menu: ContextMenuHandle,
    pub(crate) usage_menu: ContextMenuHandle,
    pub(crate) tasks_menu: ContextMenuHandle,
    pub(crate) subagents_menu: ContextMenuHandle,
    pub(crate) shells_menu: ContextMenuHandle,
    pub(crate) selected_project_id: Option<String>,
    pub(crate) branches: Rc<Vec<GitBranchInfo>>,
    pub(crate) branch_loaded: bool,
    pub(crate) branch_is_git_repository: bool,
    /// Folder `branches` was loaded for (the session's own cwd, which for a
    /// worktree session differs from its project path). A response for any
    /// other folder is stale and dropped.
    pub(crate) branch_cwd: Option<String>,
    pub(crate) branch_request_seq: u64,
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
    /// Line comments staged for this pane's next message. They stay here
    /// (with their inline cards) until removed, and every send includes the
    /// committed ones.
    pub(crate) file_comments: Vec<FileComment>,
    pub(crate) next_file_comment_id: u64,
    /// Retained tab strip scroll-follow state for this pane.
    pub(crate) tab_strip_follow: TabStripFollow,
}
