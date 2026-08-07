use crate::api::ApiClient;
use crate::error::AppResult;

#[tauri::command]
pub async fn get_git_status(path: Option<String>) -> AppResult<serde_json::Value> {
    let client = ApiClient::new();
    crate::api::git::get_git_status(&client, path.as_deref()).await
}
