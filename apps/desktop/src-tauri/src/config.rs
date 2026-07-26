use std::path::PathBuf;
use std::sync::RwLock;

use once_cell::sync::Lazy;
use serde::{Deserialize, Serialize};
use tauri::{AppHandle, Manager};

use crate::error::AppResult;

static SERVER_URL: Lazy<RwLock<String>> = Lazy::new(|| RwLock::new("http://localhost:3000".to_string()));

#[derive(Serialize, Deserialize)]
struct PersistedConfig {
    #[serde(rename = "serverUrl")]
    server_url: String,
}

fn config_file(app: &AppHandle) -> Option<PathBuf> {
    app.path().app_data_dir().ok().map(|d| d.join("config.json"))
}

pub fn get_server_url() -> String {
    SERVER_URL.read().unwrap().clone()
}

pub fn set_server_url(url: &str) {
    let normalized = url.trim_end_matches('/').to_string();
    *SERVER_URL.write().unwrap() = normalized;
}

pub fn api_base() -> String {
    format!("{}/api", get_server_url())
}

pub fn health_url() -> String {
    format!("{}/health", get_server_url())
}

/// Load the persisted backend URL from the app data directory into memory.
/// Called once at startup so the in-memory default is replaced before the
/// frontend ever reads it.
pub fn load_config(app: &AppHandle) {
    if let Some(path) = config_file(app) {
        if let Ok(text) = std::fs::read_to_string(&path) {
            if let Ok(cfg) = serde_json::from_str::<PersistedConfig>(&text) {
                set_server_url(&cfg.server_url);
            }
        }
    }
}

/// Persist the current backend URL to the app data directory so it survives
/// app restarts.
pub fn save_config(app: &AppHandle) -> AppResult<()> {
    if let Some(path) = config_file(app) {
        if let Some(parent) = path.parent() {
            std::fs::create_dir_all(parent)?;
        }
        let cfg = PersistedConfig {
            server_url: get_server_url(),
        };
        let text = serde_json::to_string(&cfg)?;
        std::fs::write(&path, text)?;
    }
    Ok(())
}
