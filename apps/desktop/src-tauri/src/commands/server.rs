use crate::config::{get_server_url, health_url, set_server_url};
use crate::error::AppResult;

#[tauri::command]
pub async fn ping_server() -> AppResult<serde_json::Value> {
    let client = reqwest::Client::new();
    let resp = client.get(health_url()).send().await?.text().await?;
    let json: serde_json::Value = serde_json::from_str(&resp)?;
    Ok(json)
}

#[tauri::command]
pub fn get_backend_url() -> String {
    get_server_url()
}

#[tauri::command]
pub fn set_backend_url(url: String) -> AppResult<()> {
    set_server_url(&url);
    Ok(())
}
