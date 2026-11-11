use std::{
    fs,
    path::{Path, PathBuf},
};

use serde::{Deserialize, Serialize};

use super::layout::PersistedLayoutState;
use super::window::PersistedWindowState;
use super::workspace::WorkspacesDocument;
use super::workspace_state;

/// On-disk shape of `state.json`.
///
/// v1 kept the drafts in this file and the workspace tree and the sidebar
/// layout in `workspace.json` and `workspace-state.json`. v2 makes them
/// sections of this one file, so a launch reads one document instead of three.
const STORAGE_VERSION: u32 = 2;

#[derive(Clone, Debug, Serialize, Deserialize)]
pub struct PersistedEnvironment {
    pub id: String,
    pub name: String,
    pub url: String,
}

#[derive(Clone, Debug, Default, Serialize, Deserialize)]
pub struct PersistedEnvironmentsState {
    pub environments: Vec<PersistedEnvironment>,
    pub active_id: Option<String>,
}

#[derive(Clone, Debug, PartialEq, Eq, Serialize, Deserialize)]
pub struct PersistedDraftMention {
    pub start: usize,
    pub end: usize,
    pub path: String,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub label: Option<String>,
}

#[derive(Clone, Debug, Serialize, Deserialize)]
pub struct PersistedDraft {
    pub prompt: String,
    pub updated_at: i64,
    #[serde(default)]
    pub mentions: Vec<PersistedDraftMention>,
    #[serde(default)]
    pub context_files: Vec<String>,
    #[serde(default)]
    pub attachments: Vec<console_core::ImageAttachment>,
    #[serde(default)]
    pub annotations: Vec<console_core::BrowserElementAnnotation>,
}

#[derive(Clone, Debug, Default, Serialize, Deserialize)]
pub struct PersistedDraftsState {
    pub drafts: std::collections::HashMap<String, PersistedDraft>,
}

/// Which servers have already been shown the first-run setup panel.
///
/// One bit per server, keyed by environment id — the same stable ids
/// `PersistedEnvironmentsState` uses (`default-local` is a literal, not a
/// generated one). A set rather than a map because presence *is* the whole
/// fact; there is no per-step progress to record.
///
/// Deliberately does NOT track whether a server is "configured". That is
/// derived live from the server every time the panel opens (projects, provider
/// key, GitHub key), so this can never disagree with reality: a user who
/// deletes their last project sees that step go back to unmet without the tour
/// re-opening, because the tour only records that it was dismissed once.
#[derive(Clone, Debug, Default, Serialize, Deserialize)]
pub struct PersistedOnboardingState {
    /// True once the setup panel has been skipped.
    ///
    /// One bit for the whole app: the panel is a first-run screen, not a
    /// per-server checklist, and a server added later is not a reason to
    /// show it again.
    #[serde(default)]
    pub seen: bool,
    /// Older builds stored a per-server set of dismissed ids. Any entry
    /// means the panel was seen, so it is read as "seen" too.
    #[serde(default)]
    pub dismissed: std::collections::HashSet<String>,
}

/// The one document the app keeps on disk.
///
/// The workspace tree and the sidebar layout used to live in `workspace.json`
/// and `workspace-state.json`; they are sections here now, so a launch reads
/// this file once instead of three times.
#[derive(Clone, Debug, Default, Serialize, Deserialize)]
pub(crate) struct StorageDocument {
    #[serde(default)]
    pub(crate) version: u32,
    #[serde(default)]
    pub(crate) window: Option<PersistedWindowState>,
    #[serde(default)]
    pub(crate) layout: Option<PersistedLayoutState>,
    #[serde(default)]
    pub(crate) environments: Option<PersistedEnvironmentsState>,
    #[serde(default)]
    pub(crate) onboarding: Option<PersistedOnboardingState>,
    #[serde(default)]
    pub(crate) workspaces: Option<WorkspacesDocument>,
    /// Drafts live in `drafts.json`, so a layout or workspace write never
    /// rewrites the images a draft can carry. Read only: this is where
    /// pre-split documents kept them, and it is dropped on the next write.
    #[serde(default, skip_serializing)]
    pub(crate) drafts: Option<PersistedDraftsState>,
}

pub(crate) fn storage_directory() -> PathBuf {
    console_shared::path::storage_directory()
}

fn storage_path() -> PathBuf {
    storage_directory().join("state.json")
}

fn drafts_path() -> PathBuf {
    storage_directory().join("drafts.json")
}

/// Read the state document.
///
/// The pre-v2 files are folded in while they still exist, so a launch sees
/// the same workspace tree and sidebar layout it saw before the merge.
pub(crate) fn read_document() -> StorageDocument {
    let mut document = read_state_file();
    fold_legacy_files(&mut document);
    document
}

fn read_state_file() -> StorageDocument {
    let Ok(contents) = fs::read_to_string(storage_path()) else {
        return StorageDocument {
            version: STORAGE_VERSION,
            ..Default::default()
        };
    };

    match serde_json::from_str::<StorageDocument>(&contents) {
        // v1 is accepted as-is: every section it wrote still exists, and the
        // drafts it carries are read from here until they move to their file.
        Ok(mut document) if document.version == STORAGE_VERSION || document.version == 1 => {
            document.version = STORAGE_VERSION;
            document
        }
        _ => StorageDocument {
            version: STORAGE_VERSION,
            ..Default::default()
        },
    }
}

