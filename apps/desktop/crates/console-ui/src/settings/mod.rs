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
pub mod server_modal;
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
pub use server_modal::*;
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
            Self::Providers => "AI Providers",
            Self::Servers => "Environments",
            Self::Usage => "Usage & Activity",
            Self::Projects => "Projects",
            Self::Models => "Model Roles",
            Self::Integrations => "Integrations",
            Self::Mcp => "MCP Servers",
            Self::DeletedChats => "Deleted Chats",
            Self::Keybindings => "Keyboard Shortcuts",
        }
    }

    pub fn as_str(&self) -> &'static str {
        match self {
            Self::Providers => "providers",
            Self::Servers => "servers",
            Self::Usage => "usage",
            Self::Projects => "projects",
            Self::Models => "models",
            Self::Integrations => "integrations",
            Self::Mcp => "mcp",
            Self::DeletedChats => "deleted-chats",
            Self::Keybindings => "keybindings",
        }
    }

    pub fn from_id(id: &str) -> Option<Self> {
        match id {
            "providers" => Some(Self::Providers),
            "servers" => Some(Self::Servers),
            "usage" => Some(Self::Usage),
            "projects" => Some(Self::Projects),
            "models" => Some(Self::Models),
            "integrations" => Some(Self::Integrations),
            "mcp" => Some(Self::Mcp),
            "deleted-chats" | "deletedChats" => Some(Self::DeletedChats),
            "keybindings" => Some(Self::Keybindings),
            _ => None,
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
