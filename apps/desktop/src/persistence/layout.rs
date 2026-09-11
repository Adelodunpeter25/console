use serde::{Deserialize, Serialize};

use super::store;

#[derive(Clone, Debug, Deserialize, Serialize, PartialEq)]
pub struct PersistedLayoutState {
    #[serde(default = "default_sidebar_visible")]
    pub sidebar_visible: bool,
    #[serde(default = "default_sidebar_width")]
    pub sidebar_width: f32,
    #[serde(default = "default_right_sidebar_visible")]
    pub right_sidebar_visible: bool,
    #[serde(default = "default_right_sidebar_width")]
    pub right_sidebar_width: f32,
    #[serde(default = "default_right_sidebar_bottom_height")]
    pub right_sidebar_bottom_height: f32,
    /// Indices into `SessionDateGroup::ALL` of collapsed sidebar groups.
    #[serde(default)]
    pub collapsed_groups: Vec<usize>,
    /// Sidebar session sort axis: "date" (default) or "project".
    #[serde(default)]
    pub sidebar_sort_mode: String,
    /// Collapsed project sections (project id, or "none") in project sort mode.
    #[serde(default)]
    pub collapsed_projects: Vec<String>,
    /// Auxiliary inspector tabs mounted in the right-sidebar tab strip.
    #[serde(default)]
    pub open_auxiliary_tabs: Vec<console_ui::AuxiliaryTab>,
}

impl PersistedLayoutState {
    pub fn open_auxiliary_tabs(&self) -> Vec<console_ui::AuxiliaryTab> {
        self.open_auxiliary_tabs.clone()
    }

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

fn default_right_sidebar_visible() -> bool {
    false
}

fn default_right_sidebar_width() -> f32 {
    280.0
}

fn default_right_sidebar_bottom_height() -> f32 {
    180.0
}

impl Default for PersistedLayoutState {
    fn default() -> Self {
        Self {
            sidebar_visible: true,
            sidebar_width: default_sidebar_width(),
            right_sidebar_visible: default_right_sidebar_visible(),
            right_sidebar_width: default_right_sidebar_width(),
            right_sidebar_bottom_height: default_right_sidebar_bottom_height(),
            collapsed_groups: Vec::new(),
            sidebar_sort_mode: String::new(),
            collapsed_projects: Vec::new(),
            open_auxiliary_tabs: Vec::new(),
        }
    }
}

pub fn save(state: PersistedLayoutState) {
    store::save_layout(state);
}
