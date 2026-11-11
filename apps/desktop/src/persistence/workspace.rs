use std::{fs, path::PathBuf};

use console_core::WorkspaceNode;
use serde::{Deserialize, Serialize};

use super::store::storage_directory;

const WORKSPACES_VERSION: u32 = 1;

#[derive(Clone, Debug, Default, Serialize, Deserialize, PartialEq, Eq)]
#[serde(rename_all = "camelCase")]
pub struct PersistedScriptTabs {
    #[serde(default)]
    pub open_script_ids: Vec<String>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub active_script_id: Option<String>,
}

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
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub script_tabs: Option<PersistedScriptTabs>,
}

#[derive(Clone, Debug, Default, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct WorkspacesDocument {
    pub version: u32,
    #[serde(default)]
    pub workspaces: Vec<PersistedWorkspace>,
}

/// The pre-v2 file that held the workspace tree. The tree is a section of
/// `state.json` now; this file is only read while it still exists.
pub(crate) fn legacy_path() -> PathBuf {
    storage_directory().join("workspace.json")
}

pub(crate) fn read_legacy() -> Option<WorkspacesDocument> {
    let contents = fs::read_to_string(legacy_path()).ok()?;
    let document = serde_json::from_str::<WorkspacesDocument>(&contents).ok()?;
    (document.version == WORKSPACES_VERSION).then_some(document)
}
