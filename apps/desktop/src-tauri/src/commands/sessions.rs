use crate::api::ApiClient;
use crate::error::AppResult;
use crate::models::{CreateSessionDto, SessionDetailResponse, SessionHeader, UpdateSessionDto};

#[tauri::command]
pub async fn list_sessions(cwd: Option<String>, project_id: Option<String>) -> AppResult<Vec<SessionHeader>> {
    let client = ApiClient::new();
    crate::api::sessions::list_sessions(
        &client,
        cwd.as_deref(),
        project_id.as_deref(),
    )
    .await
}

#[tauri::command]
pub async fn create_session(
    cwd: String,
    project_id: Option<String>,
    model_id: Option<String>,
    provider: Option<String>,
    title: Option<String>,
) -> AppResult<SessionHeader> {
    let client = ApiClient::new();
    let dto = CreateSessionDto {
        cwd,
        project_id,
        model_id,
        provider,
        title,
    };
    crate::api::sessions::create_session(&client, &dto).await
}

#[tauri::command]
pub async fn get_session(id: String) -> AppResult<SessionDetailResponse> {
    let client = ApiClient::new();
    crate::api::sessions::get_session(&client, &id).await
}

#[tauri::command]
pub async fn update_session(
    id: String,
    title: Option<String>,
    cwd: Option<String>,
    model_id: Option<String>,
    provider: Option<String>,
    approval_mode: Option<String>,
) -> AppResult<SessionHeader> {
    let client = ApiClient::new();
    let dto = UpdateSessionDto {
        title,
        cwd,
        model_id,
        provider,
        approval_mode,
    };
    crate::api::sessions::update_session(&client, &id, &dto).await
}

#[tauri::command]
pub async fn delete_session(id: String) -> AppResult<serde_json::Value> {
    let client = ApiClient::new();
    crate::api::sessions::delete_session(&client, &id).await
}