/// Fold the pre-v2 files into the state document.
///
/// `workspace.json` held the workspace tree and `workspace-state.json` the
/// sidebar layout. The legacy files win for the fields they held, because
/// that is what the app read before the merge; the layout-only fields stay as
/// the state document has them.
fn fold_legacy_files(document: &mut StorageDocument) {
    if document.workspaces.is_none() {
        if let Some(workspaces) = super::workspace::read_legacy() {
            document.workspaces = Some(workspaces);
        }
    }

    let Some(legacy) = workspace_state::read_legacy() else {
        return;
    };
    let mut layout = document.layout.clone().unwrap_or_default();
    layout.sidebar_visible = legacy.sidebar_visible;
    layout.sidebar_width = legacy.sidebar_width;
    layout.right_sidebar_visible = legacy.right_sidebar_visible;
    layout.right_sidebar_width = legacy.right_sidebar_width;
    layout.right_sidebar_bottom_height = legacy.right_sidebar_bottom_height;
    layout.right_sidebar_bottom_collapsed = legacy.right_sidebar_bottom_collapsed;
    layout.active_workspace_id = legacy.active_workspace_id;
    document.layout = Some(layout);
}

/// Drop the pre-v2 files once the state document holds their content.
fn remove_legacy_files() {
    for path in [
        super::workspace::legacy_path(),
        workspace_state::legacy_path(),
    ] {
        match fs::remove_file(&path) {
            Ok(()) => {}
            Err(error) if error.kind() == std::io::ErrorKind::NotFound => {}
            Err(error) => {
                log::warn!("Failed to remove legacy state file {path:?}: {error}");
            }
        }
    }
}

fn write_document(document: &StorageDocument) -> std::io::Result<()> {
    let path = storage_path();
    let directory = path.parent().unwrap_or_else(|| Path::new("."));
    fs::create_dir_all(directory)?;

    let temporary_path = path.with_extension(format!("json.{}.tmp", std::process::id()));
    // Compact JSON: this file is never hand-read and is rewritten often
    // (layout saves on every resize); pretty-printing costs ~2-3x size+time.
    let contents = serde_json::to_vec(document).map_err(std::io::Error::other)?;
    fs::write(&temporary_path, contents)?;
    fs::rename(temporary_path, path)?;
    // The pre-v2 files are redundant now that the state document holds their
    // content. Removing them after the write (never before) means a crash
    // mid-migration cannot lose either copy.
    remove_legacy_files();
    Ok(())
}

fn update_document(update: impl FnOnce(&mut StorageDocument)) {
    let mut document = read_document();
    update(&mut document);
    document.version = STORAGE_VERSION;
    if let Err(error) = write_document(&document) {
        log::warn!("Failed to persist console state: {error}");
    }
}

/// Read the drafts document.
///
/// Drafts live in their own file so a layout or workspace write never
/// rewrites the images a draft can carry. Documents written before the split
/// kept them in `state.json`, so that is the fallback — and the first read
/// moves them across, so dropping the old copy cannot lose them.
pub(crate) fn read_drafts() -> PersistedDraftsState {
    match fs::read_to_string(drafts_path()) {
        Ok(contents) => return serde_json::from_str(&contents).unwrap_or_default(),
        // A drafts file that cannot be read is left alone rather than
        // overwritten: the next save would replace it with nothing.
        Err(error) if error.kind() != std::io::ErrorKind::NotFound => {
            log::warn!("Failed to read drafts: {error}");
            return PersistedDraftsState::default();
        }
        Err(_) => {}
    }

    let legacy = read_state_file().drafts.unwrap_or_default();
    if let Err(error) = write_drafts_file(&legacy) {
        log::warn!("Failed to persist drafts: {error}");
    }
    legacy
}

fn write_drafts_file(state: &PersistedDraftsState) -> std::io::Result<()> {
    let path = drafts_path();
    let directory = path.parent().unwrap_or_else(|| Path::new("."));
    fs::create_dir_all(directory)?;

    let temporary_path = path.with_extension(format!("json.{}.tmp", std::process::id()));
    let contents = serde_json::to_vec(state).map_err(std::io::Error::other)?;
    fs::write(&temporary_path, contents)?;
    fs::rename(temporary_path, path)
}

pub fn save_window(state: PersistedWindowState) {
    update_document(|document| document.window = Some(state));
}

pub fn save_layout(state: PersistedLayoutState) {
    update_document(|document| document.layout = Some(state));
}

pub fn save_workspaces(state: WorkspacesDocument) {
    update_document(|document| document.workspaces = Some(state));
}

pub fn load_environments() -> Option<PersistedEnvironmentsState> {
    read_document().environments
}

/// Mark one server's setup panel as dismissed, leaving every other server's
/// state alone.
pub fn dismiss_onboarding() {
    update_document(|document| {
        let mut state = document.onboarding.clone().unwrap_or_default();
        state.seen = true;
        document.onboarding = Some(state);
    });
}

pub fn save_environments(state: PersistedEnvironmentsState) {
    update_document(|document| document.environments = Some(state));
}

/// List-only save for secondary windows: persists added/removed/edited
/// servers without clobbering the main window's boot server (`active_id`).
/// Runs as a single read-modify-write so the preserved `active_id` cannot
/// race with a concurrent main-window switch.
pub fn save_environment_list_preserving_active(environments: Vec<PersistedEnvironment>) {
    update_document(|document| {
        let active_id = document
            .environments
            .as_ref()
            .and_then(|state| state.active_id.clone());
        document.environments = Some(PersistedEnvironmentsState {
            environments,
            active_id,
        });
    });
}

pub fn save_drafts(drafts: std::collections::HashMap<String, PersistedDraft>) {
    if let Err(error) = write_drafts_file(&PersistedDraftsState { drafts }) {
        log::warn!("Failed to persist drafts: {error}");
    }
}
