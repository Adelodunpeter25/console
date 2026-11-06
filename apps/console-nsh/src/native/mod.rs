//! The app's native boundary, split the way the GPUI app splits it: `http/`
//! is the transport wiring, `services/` one module per console-core service,
//! `types.rs` the plain structs that cross into TSX.

pub mod http;
pub mod services;
pub mod types;

use nsh_core::prelude::*;

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
}
