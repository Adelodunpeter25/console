//! Application state and its domain handlers.
//!
//! `ConsoleDesktopApp` is a single gpui entity whose methods are split across
//! one file per concern — projects, image attachments, error surfacing,
//! session bookkeeping, layout persistence, and the run/SSE streaming loop —
//! so each domain stays reviewable without losing shared state.

mod attachments;
mod auth;
mod autocomplete;
mod deleted_sessions;
mod drafts;
mod environments;
mod errors;
mod execution;
mod global_actions;
mod layout;
mod notifications;
mod pagination;
mod projects;
mod providers;
mod right_sidebar;
mod run;
mod sessions;
mod settings;
mod transcript_wiring;
mod transcript_scroll;
mod usage;
mod workspace_panes;

mod app;

pub use app::ConsoleDesktopApp;
// Shared by the sibling handler modules (`layout`, `sessions`, `run`).
pub(crate) use app::{
    RIGHT_SIDEBAR_BOTTOM_MAX_HEIGHT, RIGHT_SIDEBAR_BOTTOM_MIN_HEIGHT, RIGHT_SIDEBAR_MAX_WIDTH,
    RIGHT_SIDEBAR_MIN_WIDTH, SIDEBAR_MAX_WIDTH, SIDEBAR_MIN_WIDTH, user_prompt_history,
};
