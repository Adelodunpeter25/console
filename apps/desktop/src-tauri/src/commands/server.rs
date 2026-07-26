use tauri::AppHandle;

use crate::api::client::shared_http_client;
use crate::config::{get_server_url, health_url, save_config, set_server_url};
use crate::error::AppResult;

#[tauri::command]
pub async fn ping_server() -> AppResult<serde_json::Value> {
    let resp = shared_http_client()
        .get(health_url())
        .send()
        .await?
        .text()
        .await?;
    let json: serde_json::Value = serde_json::from_str(&resp)?;
    Ok(json)
}

#[tauri::command]
pub fn get_backend_url() -> String {
    get_server_url()
}

#[tauri::command]
pub fn set_backend_url(app: AppHandle, url: String) -> AppResult<()> {
    set_server_url(&url);
    save_config(&app)
}
