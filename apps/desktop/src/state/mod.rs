//! Application state and its domain handlers.
//!
//! `ConsoleDesktopApp` is a single gpui entity whose methods are split across
//! one file per concern — projects, image attachments, error surfacing,
//! session bookkeeping, layout persistence, and the run/SSE streaming loop —
//! so each domain stays reviewable without losing shared state.

mod attachments;
mod autocomplete;
mod errors;
mod layout;
mod projects;
mod providers;
mod run;
mod sessions;
mod transcript_scroll;

mod app;

pub use app::ConsoleDesktopApp;
// Shared by the sibling handler modules (`layout`, `sessions`, `run`).
pub(crate) use app::{SIDEBAR_MAX_WIDTH, SIDEBAR_MIN_WIDTH, user_prompt_history};
