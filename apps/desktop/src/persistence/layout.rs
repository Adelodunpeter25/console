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
    /// Indices into `SessionDateGroup::ALL` of collapsed sidebar groups.
    #[serde(default)]
    pub collapsed_groups: Vec<usize>,
    /// Sidebar session sort axis: "date" (default) or "project".
    #[serde(default)]
    pub sidebar_sort_mode: String,
    /// Collapsed project sections (project id, or "none") in project sort mode.
    #[serde(default)]
    pub collapsed_projects: Vec<String>,
    #[serde(default)]
    pub pinned_collapsed: bool,
}

impl PersistedLayoutState {
    pub fn sidebar_sort_mode(&self) -> console_ui::utils::SidebarSortMode {
        match self.sidebar_sort_mode.as_str() {
            "project" => console_ui::utils::SidebarSortMode::Project,
            _ => console_ui::utils::SidebarSortMode::Date,
        }
    }
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
            collapsed_groups: Vec::new(),
            sidebar_sort_mode: String::new(),
            collapsed_projects: Vec::new(),
            pinned_collapsed: false,
        }
    }
}

pub fn save(state: PersistedLayoutState) {
    store::save_layout(state);
}
