use serde::{Deserialize, Serialize};

use super::store;

#[derive(Clone, Debug, Deserialize, Serialize, PartialEq)]
pub struct PersistedLayoutState {
    #[serde(default = "default_sidebar_visible")]
    pub sidebar_visible: bool,
    #[serde(default = "default_sidebar_width")]
    pub sidebar_width: f32,
    /// Indices into `SessionDateGroup::ALL` of collapsed sidebar groups.
    #[serde(default)]
    pub collapsed_groups: Vec<usize>,
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
            collapsed_groups: Vec::new(),
        }
    }
}

pub fn load() -> PersistedLayoutState {
    store::load_layout().unwrap_or_default()
}

pub fn save(state: PersistedLayoutState) {
    store::save_layout(state);
}
