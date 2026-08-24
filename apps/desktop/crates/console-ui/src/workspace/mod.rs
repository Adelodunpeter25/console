//! The workspace: the pane-tree layout (tabs, splits) that hosts the app's
//! views. Model types live in `console_core::types::workspace`; this module
//! owns the tree operations and the rendering.

pub mod drag;
pub mod empty_state;
pub mod ops;
pub mod pane;
pub mod tab_bar;

pub use drag::{WorkspaceDrag, WorkspaceDragPreview, WorkspaceDropAction, cancel_workspace_drags};
pub use empty_state::EmptyChatState;
pub use ops::{
    active_leaf, close_matching_tabs, close_pane, close_tab, move_tab_to_split, open_tab,
    rename_tabs, select_tab, split_pane,
};
pub use pane::{ContentRenderer, WorkspacePane};
pub use tab_bar::WorkspaceTabBar;
