use std::{fs, path::Path, path::PathBuf};

use console_core::WorkspaceNode;
use serde::{Deserialize, Serialize};

use super::store::storage_directory;

const WORKSPACES_VERSION: u32 = 1;

#[derive(Clone, Debug, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct PersistedWorkspace {
    pub id: String,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub project_id: Option<String>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub cwd: Option<String>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub name: Option<String>,
    pub root: WorkspaceNode,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub active_tab_id: Option<String>,
}

#[derive(Clone, Debug, Default, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct WorkspacesDocument {
    pub version: u32,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub active_workspace_id: Option<String>,
    #[serde(default)]
    pub workspaces: Vec<PersistedWorkspace>,
}

fn workspace_file_path() -> PathBuf {
    storage_directory().join("workspace.json")
}

pub fn load_workspaces() -> WorkspacesDocument {
    let Ok(contents) = fs::read_to_string(workspace_file_path()) else {
        return WorkspacesDocument {
            version: WORKSPACES_VERSION,
            ..Default::default()
        };
    };

    match serde_json::from_str::<WorkspacesDocument>(&contents) {
        Ok(doc) if doc.version == WORKSPACES_VERSION => doc,
        _ => WorkspacesDocument {
            version: WORKSPACES_VERSION,
            ..Default::default()
        },
    }
}

pub fn save_workspaces(doc: &WorkspacesDocument) {
    let path = workspace_file_path();
    let directory = path.parent().unwrap_or_else(|| Path::new("."));
    if let Err(e) = fs::create_dir_all(directory) {
        log::warn!("Failed to create workspace directory: {e}");
        return;
    }

    let tmp = path.with_extension(format!("json.{}.tmp", std::process::id()));
    let mut to_save = doc.clone();
    to_save.version = WORKSPACES_VERSION;
    if let Ok(bytes) = serde_json::to_vec_pretty(&to_save) {
        if fs::write(&tmp, bytes).is_ok() {
            let _ = fs::rename(tmp, path);
        }
    }
}
