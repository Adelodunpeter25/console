use crate::api::ApiClient;
use crate::error::AppResult;
use crate::models::{FileSearchResponse, SlashCommandInfo};

#[tauri::command]
pub async fn list_slash_commands(session_id: String) -> AppResult<Vec<SlashCommandInfo>> {
    let client = ApiClient::new();
    crate::api::assist::list_commands(&client, &session_id).await
}

#[tauri::command]
pub async fn search_files(session_id: String, query: String) -> AppResult<FileSearchResponse> {
    let client = ApiClient::new();
    crate::api::assist::search_files(&client, &session_id, &query).await
}
