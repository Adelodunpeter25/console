use std::{
    fs,
    path::{Path, PathBuf},
};

use serde::{Deserialize, Serialize};

use super::{layout::PersistedLayoutState, window::PersistedWindowState};

const STORAGE_VERSION: u32 = 1;

#[derive(Clone, Debug, Default, Deserialize, Serialize)]
struct StorageDocument {
    version: u32,
    window: Option<PersistedWindowState>,
    layout: Option<PersistedLayoutState>,
}

fn storage_path() -> PathBuf {
    if let Ok(path) = std::env::var("CONSOLE_STATE_DIR") {
        return PathBuf::from(path).join("state.json");
    }

    #[cfg(target_os = "macos")]
    if let Ok(home) = std::env::var("HOME") {
        return PathBuf::from(home)
            .join("Library")
            .join("Application Support")
            .join("Console")
            .join("state.json");
    }

    #[cfg(target_os = "windows")]
    if let Ok(app_data) = std::env::var("APPDATA") {
        return PathBuf::from(app_data).join("Console").join("state.json");
    }

    if let Ok(config_home) = std::env::var("XDG_CONFIG_HOME") {
        return PathBuf::from(config_home)
            .join("console")
            .join("state.json");
    }

    std::env::var("HOME")
        .map(PathBuf::from)
        .unwrap_or_else(|_| PathBuf::from("."))
        .join(".config")
        .join("console")
        .join("state.json")
}

fn read_document() -> StorageDocument {
    let Ok(contents) = fs::read_to_string(storage_path()) else {
        return StorageDocument {
            version: STORAGE_VERSION,
            ..Default::default()
        };
    };

    match serde_json::from_str::<StorageDocument>(&contents) {
        Ok(document) if document.version == STORAGE_VERSION => document,
        _ => StorageDocument {
            version: STORAGE_VERSION,
            ..Default::default()
        },
    }
}

fn write_document(document: &StorageDocument) -> std::io::Result<()> {
    let path = storage_path();
    let directory = path.parent().unwrap_or_else(|| Path::new("."));
    fs::create_dir_all(directory)?;

    let temporary_path = path.with_extension(format!("json.{}.tmp", std::process::id()));
    let contents = serde_json::to_vec_pretty(document).map_err(std::io::Error::other)?;
    fs::write(&temporary_path, contents)?;
    fs::rename(temporary_path, path)
}

fn update_document(update: impl FnOnce(&mut StorageDocument)) {
    let mut document = read_document();
    update(&mut document);
    document.version = STORAGE_VERSION;
    if let Err(error) = write_document(&document) {
        log::warn!("Failed to persist console state: {error}");
    }
}

pub fn load_window() -> Option<PersistedWindowState> {
    read_document().window
}

pub fn save_window(state: PersistedWindowState) {
    update_document(|document| document.window = Some(state));
}

pub fn load_layout() -> Option<PersistedLayoutState> {
    read_document().layout
}

pub fn save_layout(state: PersistedLayoutState) {
    update_document(|document| document.layout = Some(state));
}
