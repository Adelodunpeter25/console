use std::{fs, path::PathBuf};

use serde::{Deserialize, Serialize};

use super::store::storage_directory;

const WORKSPACE_STATE_VERSION: u32 = 1;

#[derive(Clone, Debug, Default, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct WorkspaceStateDocument {
    pub version: u32,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub active_workspace_id: Option<String>,
    #[serde(default)]
    pub sidebar_visible: bool,
    #[serde(default)]
    pub sidebar_width: f32,
    #[serde(default)]
    pub right_sidebar_visible: bool,
    #[serde(default)]
    pub right_sidebar_width: f32,
    #[serde(default = "default_right_sidebar_bottom_height")]
    pub right_sidebar_bottom_height: f32,
    #[serde(default)]
    pub right_sidebar_bottom_collapsed: bool,
}

fn default_right_sidebar_bottom_height() -> f32 {
    180.0
}

/// The pre-v2 file that held the sidebar layout. The layout is a section of
/// `state.json` now; this file is only read while it still exists.
pub(crate) fn legacy_path() -> PathBuf {
    storage_directory().join("workspace-state.json")
}

pub(crate) fn read_legacy() -> Option<WorkspaceStateDocument> {
    let contents = fs::read_to_string(legacy_path()).ok()?;
    let document = serde_json::from_str::<WorkspaceStateDocument>(&contents).ok()?;
    (document.version == WORKSPACE_STATE_VERSION).then_some(document)
}
