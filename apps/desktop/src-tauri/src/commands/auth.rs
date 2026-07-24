use crate::api::ApiClient;
use crate::error::AppResult;
use crate::models::{AuthStatusResponse, OAuthCallbackDto, OAuthLoginUrlDto};

#[tauri::command]
pub async fn get_auth_status() -> AppResult<AuthStatusResponse> {
    let client = ApiClient::new();
    crate::api::auth::get_auth_status(&client).await
}

#[tauri::command]
pub async fn get_login_url(provider: String) -> AppResult<serde_json::Value> {
    let client = ApiClient::new();
    let dto = OAuthLoginUrlDto { provider };
    crate::api::auth::get_login_url(&client, &dto).await
}

#[tauri::command]
pub async fn handle_oauth_callback(provider: String, code: String, state: Option<String>) -> AppResult<serde_json::Value> {
    let client = ApiClient::new();
    let dto = OAuthCallbackDto { provider, code, state };
    crate::api::auth::handle_callback(&client, &dto).await
}
