use serde::{Deserialize, Serialize};

use super::store;

#[derive(Clone, Debug, Deserialize, Serialize, PartialEq)]
pub struct PersistedLayoutState {
    #[serde(default = "default_sidebar_visible")]
    pub sidebar_visible: bool,
    #[serde(default = "default_sidebar_width")]
    pub sidebar_width: f32,
    /// The project whose workspace is selected, or `None` for the default one.
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub active_workspace_id: Option<String>,
    /// Collapsed project sections (project id, or "none").
    #[serde(default)]
    pub collapsed_projects: Vec<String>,
    #[serde(default)]
    pub pinned_collapsed: bool,
    #[serde(default)]
    pub workspaces_collapsed: bool,
}

fn default_sidebar_visible() -> bool {
    true
}

fn default_sidebar_width() -> f32 {
    260.0
}

impl Default for PersistedLayoutState {
    fn default() -> Self {
        Self {
            sidebar_visible: true,
            sidebar_width: default_sidebar_width(),
            active_workspace_id: None,
            collapsed_projects: Vec::new(),
            pinned_collapsed: false,
            workspaces_collapsed: false,
        }
    }
}

pub fn save(state: PersistedLayoutState) {
    store::save_layout(state);
}
