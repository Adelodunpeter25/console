//! Shared settings view-models and UI components.

pub mod accounts_page;
pub mod connection_page;
pub mod deleted_chats_page;
pub mod projects_page;
pub mod settings_shell;

pub use accounts_page::*;
pub use connection_page::*;
pub use deleted_chats_page::*;
pub use projects_page::*;
pub use settings_shell::*;

use serde::{Deserialize, Serialize};

/// Navigation tabs within the Settings Window.
#[derive(Clone, Copy, Debug, Default, Eq, PartialEq, Serialize, Deserialize)]
pub enum SettingsTab {
    #[default]
    Accounts,
    Connection,
    Projects,
    DeletedChats,
}

impl SettingsTab {
    pub fn title(&self) -> &'static str {
        match self {
            Self::Accounts => "Accounts",
            Self::Connection => "Connection",
            Self::Projects => "Projects",
            Self::DeletedChats => "Deleted chats",
        }
    }
}

/// Reachability probe state for an environment.
#[derive(Clone, Copy, Debug, Default, Eq, PartialEq, Serialize, Deserialize)]
pub enum ProbeState {
    #[default]
    Unknown,
    Probing,
    Ok,
    Failed,
}

/// A single environment item in the connection page.
#[derive(Clone, Debug, Eq, PartialEq, Serialize, Deserialize)]
pub struct EnvironmentRow {
    pub id: String,
    pub name: String,
    pub url: String,
    pub is_active: bool,
    pub probe_state: ProbeState,
}
