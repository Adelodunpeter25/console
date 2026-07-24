use crate::api::ApiClient;
use crate::error::AppResult;
use crate::models::FsTreeEntry;

#[tauri::command]
pub async fn browse_directory(path: Option<String>) -> AppResult<Vec<FsTreeEntry>> {
    let client = ApiClient::new();
    crate::api::fs::browse_directory(&client, path.as_deref()).await
}

#[tauri::command]
pub async fn pick_folder() -> AppResult<serde_json::Value> {
    let client = ApiClient::new();
    crate::api::fs::pick_folder(&client).await
}

#[tauri::command]
pub async fn get_directory_tree(path: Option<String>, depth: Option<u32>) -> AppResult<serde_json::Value> {
    let client = ApiClient::new();
    crate::api::fs::get_directory_tree(&client, path.as_deref(), depth).await
}

#[tauri::command]
pub async fn read_file(path: String, start_line: Option<u32>, end_line: Option<u32>) -> AppResult<serde_json::Value> {
    let client = ApiClient::new();
    crate::api::fs::read_file(&client, &path, start_line, end_line).await
}

#[tauri::command]
pub async fn write_file(path: String, content: String) -> AppResult<serde_json::Value> {
    let client = ApiClient::new();
    crate::api::fs::write_file(&client, &path, &content).await
}

#[tauri::command]
pub async fn delete_file(path: String) -> AppResult<serde_json::Value> {
    let client = ApiClient::new();
    crate::api::fs::delete_file(&client, &path).await
}

#[tauri::command]
pub async fn create_directory(path: String) -> AppResult<serde_json::Value> {
    let client = ApiClient::new();
    crate::api::fs::create_directory(&client, &path).await
}

#[tauri::command]
pub async fn delete_directory(path: String) -> AppResult<serde_json::Value> {
    let client = ApiClient::new();
    crate::api::fs::delete_directory(&client, &path).await
}
