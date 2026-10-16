//! Shared settings view-models and UI components.

pub mod api_key_section;
pub mod deleted_chats_page;
pub mod github_section;
pub mod integrations_page;
pub mod keybindings_page;
pub mod mcp_page;
pub mod models_page;
pub mod projects_page;
pub mod providers_page;
pub mod servers_page;
pub mod settings_shell;
pub mod usage_page;

pub use api_key_section::*;
pub use deleted_chats_page::*;
pub use github_section::*;
pub use integrations_page::*;
pub use keybindings_page::*;
pub use mcp_page::*;
pub use models_page::*;
pub use projects_page::*;
pub use providers_page::*;
pub use servers_page::*;
pub use settings_shell::*;
pub use usage_page::*;

use serde::{Deserialize, Serialize};

/// Navigation tabs within the Settings Window.
#[derive(Clone, Copy, Debug, Default, Eq, PartialEq, Serialize, Deserialize)]
pub enum SettingsTab {
    #[default]
    Providers,
    Servers,
    Usage,
    Projects,
    Models,
    Integrations,
    Mcp,
    DeletedChats,
    Keybindings,
}

impl SettingsTab {
    pub fn title(&self) -> &'static str {
        match self {
            Self::Providers => "Providers",
            Self::Servers => "Servers",
            Self::Usage => "Usage",
            Self::Projects => "Projects",
            Self::Models => "Models",
            Self::Integrations => "Integrations",
            Self::Mcp => "MCP Servers",
            Self::DeletedChats => "Deleted chats",
            Self::Keybindings => "Keyboard Shortcuts",
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

/// A single environment item in the servers page.
#[derive(Clone, Debug, Eq, PartialEq, Serialize, Deserialize)]
pub struct EnvironmentRow {
    pub id: String,
    pub name: String,
    pub url: String,
    pub is_active: bool,
    pub probe_state: ProbeState,
}
