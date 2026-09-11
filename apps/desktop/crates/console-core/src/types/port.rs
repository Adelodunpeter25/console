//! Forwarded port models for local & remote development servers.

use serde::{Deserialize, Serialize};

/// A development port forwarded by the server for client preview.
#[derive(Clone, Debug, PartialEq, Eq, Hash, Serialize, Deserialize)]
pub struct ForwardedPort {
    /// The development server's original port.
    pub port: u16,
    /// Fully-qualified URL to open in a browser or WebView.
    pub url: String,
}

/// Request to manually forward a specific port.
#[derive(Clone, Debug, PartialEq, Eq, Serialize, Deserialize)]
pub struct ForwardPortRequest {
    pub port: u16,
    #[serde(skip_serializing_if = "Option::is_none", rename = "projectId")]
    pub project_id: Option<String>,
}
