//! Application state and its domain handlers.
//!
//! `ConsoleDesktopApp` is a single gpui entity whose methods are split across
//! one file per concern — projects, image attachments, error surfacing,
//! session bookkeeping, layout persistence, and the run/SSE streaming loop —
//! so each domain stays reviewable without losing shared state.

mod attachments;
mod auth;
mod automations;
mod autocomplete;
mod background_shells;
mod bootstrap;
mod browser_actions;
mod browser_annotations;
mod chrome_import;
mod deleted_sessions;
mod draft_sync;
mod drafts;
mod editor_views;
mod environments;
mod errors;
mod execution;
mod file_comments;
mod file_saves;
mod floating_composer;
mod github;
mod global_actions;
mod integrations;
mod layout;
mod macos_notifications;
mod notifications;
mod onboarding;
mod pagination;
mod port_forward;
mod project_scripts;
mod projects;
mod provider_credentials;
mod providers;
mod repo_tasks;
mod run;
mod server_auth;
mod session_titles;
mod sessions;
mod settings;
mod stt;
mod transcript_scroll;
mod transcript_wiring;
pub mod updates;
mod usage;
mod workspace_context;
mod workspace_panes;
mod workspace_snapshot;
mod workspace_tabs;

mod app;
pub mod types;

pub use types::{ConsoleDesktopApp, FileMediaDetails, ImageFileState};
pub(crate) use workspace_snapshot::PaneChatSnapshot;
pub(crate) use workspace_tabs::new_tab_menu::new_tab_menu_items;
// Shared by the sibling handler modules (`layout`, `sessions`, `run`).
pub(crate) use app::{SIDEBAR_MAX_WIDTH, SIDEBAR_MIN_WIDTH, user_prompt_history};
