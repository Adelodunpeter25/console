pub mod layout;
pub(crate) mod store;
pub mod window;
pub mod workspace;
pub mod workspace_state;

pub use layout::PersistedLayoutState;
pub use workspace::{PersistedWorkspace, WorkspacesDocument, load_workspaces, save_workspaces};
pub use workspace_state::{
    PersistedWindowDescriptor, dedupe_ghost_windows, load_workspace_state, remove_window_descriptor,
    save_workspace_state,
};
