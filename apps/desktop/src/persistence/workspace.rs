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
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub active_pane_id: Option<String>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub bottom_terminal_tab_count: Option<usize>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub bottom_terminal_active_idx: Option<usize>,
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

/// Atomically write pre-serialized workspace bytes (compact JSON: this
/// file is rewritten on every tab action, pretty-printing is pure overhead,
/// so callers serialize once and share the bytes with the dirty check).
pub fn save_workspaces_bytes(bytes: &[u8]) {
    let path = workspace_file_path();
    let directory = path.parent().unwrap_or_else(|| Path::new("."));
    if let Err(e) = fs::create_dir_all(directory) {
        log::warn!("Failed to create workspace directory: {e}");
        return;
    }

    let tmp = path.with_extension(format!("json.{}.tmp", std::process::id()));
    if fs::write(&tmp, bytes).is_ok() {
        let _ = fs::rename(tmp, path);
    }
}
