pub mod auth;
pub mod layout;
pub mod store;
pub mod window;
pub mod workspace;
pub mod workspace_state;

pub use auth::{
    StoredServerAuth, clear as clear_server_auth, load_for as load_server_auth,
    save as save_server_auth, token_for as server_auth_token,
};
pub use layout::PersistedLayoutState;
pub use store::{dismiss_onboarding, save_workspaces};
pub use workspace::{PersistedScriptTabs, PersistedWorkspace, WorkspacesDocument};

/// Everything the app reads from disk when a window opens.
///
/// The state document holds the window frame, the sidebar layout, the
/// servers, the workspace tree and the onboarding flag; the drafts live in
/// their own file. Reading them together keeps the startup path to one read
/// of the state file instead of three.
pub struct StartupState {
    pub window: Option<window::PersistedWindowState>,
    pub layout: PersistedLayoutState,
    pub environments: Option<store::PersistedEnvironmentsState>,
    pub onboarding_seen: bool,
    pub workspaces: WorkspacesDocument,
    pub drafts: std::collections::HashMap<String, store::PersistedDraft>,
    /// Per-chat transcript scroll positions, so a chat reopens where it was
    /// left rather than at the top.
    pub transcript_scroll: Option<store::PersistedTranscriptScrollState>,
}

/// Read the state the app starts from.
pub fn load_startup() -> StartupState {
    let document = store::read_document();
    let drafts = store::read_drafts();
    StartupState {
        window: document.window,
        layout: document.layout.unwrap_or_default(),
        environments: document.environments,
        onboarding_seen: document
            .onboarding
            .map(|state| state.seen || !state.dismissed.is_empty())
            .unwrap_or(false),
        workspaces: document.workspaces.unwrap_or_default(),
        drafts: drafts.drafts,
        transcript_scroll: document.transcript_scroll,
    }
}
