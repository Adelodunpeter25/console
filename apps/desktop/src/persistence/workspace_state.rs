use std::{fs, path::Path, path::PathBuf};

use serde::{Deserialize, Serialize};

use super::store::storage_directory;
use super::window::PersistedWindowState;

const WORKSPACE_STATE_VERSION: u32 = 1;

#[derive(Clone, Debug, PartialEq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct PersistedWindowDescriptor {
    #[serde(default)]
    pub id: String,
    pub bounds: PersistedWindowState,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub active_workspace_id: Option<String>,
    #[serde(default = "default_sidebar_visible")]
    pub sidebar_visible: bool,
    #[serde(default = "default_sidebar_width")]
    pub sidebar_width: f32,
    #[serde(default)]
    pub right_sidebar_visible: bool,
    #[serde(default = "default_right_sidebar_width")]
    pub right_sidebar_width: f32,
    #[serde(default = "default_right_sidebar_bottom_height")]
    pub right_sidebar_bottom_height: f32,
}

fn default_sidebar_visible() -> bool {
    true
}

fn default_sidebar_width() -> f32 {
    260.0
}

fn default_right_sidebar_width() -> f32 {
    280.0
}

fn default_right_sidebar_bottom_height() -> f32 {
    180.0
}

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
    pub windows: Vec<PersistedWindowDescriptor>,
}

fn workspace_state_file_path() -> PathBuf {
    storage_directory().join("workspace-state.json")
}

pub fn load_workspace_state() -> WorkspaceStateDocument {
    let Ok(contents) = fs::read_to_string(workspace_state_file_path()) else {
        return WorkspaceStateDocument {
            version: WORKSPACE_STATE_VERSION,
            sidebar_visible: true,
            sidebar_width: 260.0,
            right_sidebar_visible: false,
            right_sidebar_width: 280.0,
            right_sidebar_bottom_height: 180.0,
            ..Default::default()
        };
    };

    match serde_json::from_str::<WorkspaceStateDocument>(&contents) {
        Ok(doc) if doc.version == WORKSPACE_STATE_VERSION => doc,
        _ => WorkspaceStateDocument {
            version: WORKSPACE_STATE_VERSION,
            sidebar_visible: true,
            sidebar_width: 260.0,
            right_sidebar_visible: false,
            right_sidebar_width: 280.0,
            right_sidebar_bottom_height: 180.0,
            ..Default::default()
        },
    }
}

pub fn save_workspace_state(doc: &WorkspaceStateDocument) {
    let path = workspace_state_file_path();
    let directory = path.parent().unwrap_or_else(|| Path::new("."));
    if let Err(e) = fs::create_dir_all(directory) {
        log::warn!("Failed to create workspace-state directory: {e}");
        return;
    }

    let tmp = path.with_extension(format!("json.{}.tmp", std::process::id()));
    let mut to_save = doc.clone();
    to_save.version = WORKSPACE_STATE_VERSION;
    if let Ok(bytes) = serde_json::to_vec_pretty(&to_save) {
        if fs::write(&tmp, bytes).is_ok() {
            let _ = fs::rename(tmp, path);
        }
    }
}
