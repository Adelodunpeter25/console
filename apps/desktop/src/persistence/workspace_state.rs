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
    #[serde(default)]
    pub right_sidebar_bottom_collapsed: bool,
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
    pub right_sidebar_bottom_collapsed: bool,
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

/// Remove one window's descriptor (called when its window closes) so
/// closed windows stop accumulating in `workspace-state.json`.
pub fn remove_window_descriptor(id: &str) {
    let mut doc = load_workspace_state();
    let before = doc.windows.len();
    doc.windows.retain(|w| w.id != id);
    if doc.windows.len() != before {
        save_workspace_state(&doc);
    }
}

fn window_id_timestamp(id: &str) -> (u128, u64) {
    let mut parts = id.split('-');
    let _prefix = parts.next();
    let millis = parts.next().and_then(|s| s.parse::<u128>().ok()).unwrap_or(0);
    let count = parts.next().and_then(|s| s.parse::<u64>().ok()).unwrap_or(0);
    (millis, count)
}

/// One-time startup cleanup for pre-fix ghosts: descriptors sharing a
/// workspace whose origins are within a few px are the same visible window
/// saved twice (legit cascade offsets by 30px, ghosts differ by ~1px).
/// Keeps the newest per cluster. No-op for genuinely separate windows.
pub fn dedupe_ghost_windows() {
    let mut doc = load_workspace_state();
    if doc.windows.len() < 2 {
        return;
    }
    const GHOST_PX: f32 = 8.0;
    let mut keep = vec![true; doc.windows.len()];
    // Newest first so the first of each cluster wins.
    let mut order: Vec<usize> = (0..doc.windows.len()).collect();
    order.sort_by_key(|&i| std::cmp::Reverse(window_id_timestamp(&doc.windows[i].id)));
    let mut kept: Vec<usize> = Vec::new();
    for i in order {
        let candidate = &doc.windows[i];
        let is_ghost = kept.iter().any(|&j| {
            let other = &doc.windows[j];
            candidate.active_workspace_id == other.active_workspace_id
                && (candidate.bounds.x - other.bounds.x).abs() <= GHOST_PX
                && (candidate.bounds.y - other.bounds.y).abs() <= GHOST_PX
        });
        if is_ghost {
            keep[i] = false;
        } else {
            kept.push(i);
        }
    }
    if keep.iter().any(|&k| !k) {
        let mut windows = Vec::with_capacity(kept.len());
        for (i, w) in doc.windows.into_iter().enumerate() {
            if keep[i] {
                windows.push(w);
            }
        }
        doc.windows = windows;
        save_workspace_state(&doc);
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
