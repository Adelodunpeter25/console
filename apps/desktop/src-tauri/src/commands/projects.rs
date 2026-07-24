use crate::api::ApiClient;
use crate::error::AppResult;
use crate::models::ProjectInfo;

#[tauri::command]
pub async fn list_projects() -> AppResult<Vec<ProjectInfo>> {
    let client = ApiClient::new();
    crate::api::projects::list_projects(&client).await
}

#[tauri::command]
pub async fn add_project(path: String) -> AppResult<ProjectInfo> {
    let client = ApiClient::new();
    crate::api::projects::add_project(&client, &path).await
}
