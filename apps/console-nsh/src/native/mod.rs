//! The app's native boundary, split the way the GPUI app splits it: `http/`
//! is the transport wiring, `services/` one module per console-core service,
//! `types.rs` the plain structs that cross into TSX.

pub mod http;
pub mod services;
pub mod types;

use nsh_core::prelude::*;

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
    /// response body, or an error message if the server is unreachable.
    pub async fn health(base_url: String) -> String {
        http::health(&base_url).await
    }

    /// The registered projects, newest activity first.
    pub async fn projects(base_url: String) -> Vec<ProjectSummary> {
        Projects::new(&base_url).list().await
    }

    /// The chats of one project (or all projects, when `project_id` is
    /// empty), newest first.
    pub async fn sessions(base_url: String, project_id: String) -> Vec<SessionSummary> {
        Sessions::new(&base_url).list(&project_id).await
    }
}
