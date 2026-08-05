use crate::api::ApiClient;
use crate::error::AppResult;
use crate::models::{AuthStatusResponse, OAuthCallbackDto, OAuthLoginUrlDto};
use serde::Serialize;

pub async fn get_auth_status(client: &ApiClient) -> AppResult<AuthStatusResponse> {
    client.get("/auth/status").await
}

pub async fn get_login_url(client: &ApiClient, dto: &OAuthLoginUrlDto) -> AppResult<serde_json::Value> {
    client.post("/auth/login/url", dto).await
}

pub async fn handle_callback(client: &ApiClient, dto: &OAuthCallbackDto) -> AppResult<serde_json::Value> {
    client.post("/auth/login/callback", dto).await
}

#[derive(Debug, Serialize)]
pub struct ProjectIdDto {
    pub provider: String,
    #[serde(rename = "projectId")]
    pub project_id: Option<String>,
}

pub async fn get_project_id(client: &ApiClient, provider: &str) -> AppResult<serde_json::Value> {
    client.get(&format!("/auth/project-id/{}", provider)).await
}

pub async fn set_project_id(client: &ApiClient, dto: &ProjectIdDto) -> AppResult<serde_json::Value> {
    client.post("/auth/project-id", dto).await
}
