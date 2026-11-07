//! The app's native boundary, split the way the GPUI app splits it: `http/`
//! is the transport wiring, `services/` one module per console-core service,
//! `types.rs` the plain structs that cross into TSX.

pub mod http;
pub mod services;
pub mod types;

use nsh_core::prelude::*;
use nsh_core::JsonValue;

use services::chat::Chat;
use services::workspace::WorkspacePersist;
use services::{projects::Projects, sessions::Sessions};
use types::{ProjectSummary, SessionSummary};

/// The Console backend, reached through `console-core`'s services. Every
/// function takes the backend's base URL; the desktop app's server discovery
/// replaces that when its state moves over.
#[module]
pub struct Console;

#[module]
impl Console {
    /// Fetches `{base_url}/health` through console-core and returns the
    /// response body; rejects when the server is unreachable.
    pub async fn health(base_url: String) -> Result<String, String> {
        http::health(&base_url).await
    }

    /// The registered projects, newest activity first.
    pub async fn projects(base_url: String) -> Result<Vec<ProjectSummary>, String> {
        Projects::new(&base_url).list().await
    }

    /// The chats of one project (or all projects, when `project_id` is
    /// empty), newest first.
    pub async fn sessions(base_url: String, project_id: String) -> Result<Vec<SessionSummary>, String> {
        Sessions::new(&base_url).list(&project_id).await
    }

    /// Saves one workspace's layout document (the pane-tree JSON, owned by
    /// the TSX store). See `services::workspace` for the persistence
    /// contract.
    pub fn save_workspace_layout(key: String, document: String) -> Result<(), String> {
        WorkspacePersist::save(&key, &document)
    }

    /// Loads one workspace's layout document, or `None` when the workspace
    /// was never saved (a fresh project's first launch).
    pub fn load_workspace_layout(key: String) -> Result<Option<String>, String> {
        Ok(WorkspacePersist::load(&key))
    }

    /// Deletes one workspace's layout document (the workspace was closed or
    /// its worktree removed). Missing keys are fine.
    pub fn delete_workspace_layout(key: String) -> Result<(), String> {
        WorkspacePersist::delete(&key);
        Ok(())
    }

    /// Starts one agent run and forwards each `AgentSessionEvent` to
    /// `on_event` as camelCase JSON. The promise settles when the run's
    /// stream ends, so awaiting it is the run's lifecycle.
    pub async fn run_prompt(
        base_url: String,
        session_id: String,
        prompt: JsonValue,
        on_event: Callback<JsonValue, ()>,
    ) -> Result<(), String> {
        Chat::run_prompt(&base_url, &session_id, prompt, on_event).await
    }

    /// Reattaches to a run already in flight (an app restart mid-run), from
    /// `since` events in.
    pub async fn attach_run(
        base_url: String,
        session_id: String,
        since: Option<u64>,
        on_event: Callback<JsonValue, ()>,
    ) -> Result<(), String> {
        Chat::attach_run(&base_url, &session_id, since, on_event).await
    }

    /// One session's stored message history, as the server serializes it.
    pub async fn chat_messages(base_url: String, session_id: String) -> Result<String, String> {
        Chat::messages(&base_url, &session_id).await
    }

    /// Creates a session (the launcher's submit) and returns its header.
    pub async fn create_session(
        base_url: String,
        project_id: String,
        title: Option<String>,
    ) -> Result<String, String> {
        Chat::create_session(&base_url, &project_id, title).await
    }
}
